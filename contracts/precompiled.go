package contracts

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/common/math"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/core/vm/vmcontext"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
)

var once sync.Once

func Init() {
	once.Do(func() {
		vm.PrecompiledContractsByzantium[vm.TransferAddress] = &transfer{}
		vm.PrecompiledContractsIstanbul[vm.TransferAddress] = &transfer{}
		vm.PrecompiledContractsDonut[vm.TransferAddress] = &transfer{}
		vmcontext.Transfer = TobinTransfer
	})
}

// Native transfer contract to make Celo Gold ERC20 compatible.
type transfer struct{}

func (c *transfer) RequiredGas(input []byte) uint64 {
	return params.CallValueTransferGas
}

func (c *transfer) Run(input []byte, caller common.Address, evm *vm.EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := vm.DebitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	celoGoldAddress, err := GetRegisteredAddress(&SharedSystemEVM{evm}, params.GoldTokenRegistryId)
	if err != nil {
		return nil, gas, err
	}

	// input is comprised of 3 arguments:
	//   from:  32 bytes representing the address of the sender
	//   to:    32 bytes representing the address of the recipient
	//   value: 32 bytes, a 256 bit integer representing the amount of Celo Gold to transfer
	// 3 arguments x 32 bytes each = 96 bytes total input
	if len(input) < 96 {
		return nil, gas, vm.ErrInputLength
	}

	if caller != celoGoldAddress {
		return nil, gas, fmt.Errorf("Unable to call transfer from unpermissioned address")
	}
	from := common.BytesToAddress(input[0:32])
	to := common.BytesToAddress(input[32:64])

	var parsed bool
	value, parsed := math.ParseBig256(hexutil.Encode(input[64:96]))
	if !parsed {
		return nil, gas, fmt.Errorf("Error parsing transfer: unable to parse value from " + hexutil.Encode(input[64:96]))
	}

	if from == common.ZeroAddress {
		// Mint case: Create cGLD out of thin air
		evm.StateDB.AddBalance(to, value)
	} else {
		// Fail if we're trying to transfer more than the available balance
		if !evm.Context.CanTransfer(evm.StateDB, from, value) {
			return nil, gas, vm.ErrInsufficientBalance
		}

		evm.Transfer(evm, from, to, value)
	}

	return input, gas, err
}

// TobinTransfer performs a transfer that may take a tax from the sent amount and give it to the reserve.
// If the calculation or transfer of the tax amount fails for any reason, the regular transfer goes ahead.
// NB: Gas is not charged or accounted for this calculation.
func TobinTransfer(evm *vm.EVM, sender, recipient common.Address, amount *big.Int) {
	// Run only primary evm.Call() with tracer
	if evm.GetDebug() {
		evm.SetDebug(false)
		defer func() { evm.SetDebug(true) }()
	}

	if amount.Cmp(big.NewInt(0)) != 0 {
		tax, taxRecipient, err := ComputeTobinTax(&SharedSystemEVM{evm}, sender, amount)
		if err == nil {
			Transfer(evm.StateDB, sender, recipient, new(big.Int).Sub(amount, tax))
			Transfer(evm.StateDB, sender, taxRecipient, tax)
			return
		} else {
			log.Error("Failed to get tobin tax", "error", err)
		}
	}

	// Complete a normal transfer if the amount is 0 or the tobin tax value is unable to be fetched and parsed.
	// We transfer even when the amount is 0 because state trie clearing [EIP161] is necessary at the end of a transaction
	Transfer(evm.StateDB, sender, recipient, amount)
}

// Transfer subtracts amount from sender and adds amount to recipient using the given Db
func Transfer(db vm.StateDB, sender, recipient common.Address, amount *big.Int) {
	db.SubBalance(sender, amount)
	db.AddBalance(recipient, amount)
}

func TobinTax(caller vm.SystemEVM, sender common.Address) (tax Ratio, reserveAddress common.Address, err error) {

	reserveAddress, err = GetRegisteredAddress(caller, params.ReserveRegistryId)
	if err != nil {
		return Ratio{}, common.ZeroAddress, err
	}

	ret, _, err := caller.Execute(VMAddress, reserveAddress, params.TobinTaxFunctionSelector, params.MaxGasForGetOrComputeTobinTax, big.NewInt(0))
	if err != nil {
		return Ratio{}, common.ZeroAddress, err
	}

	// Expected size of ret is 64 bytes because getOrComputeTobinTax() returns two uint256 values,
	// each of which is equivalent to 32 bytes
	if binary.Size(ret) != 64 {
		return Ratio{}, common.ZeroAddress, errors.New("length of tobin tax not equal to 64 bytes")
	}
	numerator := new(big.Int).SetBytes(ret[0:32])
	denominator := new(big.Int).SetBytes(ret[32:64])
	if denominator.Cmp(common.Big0) == 0 {
		return Ratio{}, common.ZeroAddress, ErrTobinTaxZeroDenominator
	}
	if numerator.Cmp(denominator) == 1 {
		return Ratio{}, common.ZeroAddress, ErrTobinTaxInvalidNumerator
	}
	return Ratio{numerator, denominator}, reserveAddress, nil
}

func ComputeTobinTax(caller vm.SystemEVM, sender common.Address, transferAmount *big.Int) (tax *big.Int, taxRecipient common.Address, err error) {
	taxRatio, recipient, err := TobinTax(caller, sender)
	if err != nil {
		return nil, common.ZeroAddress, err
	}

	return taxRatio.Apply(transferAmount), recipient, nil
}

var (
	ErrTobinTaxZeroDenominator  = errors.New("tobin tax denominator equal to zero")
	ErrTobinTaxInvalidNumerator = errors.New("tobin tax numerator greater than denominator")
)

type Ratio struct {
	numerator, denominator *big.Int
}

func (r *Ratio) Apply(value *big.Int) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(r.numerator, value), r.denominator)
}
