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

package contract_comm

import (
	"math/big"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contract_comm/errors"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
)

var (
	emptyMessage                = types.NewMessage(common.HexToAddress("0x0"), nil, 0, common.Big0, 0, common.Big0, nil, nil, common.Big0, []byte{}, false)
	internalEvmHandlerSingleton *InternalEVMHandler
)

// An EVM handler to make calls to smart contracts from within geth
type InternalEVMHandler struct {
	chain vm.ChainContext
}

func MakeStaticCall(registryId [32]byte, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	return makeCallWithContractId(registryId, abi, funcName, args, returnObj, gas, nil, header, state, true)
}

func MakeCall(registryId [32]byte, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state vm.StateDB, finaliseState bool) (uint64, error) {
	gasLeft, err := makeCallWithContractId(registryId, abi, funcName, args, returnObj, gas, value, header, state, false)
	if err == nil && finaliseState {
		state.Finalise(true)
	}
	return gasLeft, err
}

func MakeStaticCallWithAddress(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	return makeCallFromSystem(scAddress, abi, funcName, args, returnObj, gas, nil, header, state, true)
}

func GetRegisteredAddress(registryId [32]byte, header *types.Header, state vm.StateDB) (*common.Address, error) {
	vmevm, err := createEVM(header, state)
	if err != nil {
		return nil, err
	}
	return vm.GetRegisteredAddressWithEvm(registryId, vmevm)
}

func createEVM(header *types.Header, state vm.StateDB) (*vm.EVM, error) {
	// Normally, when making an evm call, we should use the current block's state.  However,
	// there are times (e.g. retrieving the set of validators when an epoch ends) that we need
	// to call the evm using the currently mined block.  In that case, the header and state params
	// will be non nil.
	if internalEvmHandlerSingleton == nil {
		return nil, errors.ErrNoInternalEvmHandlerSingleton
	}

	if header == nil {
		header = internalEvmHandlerSingleton.chain.CurrentHeader()
	}

	if state == nil || reflect.ValueOf(state).IsNil() {
		var err error
		state, err = internalEvmHandlerSingleton.chain.State()
		if err != nil {
			log.Error("Error in retrieving the state from the blockchain", "err", err)
			return nil, err
		}
	}

	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	context := vm.NewEVMContext(emptyMessage, header, internalEvmHandlerSingleton.chain, nil)
	evm := vm.NewEVM(context, state, internalEvmHandlerSingleton.chain.Config(), *internalEvmHandlerSingleton.chain.GetVMConfig())

	return evm, nil
}

func makeCallFromSystem(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state vm.StateDB, static bool) (uint64, error) {
	vmevm, err := createEVM(header, state)
	if err != nil {
		return 0, err
	}

	var gasLeft uint64

	if static {
		gasLeft, err = vmevm.StaticCallFromSystem(scAddress, abi, funcName, args, returnObj, gas)
	} else {
		gasLeft, err = vmevm.CallFromSystem(scAddress, abi, funcName, args, returnObj, gas, value)
	}
	if err != nil {
		log.Error("Error when invoking evm function", "err", err, "funcName", funcName, "static", static, "address", scAddress, "args", args)
		return gasLeft, err
	}

	return gasLeft, nil
}

func SetInternalEVMHandler(chain vm.ChainContext) {
	if internalEvmHandlerSingleton == nil {
		log.Trace("Setting the InternalEVMHandler Singleton")
		internalEvmHandler := InternalEVMHandler{
			chain: chain,
		}
		internalEvmHandlerSingleton = &internalEvmHandler
	}
}

func makeCallWithContractId(registryId [32]byte, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state vm.StateDB, static bool) (uint64, error) {
	scAddress, err := GetRegisteredAddress(registryId, header, state)

	if err != nil {
		if err == errors.ErrSmartContractNotDeployed {
			log.Debug("Contract not yet registered", "function", funcName, "registryId", registryId)
			return 0, err
		} else if err == errors.ErrRegistryContractNotDeployed {
			log.Debug("Registry contract not yet deployed", "function", funcName, "registryId", registryId)
			return 0, err
		} else {
			log.Error("Error in getting registered address", "function", funcName, "registryId", registryId, "err", err)
			return 0, err
		}
	}

	gasLeft, err := makeCallFromSystem(*scAddress, abi, funcName, args, returnObj, gas, value, header, state, static)
	if err != nil {
		log.Error("Error in executing function on registered contract", "function", funcName, "registryId", registryId, "err", err)
	}
	return gasLeft, err
}
