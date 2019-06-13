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
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/eth/gasprice"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var (
	errInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
	errNonWhitelistedGasCurrency = errors.New("non-whitelisted gas currency address")

	errGasPriceDoesNotExceedFloor = errors.New("gasprice does not exceed floor")

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
	gcWl       *GasCurrencyWhitelist
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
	GasFeeRecipient() *common.Address
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
func NewStateTransition(evm *vm.EVM, msg Message, gp *GasPool, gcWl *GasCurrencyWhitelist) *StateTransition {
	return &StateTransition{
		gp:       gp,
		evm:      evm,
		msg:      msg,
		gasPrice: msg.GasPrice(),
		value:    msg.Value(),
		data:     msg.Data(),
		state:    evm.StateDB,
		gcWl:     gcWl,
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
//
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
func ApplyMessage(evm *vm.EVM, msg Message, gp *GasPool, gcWl *GasCurrencyWhitelist) ([]byte, uint64, bool, error) {
	return NewStateTransition(evm, msg, gp, gcWl).TransitionDb()
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

	if st.msg.GasCurrency() != nil && (st.gcWl == nil || !st.gcWl.IsWhitelisted(*st.msg.GasCurrency())) {
		log.Trace("Gas currency not whitelisted",
			"gas currency address", st.msg.GasCurrency())
		return errNonWhitelistedGasCurrency
	}
	// gasConsumedToDetermineBalance = Charge to determine user's balance in native or non-native currency
	hasSufficientGas, gasConsumedToDetermineBalance := st.canBuyGas(st.msg.From(), mgval, st.msg.GasCurrency())
	if !hasSufficientGas {
		return errInsufficientBalanceForGas
	}
	if err := st.gp.SubGas(st.msg.Gas()); err != nil {
		return err
	}
	st.gas += st.msg.Gas()
	gasCurrency := st.msg.GasCurrency()

	// Users are not charged an extra fee for transactions where gas is paid in native currency.
	// Users are charged an extra fee for non-native gas currency transactions for the overhead
	// of the EVM transactions.
	if gasCurrency != nil {
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
		gasToDebitAndCreditGas := 3 * maxGasForDebitAndCreditTransactions
		upfrontGasCharges := gasConsumedToDetermineBalance + gasToDebitAndCreditGas
		if st.gas < upfrontGasCharges {
			log.Debug("Gas too low during buy gas",
				"gas left", st.gas,
				"gasConsumedToDetermineBalance", gasConsumedToDetermineBalance,
				"maxGasForDebitAndCreditTransactions", gasToDebitAndCreditGas)
			return errInsufficientBalanceForGas
		}
		st.gas -= upfrontGasCharges
		log.Trace("buyGas before debitGas", "upfrontGasCharges", upfrontGasCharges, "available gas", st.gas, "initial gas", st.initialGas, "gasCurrency", gasCurrency)
	}
	st.initialGas = st.msg.Gas()
	err := st.debitGas(mgval, gasCurrency)
	return err
}

func (st *StateTransition) canBuyGas(
	accountOwner common.Address,
	gasNeeded *big.Int,
	gasCurrency *common.Address) (hasSufficientGas bool, gasUsed uint64) {
	if gasCurrency == nil {
		return st.state.GetBalance(accountOwner).Cmp(gasNeeded) > 0, 0
	}
	balanceOf, gasUsed, err := GetBalanceOf(accountOwner, *gasCurrency, nil, st.evm, st.gas+st.msg.Gas())
	log.Debug("getBalanceOf balance", "account", accountOwner.Hash(), "Balance", balanceOf.String(),
		"gas used", gasUsed, "error", err)
	if err != nil {
		return false, gasUsed
	}
	return balanceOf.Cmp(gasNeeded) > 0, gasUsed
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
	transactionData := common.GetEncodedAbi(functionSelector, [][]byte{common.AddressToAbi(address), common.AmountToAbi(amount)})

	rootCaller := vm.AccountRef(common.HexToAddress("0x0"))
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
		log.Debug(logTag+" failed", "ret", hexutil.Encode(ret), "leftoverGas", leftoverGas, "err", err)
		return err
	}

	log.Debug(logTag+" successful", "ret", hexutil.Encode(ret), "leftoverGas", leftoverGas)
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
		st.state.AddBalance(to, amount)
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

//TODO(jarmg 6/13) Find a better way to do this please 
type gasPriceFloorEvm struct {
  zeroCaller vm.ContractRef
  evm *vm.EVM
}

func (gEvm gasPriceFloorEvm) MakeStaticCall(
  scAddress common.Address,
  abi abi.ABI,
  funcName string,
  args []interface{},
  returnObj interface{},
  gas uint64,
  header *types.Header,
  state *state.StateDB,
) (uint64, error) {
  return gEvm.evm.ABIStaticCall(gEvm.zeroCaller, scAddress, abi, funcName, args, returnObj, gas)
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


  gEvm := gasPriceFloorEvm{vm.AccountRef(common.HexToAddress("0x0")), st.evm}

  gasPriceFloors, goldGasPriceFloor := gasprice.GetGasPriceMapAndGold(gEvm, st.gcWl.regAdd, st.gcWl.GetListCopy())

  var gasPriceFloor *big.Int
  gasCurrency := st.msg.GasCurrency()

  if gasCurrency == nil {
    gasPriceFloor = goldGasPriceFloor
  } else if gasPriceFloors[*gasCurrency] != nil {
    gasPriceFloor = gasPriceFloors[*gasCurrency]
  } else {
    return errNonWhitelistedGasCurrency // currency should have already been checked in tx_pool
  }

  if (st.msg.GasPrice().Cmp(gasPriceFloor) == -1) {
    return errGasPriceDoesNotExceedFloor
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
	st.refundGas()
	gasUsed := st.gasUsed()
	// Pay gas fee to Coinbase chosen by the miner
	gasFee := new(big.Int).Mul(new(big.Int).SetUint64(gasUsed), st.gasPrice)
	log.Trace("Paying gas fees", "gas used", st.gasUsed(), "gasUsed", gasUsed, "gas fee", gasFee)
	log.Trace("Paying gas fees", "miner", st.evm.Coinbase, "gasFee", gasFee, "gas Currency", msg.GasCurrency())

	// TODO(asa): Revisit this when paying gas fees partially to infra fund.
	if msg.GasFeeRecipient() == nil {
		st.creditGas(msg.From(), gasFee, msg.GasCurrency())
	} else {
		st.creditGas(*msg.GasFeeRecipient(), gasFee, msg.GasCurrency())
	}

	return ret, st.gasUsed(), vmerr != nil, err
}

func (st *StateTransition) refundGas() {
	refund := st.state.GetRefund()
	// Apply refund counter, capped to half of the used gas.
	if refund > st.gasUsed()/2 {
		refund = st.gasUsed() / 2
	}

	st.gas += refund
	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)

	log.Trace("Refunding gas to sender", "sender", st.msg.From(),
		"refundAmount", remaining, "gas Currency", st.msg.GasCurrency())
	st.creditGas(st.msg.From(), remaining, st.msg.GasCurrency())
	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	st.gp.AddGas(st.gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gas
}
