package erc20gas

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/contracts/internal/n"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

const (
	maxGasForDebitGasFeesTransactions  uint64 = 1 * n.Million
	maxGasForCreditGasFeesTransactions uint64 = 1 * n.Million
	maxGasForDebitNativeTransactions   uint64 = 1 * n.Million
	maxGasForCreditNativeTransactions  uint64 = 1 * n.Million
)

var (
	tmpAddress = common.HexToAddress("0xce106a5")
)

func DebitFees(evm *vm.EVM, address common.Address, amount *big.Int, feeCurrency *common.Address) error {
	if amount.Cmp(big.NewInt(0)) == 0 {
		return nil
	}
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
	_, leftoverGas, err := evm.Call(rootCaller, *feeCurrency, transactionData, maxGasForDebitGasFeesTransactions, big.NewInt(0))
	gasUsed := maxGasForDebitGasFeesTransactions - leftoverGas
	log.Trace("debitGasFees called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)
	return err
}

func DebitFeesNative(evm *vm.EVM, address common.Address, amount *big.Int, feeCurrency *common.Address) error {
	if amount.Cmp(big.NewInt(0)) == 0 {
		return nil
	}

	// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
	//
	// Solidity: function transfer(address to, uint256 amount) returns(bool)
	transferSelector := hexutil.MustDecode("0xa9059cbb")
	transferData := common.GetEncodedAbi(transferSelector, [][]byte{common.AddressToAbi(tmpAddress), common.AmountToAbi(amount)})

	// create account ref
	caller := vm.AccountRef(address)

	_, leftoverGas, err := evm.Call(caller, *feeCurrency, transferData, maxGasForDebitNativeTransactions, big.NewInt(0))
	gasUsed := maxGasForCreditNativeTransactions - leftoverGas
	log.Trace("debitGasFees called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)
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
	_, leftoverGas, err := evm.Call(rootCaller, *feeCurrency, transactionData, maxGasForCreditGasFeesTransactions, big.NewInt(0))
	gasUsed := maxGasForCreditGasFeesTransactions - leftoverGas
	log.Trace("creditGas called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)
	return err
}

func CreditFeesNative(
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
	// create account ref
	caller := vm.AccountRef(tmpAddress)

	// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
	//
	// Solidity: function transfer(address to, uint256 amount) returns(bool)
	transferSelector := hexutil.MustDecode("0xa9059cbb")
	transfer1Data := common.GetEncodedAbi(transferSelector, [][]byte{common.AddressToAbi(from), common.AmountToAbi(refund)})
	transfer2Data := common.GetEncodedAbi(transferSelector, [][]byte{common.AddressToAbi(feeRecipient), common.AmountToAbi(tipTxFee)})
	transfer3Data := common.GetEncodedAbi(transferSelector, [][]byte{common.AddressToAbi(feeHandler), common.AmountToAbi(baseTxFee)})

	// do EVM calls
	_, leftoverGas, err := evm.Call(caller, *feeCurrency, transfer1Data, maxGasForCreditNativeTransactions, big.NewInt(0))
	if err != nil {
		return err
	}
	gasUsed := maxGasForDebitNativeTransactions - leftoverGas
	_, leftoverGas, err = evm.Call(caller, *feeCurrency, transfer2Data, maxGasForCreditNativeTransactions, big.NewInt(0))
	if err != nil {
		return err
	}
	gasUsed = gasUsed - leftoverGas
	_, leftoverGas, err = evm.Call(caller, *feeCurrency, transfer3Data, maxGasForCreditNativeTransactions, big.NewInt(0))
	if err != nil {
		return err
	}
	gasUsed = gasUsed - leftoverGas

	log.Trace("debitGasFees called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)

	return err
}
