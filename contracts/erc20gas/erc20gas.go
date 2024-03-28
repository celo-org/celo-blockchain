package erc20gas

import (
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/contracts/currency"
	"github.com/celo-org/celo-blockchain/contracts/internal/n"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

const (
	maxGasForDebitGasFeesTransactions  uint64 = 1 * n.Million
	maxGasForCreditGasFeesTransactions uint64 = 1 * n.Million
)

var (
	// Recalculate with `cast sig 'debitGasFees(address from, uint256 value)'`
	debitGasFeesSelector = hexutil.MustDecode("0x58cf9672")
	// Recalculate with `cast sig 'creditGasFees(address,address,address,address,uint256,uint256,uint256,uint256)'`
	creditGasFeesSelector = hexutil.MustDecode("0x6a30b253")
)

// Returns nil if debit is possible, used in tx pool validation
func TryDebitFees(tx *types.Transaction, from common.Address, currentVMRunner vm.EVMRunner) error {
	var fee *big.Int = tx.Fee()
	if tx.Type() == types.CeloDenominatedTxType {
		rate, err := currency.GetExchangeRate(currentVMRunner, tx.FeeCurrency())
		if err != nil {
			return err
		}
		fee = rate.FromBase(fee)
	}
	// The following code is similar to DebitFees, but that function does not work on a vm.EVMRunner,
	// so we have to adapt it instead of reusing.
	transactionData := common.GetEncodedAbi(debitGasFeesSelector, [][]byte{common.AddressToAbi(from), common.AmountToAbi(fee)})

	ret, err := currentVMRunner.ExecuteAndDiscardChanges(*tx.FeeCurrency(), transactionData, maxGasForDebitGasFeesTransactions, common.Big0)
	if err != nil {
		revertReason, err2 := abi.UnpackRevert(ret)
		if err2 == nil {
			return fmt.Errorf("TryDebitFees reverted: %s", revertReason)
		}
	}
	return err
}

func DebitFees(evm *vm.EVM, address common.Address, amount *big.Int, feeCurrency *common.Address) error {
	transactionData := common.GetEncodedAbi(debitGasFeesSelector, [][]byte{common.AddressToAbi(address), common.AmountToAbi(amount)})

	// Run only primary evm.Call() with tracer
	if evm.GetDebug() {
		evm.SetDebug(false)
		defer func() { evm.SetDebug(true) }()
	}

	rootCaller := vm.AccountRef(common.HexToAddress("0x0"))
	// The caller was already charged for the cost of this operation via IntrinsicGas.
	ret, leftoverGas, err := evm.Call(rootCaller, *feeCurrency, transactionData, maxGasForDebitGasFeesTransactions, big.NewInt(0))
	gasUsed := maxGasForDebitGasFeesTransactions - leftoverGas
	log.Trace("debitGasFees called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)
	if err != nil {
		revertReason, err2 := abi.UnpackRevert(ret)
		if err2 == nil {
			return fmt.Errorf("DebitFees reverted: %s", revertReason)
		}
	}
	return err
}

func CreditFees(
	evm *vm.EVM,
	from common.Address,
	feeRecipient common.Address,
	gatewayFeeRecipient *common.Address,
	feeHandler common.Address,
	refund *big.Int,
	tipTxFee *big.Int,
	gatewayFee *big.Int,
	baseTxFee *big.Int,
	feeCurrency *common.Address) error {
	transactionData := common.GetEncodedAbi(creditGasFeesSelector, [][]byte{common.AddressToAbi(from), common.AddressToAbi(feeRecipient), common.AddressToAbi(*gatewayFeeRecipient), common.AddressToAbi(feeHandler), common.AmountToAbi(refund), common.AmountToAbi(tipTxFee), common.AmountToAbi(gatewayFee), common.AmountToAbi(baseTxFee)})

	// Run only primary evm.Call() with tracer
	if evm.GetDebug() {
		evm.SetDebug(false)
		defer func() { evm.SetDebug(true) }()
	}

	rootCaller := vm.AccountRef(common.HexToAddress("0x0"))
	// The caller was already charged for the cost of this operation via IntrinsicGas.
	ret, leftoverGas, err := evm.Call(rootCaller, *feeCurrency, transactionData, maxGasForCreditGasFeesTransactions, big.NewInt(0))
	gasUsed := maxGasForCreditGasFeesTransactions - leftoverGas
	log.Trace("creditGas called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)
	if err != nil {
		revertReason, err2 := abi.UnpackRevert(ret)
		if err2 == nil {
			return fmt.Errorf("CreditFees reverted: %s", revertReason)
		}
	}
	return err
}
