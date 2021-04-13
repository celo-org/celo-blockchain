package contracts

import (
	"bytes"
	"errors"
	"math/big"

	abipkg "github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

// SystemCaller is the caller when the EVM is invoked from the within the blockchain system.
var SystemCaller = vm.AccountRef(common.HexToAddress("0x0"))

func StaticCallFromSystem(evm *vm.EVM, contractAddress common.Address, abi abipkg.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error) {
	staticCall := func(transactionData []byte) ([]byte, uint64, error) {
		return evm.StaticCall(SystemCaller, contractAddress, transactionData, gas)
	}

	return handleABICall(evm, abi, funcName, args, returnObj, staticCall)
}

func CallFromSystem(evm *vm.EVM, contractAddress common.Address, abi abipkg.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error) {
	call := func(transactionData []byte) ([]byte, uint64, error) {
		return evm.Call(SystemCaller, contractAddress, transactionData, gas, value)
	}
	return handleABICall(evm, abi, funcName, args, returnObj, call)
}

var (
	errorSig     = []byte{0x08, 0xc3, 0x79, 0xa0} // Keccak256("Error(string)")[:4]
	abiString, _ = abipkg.NewType("string", "", nil)
)

func unpackError(result []byte) (string, error) {
	if len(result) < 4 || !bytes.Equal(result[:4], errorSig) {
		return "<tx result not Error(string)>", errors.New("TX result not of type Error(string)")
	}
	vs, err := abipkg.Arguments{{Type: abiString}}.UnpackValues(result[4:])
	if err != nil {
		return "<invalid tx result>", err
	}
	return vs[0].(string), nil
}

func handleABICall(evm *vm.EVM, abi abipkg.ABI, funcName string, args []interface{}, returnObj interface{}, call func([]byte) ([]byte, uint64, error)) (uint64, error) {
	transactionData, err := abi.Pack(funcName, args...)
	if err != nil {
		log.Error("Error in generating the ABI encoding for the function call", "err", err, "funcName", funcName, "args", args)
		return 0, err
	}

	ret, leftoverGas, err := call(transactionData)

	if err != nil {
		msg, _ := unpackError(ret)
		// Do not log execution reverted as error for getAddressFor. This only happens before the Registry is deployed.
		// TODO(nategraf): Find a more generic and complete solution to the problem of logging tolerated EVM call failures.
		if funcName == "getAddressFor" {
			log.Trace("Error in calling the EVM", "funcName", funcName, "transactionData", hexutil.Encode(transactionData), "err", err, "msg", msg)
		} else {
			log.Error("Error in calling the EVM", "funcName", funcName, "transactionData", hexutil.Encode(transactionData), "err", err, "msg", msg)
		}
		return leftoverGas, err
	}

	log.Trace("EVM call successful", "funcName", funcName, "transactionData", hexutil.Encode(transactionData), "ret", hexutil.Encode(ret))

	if returnObj != nil {
		if err := abi.Unpack(returnObj, funcName, ret); err != nil {

			// TODO (mcortesi) Remove ErrEmptyArguments check after we change Proxy to fail on unset impl
			// `ErrEmptyArguments` is expected when when syncing & importing blocks
			// before a contract has been deployed
			if err == abipkg.ErrEmptyArguments {
				log.Trace("Error in unpacking EVM call return bytes", "err", err)
			} else {
				log.Error("Error in unpacking EVM call return bytes", "err", err)
			}
			return leftoverGas, err
		}
	}

	return leftoverGas, nil
}
