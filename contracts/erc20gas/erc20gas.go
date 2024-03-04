package erc20gas

import (
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/contracts/internal/n"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

const (
	maxGasForDebitGasFeesTransactions  uint64 = 1 * n.Million
	maxGasForCreditGasFeesTransactions uint64 = 1 * n.Million
)

func DebitFees(evm *vm.EVM, address common.Address, amount *big.Int, feeCurrency *common.Address) error {
	// Function is "debitGasFees(address from, uint256 value)"
	// selector is first 4 bytes of keccak256 of "debitGasFees(address,uint256)"
	// Source:
	// pip3 install pyethereum
	// python3 -c 'from ethereum.utils import sha3; print(sha3("debitGasFees(address,uint256)")[0:4].hex())'
	functionSelector := hexutil.MustDecode("0x58cf9672")
	transactionData := common.GetEncodedAbi(functionSelector, [][]byte{common.AddressToAbi(address), common.AmountToAbi(amount)})

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
	// Function is "creditGasFees(address,address,address,address,uint256,uint256,uint256,uint256)"
	functionSelector := hexutil.MustDecode("0x6a30b253")
	transactionData := common.GetEncodedAbi(functionSelector, [][]byte{common.AddressToAbi(from), common.AddressToAbi(feeRecipient), common.AddressToAbi(*gatewayFeeRecipient), common.AddressToAbi(feeHandler), common.AmountToAbi(refund), common.AmountToAbi(tipTxFee), common.AmountToAbi(gatewayFee), common.AmountToAbi(baseTxFee)})

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
