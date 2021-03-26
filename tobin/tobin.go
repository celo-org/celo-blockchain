package tobin

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/core/vm/runtime"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
)

func NewEnv(cfg *runtime.Config) *vm.EVM {

	context := vm.Context{
		CanTransfer: vm.CanTransfer,
		Transfer:    TobinTransfer,

		GetHash: cfg.GetHashFn,

		Origin:      cfg.Origin,
		Coinbase:    cfg.Coinbase,
		BlockNumber: cfg.BlockNumber,
		Time:        cfg.Time,
		GasPrice:    cfg.GasPrice,
	}

	if cfg.ChainConfig.Istanbul != nil {
		context.EpochSize = cfg.ChainConfig.Istanbul.Epoch
	}

	return vm.NewEVM(context, cfg.State, cfg.ChainConfig, cfg.EVMConfig)
}

// NewEVMContext creates a new context for use in the EVM.
func NewEVMContext(msg vm.Message, header *types.Header, chain vm.ChainContext, txFeeRecipient *common.Address) vm.Context {
	// If we don't have an explicit txFeeRecipient (i.e. not mining), extract from the header
	// The only call that fills the txFeeRecipient, is the ApplyTransaction from the state processor
	// All the other calls, assume that will be retrieved from the header
	var beneficiary common.Address
	if txFeeRecipient == nil {
		beneficiary = header.Coinbase
	} else {
		beneficiary = *txFeeRecipient
	}

	ctx := vm.Context{
		CanTransfer: vm.CanTransfer,
		Transfer:    TobinTransfer,
		GetHash:     vm.GetHashFn(header, chain),
		VerifySeal:  vm.VerifySealFn(header, chain),
		Origin:      msg.From(),
		Coinbase:    beneficiary,
		BlockNumber: new(big.Int).Set(header.Number),
		Time:        new(big.Int).SetUint64(header.Time),
		GasPrice:    new(big.Int).Set(msg.GasPrice()),
	}

	if chain != nil {
		ctx.EpochSize = chain.Engine().EpochSize()
		ctx.GetValidators = chain.Engine().GetValidators
		ctx.GetHeaderByNumber = chain.GetHeaderByNumber
	} else {
		ctx.GetValidators = func(blockNumber *big.Int, headerHash common.Hash) []istanbul.Validator { return nil }
		ctx.GetHeaderByNumber = func(uint64) *types.Header { panic("evm context without blockchain context") }
	}
	return ctx
}

func getTobinTax(evm *vm.EVM, sender common.Address) (numerator *big.Int, denominator *big.Int, reserveAddress *common.Address, err error) {
	reserveAddress, err = vm.GetRegisteredAddressWithEvm(params.ReserveRegistryId, evm)
	if err != nil {
		return nil, nil, nil, err
	}

	ret, _, err := evm.Call(vm.AccountRef(sender), *reserveAddress, params.TobinTaxFunctionSelector, params.MaxGasForGetOrComputeTobinTax, big.NewInt(0))
	if err != nil {
		return nil, nil, nil, err
	}

	// Expected size of ret is 64 bytes because getOrComputeTobinTax() returns two uint256 values,
	// each of which is equivalent to 32 bytes
	if binary.Size(ret) != 64 {
		return nil, nil, nil, errors.New("Length of tobin tax not equal to 64 bytes")
	}
	numerator = new(big.Int).SetBytes(ret[0:32])
	denominator = new(big.Int).SetBytes(ret[32:64])
	if denominator.Cmp(common.Big0) == 0 {
		return nil, nil, nil, errors.New("Tobin tax denominator equal to zero")
	}
	if numerator.Cmp(denominator) == 1 {
		return nil, nil, nil, errors.New("Tobin tax numerator greater than denominator")
	}
	return numerator, denominator, reserveAddress, nil
}

func Transfer(db vm.StateDB, sender, recipient common.Address, amount *big.Int) {
	db.SubBalance(sender, amount)
	db.AddBalance(recipient, amount)
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
		numerator, denominator, reserveAddress, err := getTobinTax(evm, sender)
		if err == nil {
			tobinTax := new(big.Int).Div(new(big.Int).Mul(numerator, amount), denominator)
			Transfer(evm.StateDB, sender, recipient, new(big.Int).Sub(amount, tobinTax))
			Transfer(evm.StateDB, sender, *reserveAddress, tobinTax)
			return
		} else {
			log.Error("Failed to get tobin tax", "error", err)
		}
	}

	// Complete a normal transfer if the amount is 0 or the tobin tax value is unable to be fetched and parsed.
	// We transfer even when the amount is 0 because state trie clearing [EIP161] is necessary at the end of a transaction
	Transfer(evm.StateDB, sender, recipient, amount)
}
