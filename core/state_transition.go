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
	"errors"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/contract_comm/blockchain_parameters"
	"github.com/ethereum/go-ethereum/contract_comm/currency"
	commerrs "github.com/ethereum/go-ethereum/contract_comm/errors"
	gpm "github.com/ethereum/go-ethereum/contract_comm/gasprice_minimum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var (
	ErrGasPriceDoesNotExceedMinimum = errors.New("gasprice is less than gas price minimum")

	errInsufficientBalanceForFees = errors.New("insufficient balance to pay for fees")
	errNonWhitelistedFeeCurrency  = errors.New("non-whitelisted fee currency address")
)

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
	gp              *GasPool
	msg             Message
	gas             uint64
	gasPrice        *big.Int
	initialGas      uint64
	value           *big.Int
	data            []byte
	state           vm.StateDB
	evm             *vm.EVM
	gasPriceMinimum *big.Int
}

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	//FromFrontier() (common.Address, error)
	To() *common.Address

	GasPrice() *big.Int
	Gas() uint64

	// FeeCurrency specifies the currency for gas and gateway fees.
	// nil correspond to Celo Gold (native currency).
	// All other values should correspond to ERC20 contract addresses extended to be compatible with gas payments.
	FeeCurrency() *common.Address
	GatewayFeeRecipient() *common.Address
	GatewayFee() *big.Int
	Value() *big.Int

	Nonce() uint64
	CheckNonce() bool
	Data() []byte
}

// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
func IntrinsicGas(data []byte, contractCreation bool, header *types.Header, state vm.StateDB, feeCurrency *common.Address) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if contractCreation {
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
		nonZeroGas := params.TxDataNonZeroGasEIP2028

		if (math.MaxUint64-gas)/nonZeroGas < nz {
			log.Debug("IntrinsicGas", "out of gas")
			return 0, vm.ErrOutOfGas
		}
		gas += nz * nonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			log.Debug("IntrinsicGas", "out of gas")
			return 0, vm.ErrOutOfGas
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
		gas += blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrency(header, state)
	}

	return gas, nil
}

// NewStateTransition initialises and returns a new state transition object.
func NewStateTransition(evm *vm.EVM, msg Message, gp *GasPool) *StateTransition {
	gasPriceMinimum, _ := gpm.GetGasPriceMinimum(msg.FeeCurrency(), evm.GetHeader(), evm.GetStateDB())

	return &StateTransition{
		gp:              gp,
		evm:             evm,
		msg:             msg,
		gasPrice:        msg.GasPrice(),
		value:           msg.Value(),
		data:            msg.Data(),
		state:           evm.StateDB,
		gasPriceMinimum: gasPriceMinimum,
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
//
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
func ApplyMessage(evm *vm.EVM, msg Message, gp *GasPool) ([]byte, uint64, bool, error) {
	log.Trace("Applying state transition message", "from", msg.From(), "nonce", msg.Nonce(), "to", msg.To(), "gas price", msg.GasPrice(), "fee currency", msg.FeeCurrency(), "gateway fee recipient", msg.GatewayFeeRecipient(), "gateway fee", msg.GatewayFee(), "gas", msg.Gas(), "value", msg.Value(), "data", msg.Data())
	return NewStateTransition(evm, msg, gp).TransitionDb()
}

// to returns the recipient of the message.
func (st *StateTransition) to() common.Address {
	if st.msg == nil || st.msg.To() == nil /* contract creation */ {
		return common.Address{}
	}
	return *st.msg.To()
}

func (st *StateTransition) useGas(amount uint64) error {
	if st.gas < amount {
		return vm.ErrOutOfGas
	}
	st.gas -= amount

	return nil
}

// payFees deducts gas and gateway fees from sender balance and adds the purchased amount of gas to the state.
func (st *StateTransition) payFees() error {
	feeVal := new(big.Int).Mul(new(big.Int).SetUint64(st.msg.Gas()), st.gasPrice)

	// If GatewayFeeRecipient is unspecified, the gateway fee value is ignore and the sender is not charged.
	if st.msg.GatewayFeeRecipient() != nil {
		feeVal.Add(feeVal, st.msg.GatewayFee())
	}

	if st.msg.FeeCurrency() != nil && (!currency.IsWhitelisted(*st.msg.FeeCurrency(), st.evm.GetHeader(), st.evm.GetStateDB())) {
		log.Trace("Fee currency not whitelisted", "fee currency address", st.msg.FeeCurrency())
		return errNonWhitelistedFeeCurrency
	}

	if !st.canPayFee(st.msg.From(), feeVal, st.msg.FeeCurrency()) {
		return errInsufficientBalanceForFees
	}
	if err := st.gp.SubGas(st.msg.Gas()); err != nil {
		return err
	}

	st.initialGas = st.msg.Gas()
	st.gas += st.msg.Gas()
	err := st.debitFee(st.msg.From(), feeVal, st.msg.FeeCurrency())
	return err
}

func (st *StateTransition) canPayFee(accountOwner common.Address, fee *big.Int, feeCurrency *common.Address) bool {
	if feeCurrency == nil {
		return st.state.GetBalance(accountOwner).Cmp(fee) >= 0
	}
	balanceOf, gasUsed, err := currency.GetBalanceOf(accountOwner, *feeCurrency, params.MaxGasToReadErc20Balance, st.evm.GetHeader(), st.evm.GetStateDB())
	log.Debug("balanceOf called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)

	if err != nil {
		return false
	}
	return balanceOf.Cmp(fee) > 0
}

func (st *StateTransition) debitFrom(address common.Address, amount *big.Int, feeCurrency *common.Address) error {
	if amount.Cmp(big.NewInt(0)) == 0 {
		return nil
	}
	evm := st.evm
	// Function is "debitFrom(address from, uint256 value)"
	// selector is first 4 bytes of keccak256 of "debitFrom(address,uint256)"
	// Source:
	// pip3 install pyethereum
	// python3 -c 'from ethereum.utils import sha3; print(sha3("debitFrom(address,uint256)")[0:4].hex())'
	functionSelector := hexutil.MustDecode("0x362a5f80")
	transactionData := common.GetEncodedAbi(functionSelector, [][]byte{common.AddressToAbi(address), common.AmountToAbi(amount)})

	rootCaller := vm.AccountRef(common.HexToAddress("0x0"))
	// The caller was already charged for the cost of this operation via IntrinsicGas.
	_, leftoverGas, err := evm.Call(rootCaller, *feeCurrency, transactionData, params.MaxGasForDebitFromTransactions, big.NewInt(0))
	gasUsed := params.MaxGasForDebitFromTransactions - leftoverGas
	log.Debug("debitFrom called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)
	return err
}

func (st *StateTransition) creditTo(address common.Address, amount *big.Int, feeCurrency *common.Address) error {
	if amount.Cmp(big.NewInt(0)) == 0 {
		return nil
	}
	evm := st.evm
	// Function is "creditTo(address from, uint256 value)"
	// selector is first 4 bytes of keccak256 of "creditTo(address,uint256)"
	// Source:
	// pip3 install pyethereum
	// python3 -c 'from ethereum.utils import sha3; print(sha3("creditTo(address,uint256)")[0:4].hex())'
	functionSelector := hexutil.MustDecode("0x9951b90c")
	transactionData := common.GetEncodedAbi(functionSelector, [][]byte{common.AddressToAbi(address), common.AmountToAbi(amount)})
	rootCaller := vm.AccountRef(common.HexToAddress("0x0"))
	// The caller was already charged for the cost of this operation via IntrinsicGas.
	_, leftoverGas, err := evm.Call(rootCaller, *feeCurrency, transactionData, params.MaxGasForCreditToTransactions, big.NewInt(0))
	gasUsed := params.MaxGasForCreditToTransactions - leftoverGas
	log.Debug("creditTo called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)
	return err
}

func (st *StateTransition) debitFee(from common.Address, amount *big.Int, feeCurrency *common.Address) (err error) {
	log.Trace("Debiting fee", "from", from, "amount", amount, "feeCurrency", feeCurrency)
	// native currency
	if feeCurrency == nil {
		st.state.SubBalance(from, amount)
		return nil
	} else {
		return st.debitFrom(from, amount, feeCurrency)
	}
}

func (st *StateTransition) creditFee(to common.Address, amount *big.Int, feeCurrency *common.Address) (err error) {
	// native currency
	if feeCurrency == nil {
		st.state.AddBalance(to, amount)
		return nil
	} else {
		return st.creditTo(to, amount, feeCurrency)
	}
}

func (st *StateTransition) preCheck() error {
	// Make sure this transaction's nonce is correct.
	if st.msg.CheckNonce() {
		nonce := st.state.GetNonce(st.msg.From())
		if nonce < st.msg.Nonce() {
			return ErrNonceTooHigh
		} else if nonce > st.msg.Nonce() {
			return ErrNonceTooLow
		}
	}

	// Make sure this transaction's gas price is valid.
	if st.gasPrice.Cmp(st.gasPriceMinimum) < 0 {
		log.Error("Tx gas price is less than minimum", "minimum", st.gasPriceMinimum, "price", st.gasPrice)
		return ErrGasPriceDoesNotExceedMinimum
	}

	return nil
}

// TransitionDb will transition the state by applying the current message and
// returning the result including the used gas. It returns an error if failed.
// An error indicates a consensus issue.
func (st *StateTransition) TransitionDb() (ret []byte, usedGas uint64, failed bool, err error) {
	// Check that the transaction nonce is correct
	if err = st.preCheck(); err != nil {
		log.Error("Transaction failed precheck", "err", err)
		return
	}
	msg := st.msg
	sender := vm.AccountRef(msg.From())
	contractCreation := msg.To() == nil

	// Calculate intrinsic gas.
	gas, err := IntrinsicGas(st.data, contractCreation, st.evm.GetHeader(), st.state, msg.FeeCurrency())
	if err != nil {
		return nil, 0, false, err
	}

	// If the intrinsic gas is more than provided in the tx, return without failing.
	if gas > st.msg.Gas() {
		log.Error("Transaction failed provide intrinsic gas", "err", err,
			"gas required", gas,
			"gas provided", st.msg.Gas(),
			"fee currency", st.msg.FeeCurrency())
		return nil, 0, false, vm.ErrOutOfGas
	}

	err = st.payFees()
	if err != nil {
		log.Error("Transaction failed to buy gas", "err", err, "gas", gas)
		return nil, 0, false, err
	}

	if err = st.useGas(gas); err != nil {
		log.Error("Transaction failed to use gas", "err", err, "gas", gas)
		return nil, 0, false, err
	}

	var (
		evm = st.evm
		// vm errors do not effect consensus and are therefore
		// not assigned to err, except for insufficient balance
		// error.
		vmerr error
	)
	if contractCreation {
		ret, _, st.gas, vmerr = evm.Create(sender, st.data, st.gas, st.value)
	} else {
		// Increment the nonce for the next transaction
		st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
		ret, st.gas, vmerr = evm.Call(sender, st.to(), st.data, st.gas, st.value)
	}
	if vmerr != nil {
		log.Debug("VM returned with error", "err", vmerr)
		// The only possible consensus-error would be if there wasn't
		// sufficient balance to make the transfer happen. The first
		// balance transfer may never fail.
		if vmerr == vm.ErrInsufficientBalance {
			return nil, 0, false, vmerr
		}
	}

	st.refundGas()

	err = st.distributeTxFees()
	if err != nil {
		return nil, 0, false, err
	}

	return ret, st.gasUsed(), vmerr != nil, nil
}

// distributeTxFees calculates the amounts and recipients of transaction fees and credits the accounts.
func (st *StateTransition) distributeTxFees() error {
	// Determine the refund and transaction fee to be distributed.
	refund := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	gasUsed := new(big.Int).SetUint64(st.gasUsed())
	totalTxFee := new(big.Int).Mul(gasUsed, st.gasPrice)

	// Divide the transaction into a base (the minimum transaction fee) and tip (any extra).
	baseTxFee := new(big.Int).Mul(gasUsed, st.gasPriceMinimum)
	tipTxFee := new(big.Int).Sub(totalTxFee, baseTxFee)

	// Pay gateway fee to the specified recipient.
	if st.msg.GatewayFeeRecipient() != nil {
		log.Trace("Crediting gateway fee", "recipient", *st.msg.GatewayFeeRecipient(), "amount", st.msg.GatewayFee(), "feeCurrency", st.msg.FeeCurrency())
		if err := st.creditFee(*st.msg.GatewayFeeRecipient(), st.msg.GatewayFee(), st.msg.FeeCurrency()); err != nil {
			log.Error("Failed to credit gateway fee", "err", err)
			return err
		}
	}

	log.Trace("Crediting gas fee tip", "recipient", st.evm.Coinbase, "amount", tipTxFee, "feeCurrency", st.msg.FeeCurrency())
	if err := st.creditFee(st.evm.Coinbase, tipTxFee, st.msg.FeeCurrency()); err != nil {
		return err
	}

	// Send the base of the transaction fee to the infrastructure fund.
	governanceAddress, err := vm.GetRegisteredAddressWithEvm(params.GovernanceRegistryId, st.evm)
	if err != nil {
		if err != commerrs.ErrSmartContractNotDeployed && err != commerrs.ErrRegistryContractNotDeployed {
			return err
		}
		log.Trace("Cannot credit gas fee to infrastructure fund: refunding fee to sender", "error", err, "fee", baseTxFee)
		refund.Add(refund, baseTxFee)
	} else {
		log.Trace("Crediting gas fee tip", "recipient", *governanceAddress, "amount", baseTxFee, "feeCurrency", st.msg.FeeCurrency())
		if err = st.creditFee(*governanceAddress, baseTxFee, st.msg.FeeCurrency()); err != nil {
			return err
		}
	}

	log.Trace("Crediting refund", "recipient", st.msg.From(), "amount", refund, "feeCurrency", st.msg.FeeCurrency())
	err = st.creditFee(st.msg.From(), refund, st.msg.FeeCurrency())
	if err != nil {
		log.Error("Failed to refund gas", "err", err)
		return err
	}
	return nil
}

// refundGas adds unused gas back the state transition and gas pool.
func (st *StateTransition) refundGas() {
	refund := st.state.GetRefund()
	// Apply refund counter, capped to half of the used gas.
	if refund > st.gasUsed()/2 {
		refund = st.gasUsed() / 2
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
