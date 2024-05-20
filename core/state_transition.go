// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"fmt"
	"math"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/currency"
	"github.com/celo-org/celo-blockchain/contracts/erc20gas"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/core/vm/vmcontext"
	gpm "github.com/celo-org/celo-blockchain/eth/gasprice"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
	// cmath "github.com/celo-org/celo-blockchain/common/math"
)

// Used for EOA  Check
// var emptyCodeHash = crypto.Keccak256Hash(nil)

// The State Transitioning Model
//
// A state transition is a change made when a transaction is applied to the current world
// state. The state transitioning model does all the necessary work to work out a valid new
// state root.
//
//  1. Nonce handling
//  2. Pre pay gas
//  3. Create a new state object if the recipient is \0*32
//  4. Value transfer
//
// == If contract creation ==
//
//	4a. Attempt to run transaction data
//	4b. If valid, use result as code for the new state object
//
// == end ==
//
//  5. Run Script section
//  6. Derive new state root
type StateTransition struct {
	gp              *GasPool
	msg             Message
	gas             uint64
	gasPrice        *big.Int
	gasFeeCap       *big.Int
	gasTipCap       *big.Int
	initialGas      uint64
	value           *big.Int
	data            []byte
	state           vm.StateDB
	evm             *vm.EVM
	vmRunner        vm.EVMRunner
	gasPriceMinimum *big.Int
	sysCtx          *SysContractCallCtx
	erc20FeeDebited *big.Int
}

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	To() *common.Address

	GasPrice() *big.Int
	GasFeeCap() *big.Int
	GasTipCap() *big.Int
	Gas() uint64

	// FeeCurrency specifies the currency from which to pay for the fees
	// nil correspond to Celo Gold (native currency).
	// All other values should correspond to ERC20 contract addresses extended to be compatible with gas payments.
	FeeCurrency() *common.Address
	// MaxFeeInFeeCurrency Set iff it's a celo denominated tx. Maximum value of fees when converted to FeeCurrency.
	MaxFeeInFeeCurrency() *big.Int
	GatewayFeeRecipient() *common.Address
	GatewayFee() *big.Int
	GatewaySet() bool
	Value() *big.Int

	Nonce() uint64
	IsFake() bool
	Data() []byte
	AccessList() types.AccessList

	// Whether this transaction omitted the 3 Celo-only fields (FeeCurrency & co.)
	EthCompatible() bool
}

func CheckEthCompatibility(msg Message) error {
	if msg.EthCompatible() && (msg.FeeCurrency() != nil || msg.GatewaySet()) {
		return types.ErrEthCompatibleTransactionIsntCompatible
	}
	return nil

}

// ExecutionResult includes all output after executing given evm
// message no matter the execution itself is successful or not.
type ExecutionResult struct {
	UsedGas    uint64 // Total used gas but include the refunded gas
	Err        error  // Any error encountered during the execution(listed in core/vm/errors.go)
	ReturnData []byte // Returned data from evm(function result or data supplied with revert opcode)
}

// Unwrap returns the internal evm error which allows us for further
// analysis outside.
func (result *ExecutionResult) Unwrap() error {
	return result.Err
}

// Failed returns the indicator whether the execution is successful or not
func (result *ExecutionResult) Failed() bool { return result.Err != nil }

// Return is a helper function to help caller distinguish between revert reason
// and function return. Return returns the data after execution if no error occurs.
func (result *ExecutionResult) Return() []byte {
	if result.Err != nil {
		return nil
	}
	return common.CopyBytes(result.ReturnData)
}

// Revert returns the concrete revert reason if the execution is aborted by `REVERT`
// opcode. Note the reason can be nil if no data supplied with revert opcode.
func (result *ExecutionResult) Revert() []byte {
	if result.Err != vm.ErrExecutionReverted {
		return nil
	}
	return common.CopyBytes(result.ReturnData)
}

// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
func IntrinsicGas(data []byte, accessList types.AccessList, isContractCreation bool, feeCurrency *common.Address, gasForAlternativeCurrency uint64, isEIP2028 bool) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if isContractCreation {
		gas = params.TxGasContractCreation
	} else {
		gas = params.TxGas
	}
	// Bump the required gas by the amount of transactional data
	if len(data) > 0 {
		// Zero and non-zero bytes are priced differently
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		// Make sure we don't exceed uint64 for all data combinations
		nonZeroGas := params.TxDataNonZeroGasFrontier
		if isEIP2028 {
			nonZeroGas = params.TxDataNonZeroGasEIP2028
		}

		if (math.MaxUint64-gas)/nonZeroGas < nz {
			log.Debug("IntrinsicGas", "gas uint overflow")
			return 0, ErrGasUintOverflow
		}
		gas += nz * nonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			log.Debug("IntrinsicGas", "gas uint overflow")
			return 0, ErrGasUintOverflow
		}
		gas += z * params.TxDataZeroGas
	}

	// This gas is used for charging user for one `debitFrom` transaction to deduct their balance in
	// non-native currency and two `creditTo` transactions, one covers for the  miner fee in
	// non-native currency at the end and the other covers for the user refund at the end.
	// A user might or might not have a gas refund at the end and even if they do the gas refund might
	// be smaller than maxGasForDebitAndCreditTransactions. We still decide to deduct and do the refund
	// since it makes the mining fee more consistent with respect to the gas fee. Otherwise, we would
	// have to expect the user to estimate the mining fee right or else end up losing
	// min(gas sent - gas charged, maxGasForDebitAndCreditTransactions) extra.
	// In this case, however, the user always ends up paying maxGasForDebitAndCreditTransactions
	// keeping it consistent.
	if feeCurrency != nil {
		if (math.MaxUint64 - gas) < gasForAlternativeCurrency {
			log.Debug("IntrinsicGas", "gas uint overflow")
			return 0, ErrGasUintOverflow
		}
		gas += gasForAlternativeCurrency
	}

	if accessList != nil {
		gas += uint64(len(accessList)) * params.TxAccessListAddressGas
		gas += uint64(accessList.StorageKeys()) * params.TxAccessListStorageKeyGas
	}
	return gas, nil
}

// NewStateTransition initialises and returns a new state transition object.
func NewStateTransition(evm *vm.EVM, msg Message, gp *GasPool, vmRunner vm.EVMRunner, sysCtx *SysContractCallCtx) *StateTransition {
	var gasPriceMinimum *big.Int
	if evm.ChainConfig().IsEspresso(evm.Context.BlockNumber) {
		if msg.MaxFeeInFeeCurrency() != nil {
			// Celo denominated tx
			gasPriceMinimum = sysCtx.GetGasPriceMinimum(nil)
		} else {
			gasPriceMinimum = sysCtx.GetGasPriceMinimum(msg.FeeCurrency())
		}
	} else {
		gasPriceMinimum, _ = gpm.GetBaseFeeForCurrency(vmRunner, msg.FeeCurrency(), nil)
	}

	return &StateTransition{
		gp:              gp,
		evm:             evm,
		vmRunner:        vmRunner,
		msg:             msg,
		gasPrice:        msg.GasPrice(),
		gasFeeCap:       msg.GasFeeCap(),
		gasTipCap:       msg.GasTipCap(),
		value:           msg.Value(),
		data:            msg.Data(),
		state:           evm.StateDB,
		gasPriceMinimum: gasPriceMinimum,
		sysCtx:          sysCtx,
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
//
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
func ApplyMessage(evm *vm.EVM, msg Message, gp *GasPool, vmRunner vm.EVMRunner, sysCtx *SysContractCallCtx) (*ExecutionResult, error) {
	log.Trace("Applying state transition message", "from", msg.From(), "nonce", msg.Nonce(), "to", msg.To(), "gas price", msg.GasPrice(), "fee currency", msg.FeeCurrency(), "gateway fee recipient", msg.GatewayFeeRecipient(), "gateway fee", msg.GatewayFee(), "gas", msg.Gas(), "value", msg.Value(), "data", msg.Data())
	return NewStateTransition(evm, msg, gp, vmRunner, sysCtx).TransitionDb()
}

// to returns the recipient of the message.
func (st *StateTransition) to() common.Address {
	if st.msg == nil || st.msg.To() == nil /* contract creation */ {
		return common.Address{}
	}
	return *st.msg.To()
}

// payFees deducts gas and gateway fees from sender balance and adds the purchased amount of gas to the state.
func (st *StateTransition) payFees(espresso bool, denominatedCurrencyRate *currency.ExchangeRate) error {
	var isWhiteListed bool
	if espresso {
		isWhiteListed = st.sysCtx.IsWhitelisted(st.msg.FeeCurrency())
	} else {
		isWhiteListed = currency.IsWhitelisted(st.vmRunner, st.msg.FeeCurrency())
	}
	if !isWhiteListed {
		log.Trace("Fee currency not whitelisted", "fee currency address", st.msg.FeeCurrency())
		return ErrNonWhitelistedFeeCurrency
	}
	if err := st.canPayFee(st.msg.From(), st.msg.FeeCurrency(), espresso, denominatedCurrencyRate); err != nil {
		return err
	}
	if err := st.gp.SubGas(st.msg.Gas()); err != nil {
		return err
	}

	st.initialGas = st.msg.Gas()
	st.gas += st.msg.Gas()
	err := st.debitFee(st.msg.From(), st.msg.FeeCurrency(), denominatedCurrencyRate)
	return err
}

// canPayFee checks whether accountOwner's balance can cover transaction fee.
//
// For native token(CELO) as feeCurrency:
//   - Pre-Espresso: it ensures balance >= GasPrice * gas + gatewayFee (1)
//   - Post-Espresso: it ensures balance >= GasFeeCap * gas + value + gatewayFee (2)
//
// For non-native tokens(cUSD, cEUR, ...) as feeCurrency:
//   - Pre-Espresso: it ensures balance > GasPrice * gas + gatewayFee (3)
//   - Post-Espresso: it ensures balance >= GasFeeCap * gas + gatewayFee (4)
func (st *StateTransition) canPayFee(accountOwner common.Address, feeCurrency *common.Address, espresso bool, denominatedCurrencyRate *currency.ExchangeRate) error {
	if feeCurrency == nil {
		balance := st.state.GetBalance(st.msg.From())
		if espresso {
			// feeGap = GasFeeCap * gas + value + gatewayFee, as in (2)
			feeGap := new(big.Int).SetUint64(st.msg.Gas())
			feeGap.Mul(feeGap, st.gasFeeCap)
			feeGap.Add(feeGap, st.value)
			if st.msg.GatewayFeeRecipient() != nil {
				feeGap.Add(feeGap, st.msg.GatewayFee())
			}

			if balance.Cmp(feeGap) < 0 {
				return fmt.Errorf("%w: address %v have %v want %v", ErrInsufficientFunds, st.msg.From().Hex(), balance, feeGap)
			}
		} else {
			// effectiveFee = GasPrice * gas + gatewayFee, as in (1)
			effectiveFee := new(big.Int).Mul(new(big.Int).SetUint64(st.msg.Gas()), st.gasPrice)
			if st.msg.GatewayFeeRecipient() != nil {
				effectiveFee.Add(effectiveFee, st.msg.GatewayFee())
			}

			if balance.Cmp(effectiveFee) < 0 {
				return fmt.Errorf("%w: address %v have %v want %v", ErrInsufficientFunds, st.msg.From().Hex(), balance, effectiveFee)
			}
		}
		return nil
	} else {
		balance, err := currency.GetBalanceOf(st.vmRunner, accountOwner, *feeCurrency)
		if err != nil {
			return err
		}
		if espresso {
			var feeGap *big.Int
			// feeGap = GasFeeCap * gas + gatewayFee, as in (4)
			feeGap = new(big.Int).SetUint64(st.msg.Gas())
			feeGap.Mul(feeGap, st.gasFeeCap)
			if st.msg.GatewayFeeRecipient() != nil {
				feeGap.Add(feeGap, st.msg.GatewayFee())
			}
			if st.msg.MaxFeeInFeeCurrency() != nil {
				// Celo Denominated Tx: max fee is a tx field
				// Translate fees to feeCurrency
				feeGap = denominatedCurrencyRate.FromBase(feeGap)
				// Check that fee <= MaxFeeInFeeCurrency
				if feeGap.Cmp(st.msg.MaxFeeInFeeCurrency()) > 0 {
					return ErrDenominatedLowMaxFee
				}
			}
			if balance.Cmp(feeGap) < 0 {
				return fmt.Errorf("%w: address %v have %v want %v", ErrInsufficientFunds, st.msg.From().Hex(), balance, feeGap)
			}
		} else {
			// effectiveFee = GasPrice * gas + gatewayFee, as in (3)
			effectiveFee := new(big.Int).Mul(new(big.Int).SetUint64(st.msg.Gas()), st.gasPrice)
			if st.msg.GatewayFeeRecipient() != nil {
				effectiveFee.Add(effectiveFee, st.msg.GatewayFee())
			}
			if balance.Cmp(effectiveFee) <= 0 {
				return fmt.Errorf("%w: address %v have %v want %v", ErrInsufficientFunds, st.msg.From().Hex(), balance, effectiveFee)
			}
		}
	}
	return nil
}

func (st *StateTransition) debitFee(from common.Address, feeCurrency *common.Address, denominatedCurrencyRate *currency.ExchangeRate) (err error) {
	if st.evm.Config.SkipDebitCredit {
		return nil
	}
	effectiveFee := new(big.Int).Mul(new(big.Int).SetUint64(st.msg.Gas()), st.gasPrice)
	// If GatewayFeeRecipient is unspecified, the gateway fee value is ignore and the sender is not charged.
	if st.msg.GatewayFeeRecipient() != nil {
		effectiveFee.Add(effectiveFee, st.msg.GatewayFee())
	}
	log.Trace("Debiting fee", "from", from, "amount", effectiveFee, "feeCurrency", feeCurrency)
	// native currency
	if feeCurrency == nil {
		st.state.SubBalance(from, effectiveFee)
		return nil
	} else {
		if st.msg.MaxFeeInFeeCurrency() != nil {
			st.erc20FeeDebited = denominatedCurrencyRate.FromBase(effectiveFee)
		} else {
			st.erc20FeeDebited = effectiveFee
		}
		return erc20gas.DebitFees(st.evm, from, st.erc20FeeDebited, feeCurrency)
	}
}

func (st *StateTransition) preCheck() error {
	// Only check transactions that are not fake
	if !st.msg.IsFake() {
		// Make sure this transaction's nonce is correct.
		stNonce := st.state.GetNonce(st.msg.From())
		if msgNonce := st.msg.Nonce(); stNonce < msgNonce {
			return fmt.Errorf("%w: address %v, tx: %d state: %d", ErrNonceTooHigh,
				st.msg.From().Hex(), msgNonce, stNonce)
		} else if stNonce > msgNonce {
			return fmt.Errorf("%w: address %v, tx: %d state: %d", ErrNonceTooLow,
				st.msg.From().Hex(), msgNonce, stNonce)
		}
	}
	// Make sure that transaction gasFeeCap is greater than the baseFee (post london)
	if st.evm.ChainConfig().IsLondon(st.evm.Context.BlockNumber) {
		// Skip the checks if gas fields are zero and baseFee was explicitly disabled (eth_call)
		if !st.evm.Config.NoBaseFee || st.gasFeeCap.BitLen() > 0 || st.gasTipCap.BitLen() > 0 {
			if l := st.gasFeeCap.BitLen(); l > 256 {
				return fmt.Errorf("%w: address %v, maxFeePerGas bit length: %d", ErrFeeCapVeryHigh,
					st.msg.From().Hex(), l)
			}
			if l := st.gasTipCap.BitLen(); l > 256 {
				return fmt.Errorf("%w: address %v, maxPriorityFeePerGas bit length: %d", ErrTipVeryHigh,
					st.msg.From().Hex(), l)
			}
			if st.gasFeeCap.Cmp(st.gasTipCap) < 0 {
				return fmt.Errorf("%w: address %v, maxPriorityFeePerGas: %s, maxFeePerGas: %s", ErrTipAboveFeeCap,
					st.msg.From().Hex(), st.gasTipCap, st.gasFeeCap)
			}
			if st.gasFeeCap.Cmp(st.gasPriceMinimum) < 0 {
				return fmt.Errorf("%w: address %v, maxFeePerGas: %s baseFee: %s", ErrFeeCapTooLow,
					st.msg.From().Hex(), st.gasFeeCap, st.gasPriceMinimum)
			}
		}
	} else { // Make sure this transaction's gas price >= baseFee (pre Espresso)
		if !st.evm.Config.NoBaseFee && st.gasPrice.Cmp(st.gasPriceMinimum) < 0 {
			log.Debug("Tx gas price is less than minimum", "minimum", st.gasPriceMinimum, "price", st.gasPrice)
			return ErrGasPriceDoesNotExceedMinimum
		}
	}

	// // Make sure the sender is an EOA
	// if codeHash := st.state.GetCodeHash(st.msg.From()); codeHash != emptyCodeHash && codeHash != (common.Hash{}) {
	// 	return fmt.Errorf("%w: address %v, codehash: %s", ErrSenderNoEOA,
	// 		st.msg.From().Hex(), codeHash)
	// }

	return nil

}

// TransitionDb will transition the state by applying the current message and
// returning the evm execution result with following fields.
//
//   - used gas: total gas used (including gas being refunded)
//   - returndata: the returned data from evm
//   - concrete execution error: various EVM errors which abort the execution, e.g.
//     ErrOutOfGas, ErrExecutionReverted
//
// However if any consensus issue encountered, return the error directly with
// nil evm execution result.
func (st *StateTransition) TransitionDb() (*ExecutionResult, error) {
	// First check this message satisfies all consensus rules before
	// applying the message. The rules include these clauses
	//
	// 0. If the message is from an eth-compatible transaction, that we support those
	//    and that none of the non-eth-compatible fields are present
	// 1. the nonce of the message caller is correct
	// 2. the gas price meets the minimum gas price
	// 3. caller has enough balance (in the right currency) to cover transaction fee
	// 4. the amount of gas required is available in the block
	// 5. the purchased gas is enough to cover intrinsic usage
	// 6. there is no overflow when calculating intrinsic gas
	// 7. caller has enough balance to cover asset transfer for **topmost** call

	// Clause 0
	if st.msg.EthCompatible() && !st.evm.ChainConfig().IsDonut(st.evm.Context.BlockNumber) {
		return nil, ErrEthCompatibleTransactionsNotSupported
	}
	if err := CheckEthCompatibility(st.msg); err != nil {
		return nil, err
	}
	if st.evm.ChainConfig().IsGingerbread(st.evm.Context.BlockNumber) && st.msg.GatewaySet() {
		return nil, ErrGatewayFeeDeprecated
	}
	if !st.evm.ChainConfig().IsHFork(st.evm.Context.BlockNumber) && st.msg.MaxFeeInFeeCurrency() != nil {
		return nil, ErrTxTypeNotSupported
	}
	if st.msg.FeeCurrency() == nil && st.msg.MaxFeeInFeeCurrency() != nil {
		return nil, ErrDenominatedNoCurrency
	}
	var denominatedCurrencyRate *currency.ExchangeRate
	// Get rate only for CELO denominated txs (H fork)
	if st.msg.MaxFeeInFeeCurrency() != nil {
		var err error
		denominatedCurrencyRate, err = currency.GetExchangeRate(st.vmRunner, st.msg.FeeCurrency())
		if err != nil {
			return nil, err
		}
	}

	// Check clauses 1-2
	if err := st.preCheck(); err != nil {
		return nil, err
	}

	if tracer := st.evm.Config.Tracer; tracer != nil {
		tracer.CaptureTxStart(st.initialGas)
		defer func() {
			tracer.CaptureTxEnd(st.gas)
		}()
	}

	msg := st.msg
	sender := vm.AccountRef(msg.From())
	istanbul := st.evm.ChainConfig().IsIstanbul(st.evm.Context.BlockNumber)
	espresso := st.evm.ChainConfig().IsEspresso(st.evm.Context.BlockNumber)
	contractCreation := msg.To() == nil

	// Calculate intrinsic gas, check clauses 5-6
	gasForAlternativeCurrency := uint64(0)
	// If the fee currency is nil, do not retrieve the intrinsic gas adjustment from the chain state, as it will not be used.
	if msg.FeeCurrency() != nil {
		if espresso {
			gasForAlternativeCurrency = st.sysCtx.GetIntrinsicGasForAlternativeFeeCurrency()
		} else {
			gasForAlternativeCurrency = blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrencyOrDefault(st.vmRunner)
		}
	}
	gas, err := IntrinsicGas(st.data, st.msg.AccessList(), contractCreation, msg.FeeCurrency(), gasForAlternativeCurrency, istanbul)
	if err != nil {
		return nil, err
	}

	// If the intrinsic gas is more than provided in the tx, return without failing.
	if gas > st.msg.Gas() {
		return nil, fmt.Errorf("%w: have %d, want %d", ErrIntrinsicGas, st.msg.Gas(), gas)
	}
	// Check clauses 3-4, pay the fees (which buys gas), and subtract the intrinsic gas
	err = st.payFees(espresso, denominatedCurrencyRate)
	if err != nil {
		log.Error("Transaction failed to buy gas", "err", err, "gas", gas)
		return nil, err
	}
	st.gas -= gas

	// Check clause 7
	if msg.Value().Sign() > 0 && !st.evm.Context.CanTransfer(st.state, msg.From(), msg.Value()) {
		return nil, fmt.Errorf("%w: address %v", ErrInsufficientFundsForTransfer, msg.From().Hex())
	}

	// Set up the initial access list.
	if rules := st.evm.ChainConfig().Rules(st.evm.Context.BlockNumber); rules.IsBerlin {
		st.state.PrepareAccessList(msg.From(), msg.To(), vm.ActivePrecompiles(rules), msg.AccessList())
	}
	var (
		ret   []byte
		vmerr error // vm errors do not effect consensus and are therefore not assigned to err
	)
	if contractCreation {
		ret, _, st.gas, vmerr = st.evm.Create(sender, st.data, st.gas, st.value)
	} else {
		// Increment the nonce for the next transaction
		st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
		ret, st.gas, vmerr = st.evm.Call(sender, st.to(), st.data, st.gas, st.value)
	}

	if !espresso {
		// Before EIP-3529: refunds were capped to gasUsed / 2
		st.refundGas(params.RefundQuotient)
	} else {
		// After EIP-3529: refunds are capped to gasUsed / 5
		st.refundGas(params.RefundQuotientEIP3529)
	}

	err = st.creditTxFees(denominatedCurrencyRate)
	if err != nil {
		return nil, err
	}
	return &ExecutionResult{
		UsedGas:    st.gasUsed(),
		Err:        vmerr,
		ReturnData: ret,
	}, nil
}

// creditTxFees calculates the amounts and recipients of transaction fees and credits the accounts.
func (st *StateTransition) creditTxFees(denominatedCurrencyRate *currency.ExchangeRate) error {
	if st.evm.Config.SkipDebitCredit {
		return nil
	}

	// Run only primary evm.Call() with tracer
	if st.evm.GetDebug() {
		st.evm.SetDebug(false)
		defer func() { st.evm.SetDebug(true) }()
	}

	// Determine the refund and transaction fee to be distributed.
	refund := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	gasUsed := new(big.Int).SetUint64(st.gasUsed())
	totalTxFee := new(big.Int).Mul(gasUsed, st.gasPrice)
	from := st.msg.From()

	// Divide the transaction into a base (the minimum transaction fee) and tip (any extra, or min(max tip, feecap - GPM) if espresso).
	baseTxFee := new(big.Int).Mul(gasUsed, st.gasPriceMinimum)
	// No need to do effectiveTip calculation, because st.gasPrice == effectiveGasPrice, and effectiveTip = effectiveGasPrice - baseTxFee
	tipTxFee := new(big.Int).Sub(totalTxFee, baseTxFee)

	if st.msg.MaxFeeInFeeCurrency() != nil {
		// Celo Denominated

		// We want to ensure that
		// 		st.erc20FeeDebited = tipTxFee + baseTxFee + refund
		// so that debit and credit totals match. Since the exchange rate
		// conversions have limited accuracy, the only way to achieve this
		// is to calculate one of the three credit values based on the two
		// others after the exchange rate conversion.
		tipTxFee = denominatedCurrencyRate.FromBase(tipTxFee)
		baseTxFee = denominatedCurrencyRate.FromBase(baseTxFee)
		totalTxFee.Add(tipTxFee, baseTxFee)
		refund.Sub(st.erc20FeeDebited, totalTxFee) // refund = debited - tip - basefee
		// No need to exchange gateway fee since it's it's deprecated on G fork,
		// and MaxFeeInFeeCurrency can only be present in H fork (which implies G fork)
	}

	feeCurrency := st.msg.FeeCurrency()

	gatewayFeeRecipient := st.msg.GatewayFeeRecipient()
	if gatewayFeeRecipient == nil {
		gatewayFeeRecipient = &common.ZeroAddress
	}

	caller := &vmcontext.SharedEVMRunner{EVM: st.evm}
	var contractId [32]byte
	if st.evm.ChainConfig().IsGingerbread(st.evm.Context.BlockNumber) {
		contractId = config.FeeHandlerId
	} else {
		contractId = config.GovernanceRegistryId
	}
	feeHandlerAddress, err := contracts.GetRegisteredAddress(caller, contractId)
	if err != nil {
		if err != contracts.ErrSmartContractNotDeployed && err != contracts.ErrRegistryContractNotDeployed {
			return err
		}
		log.Trace("Cannot credit gas fee to FeeHandler: refunding fee to sender", "error", err, "fee", baseTxFee)
		feeHandlerAddress = common.ZeroAddress
		refund.Add(refund, baseTxFee)
		baseTxFee = new(big.Int)
	}

	log.Trace("creditTxFees", "from", from, "refund", refund, "feeCurrency", st.msg.FeeCurrency(),
		"gatewayFeeRecipient", *gatewayFeeRecipient, "gatewayFee", st.msg.GatewayFee(),
		"coinbaseFeeRecipient", st.evm.Context.Coinbase, "coinbaseFee", tipTxFee,
		"feeHandler", feeHandlerAddress, "communityFundFee", baseTxFee)
	if feeCurrency == nil {
		if gatewayFeeRecipient != &common.ZeroAddress {
			st.state.AddBalance(*gatewayFeeRecipient, st.msg.GatewayFee())
		}
		if feeHandlerAddress != common.ZeroAddress {
			st.state.AddBalance(feeHandlerAddress, baseTxFee)
		}
		st.state.AddBalance(st.evm.Context.Coinbase, tipTxFee)
		st.state.AddBalance(from, refund)
	} else {
		if err = erc20gas.CreditFees(st.evm, from, st.evm.Context.Coinbase, gatewayFeeRecipient, feeHandlerAddress, refund, tipTxFee, st.msg.GatewayFee(), baseTxFee, feeCurrency); err != nil {
			log.Error("Error crediting", "from", from, "coinbase", st.evm.Context.Coinbase, "gateway", gatewayFeeRecipient, "feeHandler", feeHandlerAddress)
			return err
		}

	}
	return nil
}

// refundGas adds unused gas back the state transition and gas pool.
func (st *StateTransition) refundGas(refundQuotient uint64) {
	// Apply refund counter, capped to a refund quotient
	refund := st.gasUsed() / refundQuotient
	if refund > st.state.GetRefund() {
		refund = st.state.GetRefund()
	}

	st.gas += refund
	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	st.gp.AddGas(st.gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gas
}
