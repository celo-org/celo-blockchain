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
	gpm "github.com/celo-org/celo-blockchain/contracts/gasprice_minimum"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/core/vm/vmcontext"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
	// cmath "github.com/celo-org/celo-blockchain/common/math"
)

// Used for EOA  Check
// var emptyCodeHash = crypto.Keccak256Hash(nil)

/*
The State Transitioning Model

A state transition is a change made when a transaction is applied to the current world state
The state transitioning model does all the necessary work to work out a valid new state root.

1) Nonce handling
2) Pre pay gas
3) Create a new state object if the recipient is \0*32
4) Value transfer
== If contract creation ==
  4a) Attempt to run transaction data
  4b) If valid, use result as code for the new state object
== end ==
5) Run Script section
6) Derive new state root
*/
type StateTransition struct {
	gp         *GasPool
	msg        Message
	gas        uint64
	gasPrice   *big.Int
	gasFeeCap  *big.Int
	gasTipCap  *big.Int
	initialGas uint64
	value      *big.Int
	data       []byte
	state      vm.StateDB
	evm        *vm.EVM

	// CELO Fields
	vmRunner vm.EVMRunner
	baseFee  *big.Int
	sysCtx   *SysContractCallCtx
}

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	To() *common.Address

	GasPrice() *big.Int
	GasFeeCap() *big.Int
	GasTipCap() *big.Int
	Gas() uint64
	Value() *big.Int

	Nonce() uint64
	IsFake() bool
	Data() []byte
	AccessList() types.AccessList

	// CELO Fields

	// FeeCurrency specifies the currency for gas and gateway fees.
	// nil correspond to Celo Gold (native currency).
	// All other values should correspond to ERC20 contract addresses extended to be compatible with gas payments.
	FeeCurrency() *common.Address
	GatewayFeeRecipient() *common.Address
	GatewayFee() *big.Int
	// Whether this transaction omitted the 3 Celo-only fields (FeeCurrency & co.)
	EthCompatible() bool
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
func IntrinsicGas(data []byte, accessList types.AccessList, isContractCreation bool, isHomestead, isEIP2028 bool, feeCurrency *common.Address, gasForAlternativeCurrency uint64) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if isContractCreation && isHomestead {
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
			return 0, ErrGasUintOverflow
		}
		gas += nz * nonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
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
	var baseFee *big.Int
	if evm.ChainConfig().IsEspresso(evm.Context.BlockNumber) {
		baseFee = sysCtx.GetGasPriceMinimum(msg.FeeCurrency())
	} else {
		baseFee, _ = gpm.GetGasPriceMinimum(vmRunner, msg.FeeCurrency())
	}

	return &StateTransition{
		gp:        gp,
		evm:       evm,
		msg:       msg,
		gasPrice:  msg.GasPrice(),
		gasFeeCap: msg.GasFeeCap(),
		gasTipCap: msg.GasTipCap(),
		value:     msg.Value(),
		data:      msg.Data(),
		state:     evm.StateDB,

		// CELO Fields
		vmRunner: vmRunner,
		baseFee:  baseFee,
		sysCtx:   sysCtx,
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
	return NewStateTransition(evm, msg, gp, vmRunner, sysCtx).TransitionDb()
}

// ApplyMessageWithoutGasPriceMinimum applies the given message with the gas price minimum
// set to zero. It's only for use in eth_call and eth_estimateGas, so that they can be used
// with gas price set to zero if the sender doesn't have funds to pay for gas.
// Returns the gas used (which does not include gas refunds) and an error if it failed.
func ApplyMessageWithoutGasPriceMinimum(evm *vm.EVM, msg Message, gp *GasPool, vmRunner vm.EVMRunner, sysCtx *SysContractCallCtx) (*ExecutionResult, error) {
	log.Trace("Applying state transition message without gas price minimum", "from", msg.From(), "nonce", msg.Nonce(), "to", msg.To(), "fee currency", msg.FeeCurrency(), "gateway fee recipient", msg.GatewayFeeRecipient(), "gateway fee", msg.GatewayFee(), "gas limit", msg.Gas(), "value", msg.Value(), "data", msg.Data())
	st := NewStateTransition(evm, msg, gp, vmRunner, sysCtx)
	st.baseFee = common.Big0
	return st.TransitionDb()
}

// to returns the recipient of the message.
func (st *StateTransition) to() common.Address {
	if st.msg == nil || st.msg.To() == nil /* contract creation */ {
		return common.Address{}
	}
	return *st.msg.To()
}

// buyGas checks whether accountOwner's balance can cover transaction fee.
//
// For native token(CELO) as feeCurrency:
//   - Pre-Espresso: it ensures balance >= GasPrice * gas + gatewayFee (1)
//   - Post-Espresso: it ensures balance >= GasFeeCap * gas + gatewayFee + value (2)
func (st *StateTransition) buyGas(espresso bool) error {
	mgval := new(big.Int).SetUint64(st.msg.Gas())
	mgval = mgval.Mul(mgval, st.gasPrice)
	balanceCheck := mgval
	if espresso {
		balanceCheck = new(big.Int).SetUint64(st.msg.Gas())
		balanceCheck = balanceCheck.Mul(balanceCheck, st.gasFeeCap)
		balanceCheck.Add(balanceCheck, st.value)
	}

	if st.msg.GatewayFeeRecipient() != nil {
		balanceCheck = balanceCheck.Add(balanceCheck, st.msg.GatewayFee())
		mgval = mgval.Add(mgval, st.msg.GatewayFee())
	}

	if have, want := st.state.GetBalance(st.msg.From()), balanceCheck; have.Cmp(want) < 0 {
		return fmt.Errorf("%w: address %v have %v want %v", ErrInsufficientFunds, st.msg.From().Hex(), have, want)
	}
	if err := st.gp.SubGas(st.msg.Gas()); err != nil {
		return err
	}
	st.gas += st.msg.Gas()

	st.initialGas = st.msg.Gas()
	st.state.SubBalance(st.msg.From(), mgval)
	return nil
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
		// // Make sure the sender is an EOA
		// if codeHash := st.state.GetCodeHash(st.msg.From()); codeHash != emptyCodeHash && codeHash != (common.Hash{}) {
		// 	return fmt.Errorf("%w: address %v, codehash: %s", ErrSenderNoEOA,
		// 		st.msg.From().Hex(), codeHash)
		// }
	}
	// Make sure that transaction gasFeeCap is greater than the baseFee (post espresso)
	if st.evm.ChainConfig().IsEspresso(st.evm.Context.BlockNumber) {
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
			if st.gasFeeCap.Cmp(st.baseFee) < 0 {
				return fmt.Errorf("%w: address %v, maxFeePerGas: %s baseFee: %s", ErrFeeCapTooLow,
					st.msg.From().Hex(), st.gasFeeCap, st.baseFee)
			}
		}
	} else { // Make sure this transaction's gas price >= baseFee (pre Espresso)
		if st.gasPrice.Cmp(st.baseFee) < 0 {
			log.Debug("Tx gas price is less than minimum", "minimum", st.baseFee, "price", st.gasPrice)
			return ErrGasPriceDoesNotExceedMinimum
		}
	}

	espresso := st.evm.ChainConfig().IsEspresso(st.evm.Context.BlockNumber)
	if err := st.checkCurrencyIsWhitelisted(espresso); err != nil {
		return err
	}

	if st.msg.FeeCurrency() != nil {
		return st.buyGasAlternativeCurrency(espresso)
	}
	return st.buyGas(espresso)
}

// TransitionDb will transition the state by applying the current message and
// returning the evm execution result with following fields.
//
// - used gas:
//      total gas used (including gas being refunded)
// - returndata:
//      the returned data from evm
// - concrete execution error:
//      various **EVM** error which aborts the execution,
//      e.g. ErrOutOfGas, ErrExecutionReverted
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
	// 1.1. the gas price meets the minimum gas price
	// 2. caller has enough balance to cover transaction fee(gaslimit * gasprice)
	// 3. the amount of gas required is available in the block
	// 4. the purchased gas is enough to cover intrinsic usage
	// 5. there is no overflow when calculating intrinsic gas
	// 6. caller has enough balance to cover asset transfer for **topmost** call

	// Clause 0
	if st.msg.EthCompatible() && !st.evm.ChainConfig().IsDonut(st.evm.Context.BlockNumber) {
		return nil, ErrEthCompatibleTransactionsNotSupported
	}
	if err := CheckEthCompatibility(st.msg); err != nil {
		return nil, err
	}

	if err := st.preCheck(); err != nil {
		return nil, err
	}
	msg := st.msg
	sender := vm.AccountRef(msg.From())
	homestead := st.evm.ChainConfig().IsHomestead(st.evm.Context.BlockNumber)
	istanbul := st.evm.ChainConfig().IsIstanbul(st.evm.Context.BlockNumber)
	espresso := st.evm.ChainConfig().IsEspresso(st.evm.Context.BlockNumber)
	contractCreation := msg.To() == nil

	gasForAlternativeCurrency := st.gasForAlternativeCurrency(msg.FeeCurrency(), espresso)
	// Check clauses 4-5, subtract intrinsic gas if everything is correct
	gas, err := IntrinsicGas(st.data, st.msg.AccessList(), contractCreation, homestead, istanbul, msg.FeeCurrency(), gasForAlternativeCurrency)
	if err != nil {
		return nil, err
	}
	if st.gas < gas {
		return nil, fmt.Errorf("%w: have %d, want %d", ErrIntrinsicGas, st.gas, gas)
	}
	st.gas -= gas

	// Check clause 6
	if msg.Value().Sign() > 0 && !st.evm.Context.CanTransfer(st.state, msg.From(), msg.Value()) {
		return nil, fmt.Errorf("%w: address %v", ErrInsufficientFundsForTransfer, msg.From().Hex())
	}

	// Set up the initial access list.
	if rules := st.evm.ChainConfig().Rules(st.evm.Context.BlockNumber); rules.IsEspresso {
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

	// CELO: This functionality has been extracted to distributeFees()

	// if !london {
	// 	// Before EIP-3529: refunds were capped to gasUsed / 2
	// 	st.refundGas(params.RefundQuotient)
	// } else {
	// 	// After EIP-3529: refunds are capped to gasUsed / 5
	// 	st.refundGas(params.RefundQuotientEIP3529)
	// }
	// effectiveTip := st.gasPrice
	// if london {
	// 	effectiveTip = cmath.BigMin(st.gasTipCap, new(big.Int).Sub(st.gasFeeCap, st.evm.Context.BaseFee))
	// }
	// st.state.AddBalance(st.evm.Context.Coinbase, new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed()), effectiveTip))

	if err = st.distributeTxFees(espresso); err != nil {
		return nil, err
	}

	return &ExecutionResult{
		UsedGas:    st.gasUsed(),
		Err:        vmerr,
		ReturnData: ret,
	}, nil
}

func (st *StateTransition) refundGas(refundQuotient uint64) {
	// Apply refund counter, capped to a refund quotient
	refund := st.gasUsed() / refundQuotient
	if refund > st.state.GetRefund() {
		refund = st.state.GetRefund()
	}
	st.gas += refund

	// CELO: This is being handled on distributedTxFees as there are more rules to it

	// // Return ETH for remaining gas, exchanged at the original rate.
	// remaining := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	// st.state.AddBalance(st.msg.From(), remaining)

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	st.gp.AddGas(st.gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gas
}

// distributeTxFees calculates the amounts and recipients of transaction fees and credits the accounts.
func (st *StateTransition) distributeTxFees(espresso bool) error {
	if !espresso {
		// Before EIP-3529: refunds were capped to gasUsed / 2
		st.refundGas(params.RefundQuotient)
	} else {
		// After EIP-3529: refunds are capped to gasUsed / 5
		st.refundGas(params.RefundQuotientEIP3529)
	}

	// st.gas = gas que hay q devolver (gas q queda despues de ejecutar todo y aplicar el aplicar)
	remainingGas := st.gas
	usedGas := st.initialGas - remainingGas

	toRefund := new(big.Int).Mul(new(big.Int).SetUint64(remainingGas), st.gasPrice)
	toPayTotal := new(big.Int).Mul(new(big.Int).SetUint64(usedGas), st.gasPrice)

	// Divide the transaction into a base (the minimum transaction fee) and tip (any extra, or min(max tip, feecap - GPM) if espresso).
	toPayBase := new(big.Int).Mul(new(big.Int).SetUint64(usedGas), st.baseFee)
	// No need to do effectiveTip calculation, because st.gasPrice == effectiveGasPrice, and effectiveTip = effectiveGasPrice - baseTxFee
	toPayTip := new(big.Int).Sub(toPayTotal, toPayBase)

	gatewayFeeRecipient := st.msg.GatewayFeeRecipient()
	if gatewayFeeRecipient == nil {
		gatewayFeeRecipient = &common.ZeroAddress
	}

	governanceAddress, err := st.getGovernanceAddress()
	if err != nil {
		return err
	}
	if governanceAddress == common.ZeroAddress {
		log.Trace("Cannot credit gas fee to community fund: refunding fee to sender", "error", err, "fee", toPayBase)
		toRefund.Add(toRefund, toPayBase)
		toPayBase = new(big.Int)
	}

	log.Trace("distributeTxFees", "from", st.msg.From(), "refund", toRefund, "feeCurrency", st.msg.FeeCurrency(),
		"gatewayFeeRecipient", *gatewayFeeRecipient, "gatewayFee", st.msg.GatewayFee(),
		"coinbaseFeeRecipient", st.evm.Context.Coinbase, "coinbaseFee", toPayTip,
		"comunityFundRecipient", governanceAddress, "communityFundFee", toPayBase)

	if st.msg.FeeCurrency() == nil {
		if gatewayFeeRecipient != &common.ZeroAddress {
			st.state.AddBalance(*gatewayFeeRecipient, st.msg.GatewayFee())
		}
		if governanceAddress != common.ZeroAddress {
			st.state.AddBalance(governanceAddress, toPayBase)
		}
		st.state.AddBalance(st.evm.Context.Coinbase, toPayTip)
		st.state.AddBalance(st.msg.From(), toRefund)
	} else {
		if err = erc20gas.CreditFees(st.evm, st.msg.From(), st.evm.Context.Coinbase, gatewayFeeRecipient, governanceAddress, toRefund, toPayTip, st.msg.GatewayFee(), toPayBase, st.msg.FeeCurrency()); err != nil {
			log.Error("Error crediting", "from", st.msg.From(), "coinbase", st.evm.Context.Coinbase, "gateway", gatewayFeeRecipient, "fund", governanceAddress)
			return err
		}

	}
	return nil
}

func (st *StateTransition) getGovernanceAddress() (common.Address, error) {
	// Run only primary evm.Call() with tracer
	if st.evm.GetDebug() {
		st.evm.SetDebug(false)
		defer func() { st.evm.SetDebug(true) }()
	}
	caller := &vmcontext.SharedEVMRunner{EVM: st.evm}
	governanceAddress, err := contracts.GetRegisteredAddress(caller, config.GovernanceRegistryId)
	if err != nil {
		if err != contracts.ErrSmartContractNotDeployed && err != contracts.ErrRegistryContractNotDeployed {
			return common.ZeroAddress, err
		}
		return common.ZeroAddress, nil
	}
	return governanceAddress, nil
}

// byGasAlternativeCurrency checks whether accountOwner's balance can cover transaction fee.
// For non-native tokens(cUSD, cEUR, ...) as feeCurrency:
//   - Pre-Espresso: it ensures balance > GasPrice * gas + gatewayFee (1)
//   - Post-Espresso: it ensures balance >= GasFeeCap * gas + gatewayFee (2) (CIP-45)
func (st *StateTransition) buyGasAlternativeCurrency(espresso bool) error {
	balance, err := currency.GetBalanceOf(st.vmRunner, st.msg.From(), *st.msg.FeeCurrency())
	if err != nil {
		return err
	}

	mgval := new(big.Int).SetUint64(st.msg.Gas())
	mgval = mgval.Mul(mgval, st.gasPrice)
	balanceCheck := mgval
	if espresso {
		balanceCheck = new(big.Int).SetUint64(st.msg.Gas())
		balanceCheck = balanceCheck.Mul(balanceCheck, st.gasFeeCap)
		// We don't add st.value because it's on a different currency
	}

	if st.msg.GatewayFeeRecipient() != nil {
		balanceCheck = balanceCheck.Add(balanceCheck, st.msg.GatewayFee())
		mgval = mgval.Add(mgval, st.msg.GatewayFee())
	}

	var hasEnoughBalance bool
	if espresso {
		hasEnoughBalance = balance.Cmp(balanceCheck) >= 0 // (2)
	} else {
		hasEnoughBalance = balance.Cmp(balanceCheck) > 0 // (1)
	}
	if !hasEnoughBalance {
		return fmt.Errorf("%w: address %v have %v want %v", ErrInsufficientFunds, st.msg.From().Hex(), balance, balanceCheck)
	}

	if err := st.gp.SubGas(st.msg.Gas()); err != nil {
		return err
	}
	st.gas += st.msg.Gas()

	st.initialGas = st.msg.Gas()
	// st.state.SubBalance(st.msg.From(), mgval) // We don't do this since it's on a different currency
	return erc20gas.DebitFees(st.evm, st.msg.From(), mgval, st.msg.FeeCurrency())
}

func CheckEthCompatibility(msg Message) error {
	if msg.EthCompatible() && !(msg.FeeCurrency() == nil && msg.GatewayFeeRecipient() == nil && msg.GatewayFee().Sign() == 0) {
		return types.ErrEthCompatibleTransactionIsntCompatible
	}
	return nil
}

func (st *StateTransition) gasForAlternativeCurrency(feeCurrency *common.Address, espresso bool) uint64 {
	gasForAlternativeCurrency := uint64(0)
	// If the fee currency is nil, do not retrieve the intrinsic gas adjustment from the chain state, as it will not be used.
	if feeCurrency != nil {
		if espresso {
			gasForAlternativeCurrency = st.sysCtx.GetIntrinsicGasForAlternativeFeeCurrency()
		} else {
			gasForAlternativeCurrency = blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrencyOrDefault(st.vmRunner)
		}
	}
	return gasForAlternativeCurrency
}

func (st *StateTransition) checkCurrencyIsWhitelisted(espresso bool) error {
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
	return nil
}
