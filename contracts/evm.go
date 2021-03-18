// Copyright 2017 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package contracts

import (
	"bytes"
	goerrors "errors"
	"math/big"
	"time"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/contracts/errors"
	"github.com/celo-org/celo-blockchain/core/vm"

	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
)

// systemCaller is the caller when the EVM is invoked from the within the blockchain system.
var systemCaller = vm.AccountRef(common.HexToAddress("0x0"))

type ContractCaller interface {
	// StartNoGas will stop metering gas until `reactive` is called
	// TODO remove after refactoring
	StartNoGas() (reactivate func())

	StaticCall(addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error)
	Call(addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error)
	ContractDeployed(addr common.Address) bool
}

func MakeCallFromSystem(evm *vm.EVM, scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, static bool) (uint64, error) {
	// Record a metrics data point about execution time.
	timer := metrics.GetOrRegisterTimer("contract_comm/systemcall/"+funcName, nil)
	start := time.Now()
	defer timer.UpdateSince(start)

	var gasLeft uint64
	var err error

	if static {
		gasLeft, err = StaticCallFromSystem(evm, scAddress, abi, funcName, args, returnObj, gas)
	} else {
		gasLeft, err = callFromSystem(evm, scAddress, abi, funcName, args, returnObj, gas, value)
	}
	if err != nil {
		log.Error("Error when invoking evm function", "err", err, "funcName", funcName, "static", static, "address", scAddress, "args", args, "gas", gas, "gasLeft", gasLeft, "value", value)
		return gasLeft, err
	}

	return gasLeft, nil
}

func MakeCallWithContractId(evm *vm.EVM, registryId [32]byte, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, static bool) (uint64, error) {
	scAddress, err := GetRegisteredAddress(evm, registryId)

	if err != nil {
		if err == errors.ErrSmartContractNotDeployed {
			log.Debug("Contract not yet registered", "function", funcName, "registryId", hexutil.Encode(registryId[:]))
			return 0, err
		} else if err == errors.ErrRegistryContractNotDeployed {
			log.Debug("Registry contract not yet deployed", "function", funcName, "registryId", hexutil.Encode(registryId[:]))
			return 0, err
		} else {
			log.Error("Error in getting registered address", "function", funcName, "registryId", hexutil.Encode(registryId[:]), "err", err)
			return 0, err
		}
	}

	gasLeft, err := MakeCallFromSystem(evm, scAddress, abi, funcName, args, returnObj, gas, value, static)
	if err != nil {
		log.Error("Error in executing function on registered contract", "function", funcName, "registryId", hexutil.Encode(registryId[:]), "err", err)
	}
	return gasLeft, err
}

func StaticCallFromSystem(evm *vm.EVM, contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error) {
	staticCall := func(transactionData []byte) ([]byte, uint64, error) {
		return evm.StaticCall(systemCaller, contractAddress, transactionData, gas)
	}

	return handleABICall(abi, funcName, args, returnObj, staticCall)
}

func callFromSystem(evm *vm.EVM, contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error) {
	call := func(transactionData []byte) ([]byte, uint64, error) {
		return evm.Call(systemCaller, contractAddress, transactionData, gas, value)
	}
	return handleABICall(abi, funcName, args, returnObj, call)
}

var (
	errorSig     = []byte{0x08, 0xc3, 0x79, 0xa0} // Keccak256("Error(string)")[:4]
	abiString, _ = abi.NewType("string", "", nil)
)

func unpackError(result []byte) (string, error) {
	if len(result) < 4 || !bytes.Equal(result[:4], errorSig) {
		return "<tx result not Error(string)>", goerrors.New("TX result not of type Error(string)")
	}
	vs, err := abi.Arguments{{Type: abiString}}.UnpackValues(result[4:])
	if err != nil {
		return "<invalid tx result>", err
	}
	return vs[0].(string), nil
}

func handleABICall(contractAbi abi.ABI, funcName string, args []interface{}, returnObj interface{}, call func([]byte) ([]byte, uint64, error)) (uint64, error) {
	transactionData, err := contractAbi.Pack(funcName, args...)
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
		if err := contractAbi.Unpack(returnObj, funcName, ret); err != nil {

			// TODO (mcortesi) Remove ErrEmptyArguments check after we change Proxy to fail on unset impl
			// `ErrEmptyArguments` is expected when when syncing & importing blocks
			// before a contract has been deployed
			if err == abi.ErrEmptyArguments {
				log.Trace("Error in unpacking EVM call return bytes", "err", err)
			} else {
				log.Error("Error in unpacking EVM call return bytes", "err", err)
			}
			return leftoverGas, err
		}
	}

	return leftoverGas, nil
}
