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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"math"
	"math/big"
)

var (
	errInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
	// This is the amount of gas a single debitFrom or creditTo request can use.
	// This prevents arbitrary computation to be performed in these functions.
	// During testing, I noticed that a single invocation of debit gas consumes 7649 gas
	// and a single invocation of creditGas consumes 7943 gas.
	maxGasForDebitAndCreditTransactions uint64 = 10 * 1000
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
	gp         *GasPool
	msg        Message
	gas        uint64
	gasPrice   *big.Int
	initialGas uint64
	value      *big.Int
	data       []byte
	state      vm.StateDB
	evm        *vm.EVM
}

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	//FromFrontier() (common.Address, error)
	To() *common.Address

	GasPrice() *big.Int
	Gas() uint64
	// nil correspond to Celo Gold (native currency).
	// All other values should correspond to ERC20 contract addresses extended to be compatible with gas payments.
	GasCurrency() *common.Address
	Value() *big.Int

	Nonce() uint64
	CheckNonce() bool
	Data() []byte
}

// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
func IntrinsicGas(data []byte, contractCreation, homestead bool) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if contractCreation && homestead {
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
		if (math.MaxUint64-gas)/params.TxDataNonZeroGas < nz {
			log.Debug("IntrinsicGas", "out of gas")
			return 0, vm.ErrOutOfGas
		}
		gas += nz * params.TxDataNonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			log.Debug("IntrinsicGas", "out of gas")
			return 0, vm.ErrOutOfGas
		}
		gas += z * params.TxDataZeroGas
	}
	return gas, nil
}

// NewStateTransition initialises and returns a new state transition object.
func NewStateTransition(evm *vm.EVM, msg Message, gp *GasPool) *StateTransition {
	return &StateTransition{
		gp:       gp,
		evm:      evm,
		msg:      msg,
		gasPrice: msg.GasPrice(),
		value:    msg.Value(),
		data:     msg.Data(),
		state:    evm.StateDB,
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

func (st *StateTransition) buyGas() error {
	mgval := new(big.Int).Mul(new(big.Int).SetUint64(st.msg.Gas()), st.gasPrice)
	hasSufficientGas, gasUsed := st.canBuyGas(st.msg.From(), mgval, st.msg.GasCurrency())
	if !hasSufficientGas {
		return errInsufficientBalanceForGas
	}
	if err := st.gp.SubGas(st.msg.Gas()); err != nil {
		return err
	}
	st.gas += st.msg.Gas()
	st.initialGas = st.msg.Gas()
	gasCurrency := st.msg.GasCurrency()
	// Charge user for the gas used to determine gas balance.
	// It is 0 for native gas currency, and we charge maxGasForDebitAndCreditTransactions for safety and give
	// that as the gas allowance for the debit and the credit transactions.
	chargeForGasWithdrawal := new(big.Int).SetUint64(maxGasForDebitAndCreditTransactions)
	mgval = new(big.Int).Add(mgval, chargeForGasWithdrawal.Mul(chargeForGasWithdrawal, st.gasPrice))
	amount := new(big.Int).Add(mgval, new(big.Int).SetUint64(gasUsed))
	err := st.debitGas(amount, gasCurrency)
	if err != nil {
		return err
	}
	return nil
}

func (st *StateTransition) canBuyGas(
	accountOwner common.Address,
	gasNeeded *big.Int,
	gasCurrency *common.Address) (hasSufficientGas bool, gasUsed uint64) {
	if gasCurrency == nil {
		return st.state.GetBalance(accountOwner).Cmp(gasNeeded) > 0, 0
	}
	balanceOf, gasUsed, err := st.getBalanceOf(accountOwner, gasCurrency)
	log.Debug("getBalanceOf balance", "account", accountOwner.Hash(), "Balance", balanceOf.String(),
		"gas used", gasUsed, "error", err)
	if err != nil {
		return false, gasUsed
	}
	return balanceOf.Cmp(gasNeeded) > 0, gasUsed
}

type ZeroAddress int64

func (ZeroAddress) Address() common.Address {
	var address common.Address
	// Not required since address is, by default, initialized to 0
	// copy(address[:], "0000000000000000000000000000000000000000")
	return address
}

// contractAddress must have a function  with following signature
// "function balanceOf(address _owner) public view returns (uint256)"
func (st *StateTransition) getBalanceOf(accountOwner common.Address, contractAddress *common.Address) (
	balance *big.Int, gasUsed uint64, err error) {
	evm := st.evm
	functionSelector := getBalanceOfFunctionSelector()
	transactionData := getEncodedAbiWithOneArg(functionSelector, addressToAbi(accountOwner))
	anyCaller := ZeroAddress(0)  // any caller will work
	log.Trace("getBalanceOf", "caller", anyCaller, "customTokenContractAddress",
		*contractAddress, "gas", st.gas, "transactionData", hexutil.Encode(transactionData))
	ret, leftoverGas, err := evm.StaticCall(anyCaller, *contractAddress, transactionData, st.gas + st.msg.Gas())
	gasUsed = st.gas + st.msg.Gas() - leftoverGas
	if err != nil {
		log.Debug("getBalanceOf error occurred", "Error", err)
		return nil, gasUsed, err
	}
	result := big.NewInt(0)
	result.SetBytes(ret)
	log.Trace("getBalanceOf balance", "account", accountOwner.Hash(), "Balance", result.String(),
		"gas used", gasUsed)
	return result, gasUsed, nil
}

func (st *StateTransition) debitOrCreditErc20Balance(
	functionSelector []byte, address common.Address, amount *big.Int,
	gasCurrency *common.Address, logTag string) (err error) {
	if amount.Cmp(big.NewInt(0)) == 0 {
		log.Trace(logTag + " successful: nothing to debit or credit")
		return nil
	}

	log.Debug(logTag, "amount", amount, "gasCurrency", gasCurrency.String())
	evm := st.evm
	transactionData := getEncodedAbiWithTwoArgs(functionSelector, addressToAbi(address), amountToAbi(amount))

	rootCaller := ZeroAddress(0)
	maxGasForCall := st.gas
	// Limit the gas used by these calls to prevent a gas stealing attack.
	if maxGasForCall > maxGasForDebitAndCreditTransactions {
		maxGasForCall = maxGasForDebitAndCreditTransactions
	}
	log.Trace(logTag, "rootCaller", rootCaller, "customTokenContractAddress",
		*gasCurrency, "gas", maxGasForCall, "value", 0, "transactionData", hexutil.Encode(transactionData))
	// We will not charge the user directly for this call. The caller should charge the payer, which might
	// or might not be this user, for "maxGasForDebitAndCreditTransactions" amount of gas.
	ret, leftoverGas, err := evm.Call(
		rootCaller, *gasCurrency, transactionData, maxGasForCall, big.NewInt(0))
	if err != nil {
		log.Debug(logTag + " failed", "ret", hexutil.Encode(ret), "leftoverGas", leftoverGas, "err", err)
		return err
	}

	log.Debug(logTag + " successful", "ret", hexutil.Encode(ret), "leftoverGas", leftoverGas)
	return nil
}


func (st *StateTransition) debitGas(amount *big.Int, gasCurrency *common.Address) (err error) {
	// native currency
	if gasCurrency == nil {
		st.state.SubBalance(st.msg.From(), amount)
		return nil
	}

	return st.debitOrCreditErc20Balance(
		getDebitFromFunctionSelector(),
		st.msg.From(),
		amount,
		gasCurrency,
		"debitGas",
		)
}

func (st *StateTransition) creditGas(to common.Address, amount *big.Int, gasCurrency *common.Address) (err error) {
	// native currency
	if gasCurrency == nil {
		st.state.AddBalance(to, new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed()), st.gasPrice))
		return nil
	}

	return st.debitOrCreditErc20Balance(
		getCreditToFunctionSelector(),
		to,
		amount,
		gasCurrency,
		"creditGas")
}

func getDebitFromFunctionSelector() []byte {
	// Function is "debitFrom(address from, uint256 value)"
	// selector is first 4 bytes of keccak256 of "debitFrom(address,uint256)"
	// Source:
	// pip3 install pyethereum
	// python3 -c 'from ethereum.utils import sha3; print(sha3("debitFrom(address,uint256)")[0:4].hex())'
	return hexutil.MustDecode("0x362a5f80")
}

func getCreditToFunctionSelector() []byte {
	// Function is "creditTo(address from, uint256 value)"
	// selector is first 4 bytes of keccak256 of "creditTo(address,uint256)"
	// Source:
	// pip3 install pyethereum
	// python3 -c 'from ethereum.utils import sha3; print(sha3("creditTo(address,uint256)")[0:4].hex())'
	return hexutil.MustDecode("0x9951b90c")
}

func getBalanceOfFunctionSelector() []byte {
	// Function is "balanceOf(address _owner)"
	// selector is first 4 bytes of keccak256 of "balanceOf(address)"
	// Source:
	// pip3 install pyethereum
	// python3 -c 'from ethereum.utils import sha3; print(sha3("balanceOf(address)")[0:4].hex())'
	return hexutil.MustDecode("0x70a08231")
}


func addressToAbi(address common.Address) []byte {
	// Now convert address and amount to 32 byte (256-bit) chunks.
	return common.LeftPadBytes(address.Bytes(), 32)
}

func amountToAbi(amount *big.Int) []byte {
	// Get amount as 32 bytes
	return common.LeftPadBytes(amount.Bytes(), 32)
}

// Generates ABI for a given method and its argument.
func getEncodedAbiWithOneArg(methodSelector []byte, var1Abi []byte) []byte {
	encodedAbi := make([]byte, len(methodSelector)+len(var1Abi))
	copy(encodedAbi[0:len(methodSelector)], methodSelector[:])
	copy(encodedAbi[len(methodSelector):len(methodSelector)+len(var1Abi)], var1Abi[:])
	return encodedAbi
}

// Generates ABI for a given method and its arguments.
func getEncodedAbiWithTwoArgs(methodSelector []byte, var1Abi []byte, var2Abi []byte) []byte {
	encodedAbi := make([]byte, len(methodSelector)+len(var1Abi)+len(var2Abi))
	copy(encodedAbi[0:len(methodSelector)], methodSelector[:])
	copy(encodedAbi[len(methodSelector):len(methodSelector)+len(var1Abi)], var1Abi[:])
	copy(encodedAbi[len(methodSelector)+len(var1Abi):], var2Abi[:])
	return encodedAbi
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
	return st.buyGas()
}

// TransitionDb will transition the state by applying the current message and
// returning the result including the used gas. It returns an error if failed.
// An error indicates a consensus issue.
func (st *StateTransition) TransitionDb() (ret []byte, usedGas uint64, failed bool, err error) {
	if err = st.preCheck(); err != nil {
		return
	}
	msg := st.msg
	sender := vm.AccountRef(msg.From())
	homestead := st.evm.ChainConfig().IsHomestead(st.evm.BlockNumber)
	contractCreation := msg.To() == nil

	// Pay intrinsic gas
	gas, err := IntrinsicGas(st.data, contractCreation, homestead)
	if err != nil {
		return nil, 0, false, err
	}
	if err = st.useGas(gas); err != nil {
		return nil, 0, false, err
	}

	var (
		evm = st.evm
		// vm errors do not effect consensus and are therefor
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
	// Refund unused gas to sender
	st.refundGas()
	// Pay gas fee to Coinbase chosen by the miner
	st.creditGas(
		st.evm.Coinbase,
		new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed()), st.gasPrice),
		st.msg.GasCurrency())

	return ret, st.gasUsed(), vmerr != nil, err
}

func (st *StateTransition) refundGas() {
	// Apply refund counter, capped to half of the used gas.
	refund := st.gasUsed() / 2
	if refund > st.state.GetRefund() {
		refund = st.state.GetRefund()
	}
	// We charge the user for cost associated with gas refund to their account as well as the
	// cost associated with paying the mining fee to Coinbase account.
	refund -= 2 * maxGasForDebitAndCreditTransactions
	if refund < 0 {
		log.Info("refundGas not possible since refund amount is too small",
			"refund", refund, "maxGasForDebitAndCreditTransactions", maxGasForDebitAndCreditTransactions)
		return
	}

	st.gas += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	st.creditGas(st.msg.From(), remaining, st.msg.GasCurrency())
	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	st.gp.AddGas(st.gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gas
}
