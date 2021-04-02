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
	"time"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/contract_comm/errors"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
)

var (
	emptyMessage                = types.NewMessage(common.HexToAddress("0x0"), nil, 0, common.Big0, 0, common.Big0, nil, nil, common.Big0, []byte{}, false, false)
	internalEvmHandlerSingleton *InternalEVMHandler
)

// An EVM handler to make calls to smart contracts from within geth
type InternalEVMHandler struct {
	chain vm.ChainContext
}

// MakeStaticCall performs a static (read-only) ABI call against the contract specfied by the registry id.
func MakeStaticCall(registryId [32]byte, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	scAddress, err := resolveAddressForCall(registryId, funcName, header, state, false)
	if err != nil {
		return 0, err
	}
	return MakeStaticCallToAddress(scAddress, abi, funcName, args, returnObj, gas, header, state)

}

// MakeMemoizedStaticCall performs a static (read-only) ABI call to the smart contract address given.
// It will attempt to memoize the call based on the contract address, transaction name, gas allowance, and state root.
func MakeMemoizedStaticCall(registryId [32]byte, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	// Record a metrics data point about execution time.
	timer := metrics.GetOrRegisterTimer("contract_comm/systemcall/"+funcName, nil)
	start := time.Now()
	defer timer.UpdateSince(start)

	scAddress, err := resolveAddressForCall(registryId, funcName, header, state, true)
	if err != nil {
		return 0, err
	}

	vmevm, err := createEVM(header, state)
	if err != nil {
		return 0, err
	}
	gasLeft, err := vmevm.MemoizedStaticCallFromSystem(scAddress, abi, funcName, args, returnObj, gas)

	if err != nil {
		log.Error("Error when performing an evm static call", "err", err, "funcName", funcName, "address", scAddress, "args", args, "gas", gas, "gasLeft", gasLeft)
		return gasLeft, err
	}

	return gasLeft, nil
}

// MakeCall performs a mutating ABI call against the contract specfied by the registry id.
func MakeCall(registryId [32]byte, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state vm.StateDB, finaliseState bool) (uint64, error) {
	// Record a metrics data point about execution time.
	timer := metrics.GetOrRegisterTimer("contract_comm/systemcall/"+funcName, nil)
	start := time.Now()
	defer timer.UpdateSince(start)

	scAddress, err := resolveAddressForCall(registryId, funcName, header, state, false)
	if err != nil {
		return 0, err
	}

	vmevm, err := createEVM(header, state)
	if err != nil {
		return 0, err
	}
	gasLeft, err := vmevm.CallFromSystem(scAddress, abi, funcName, args, returnObj, gas, value)

	if err != nil {
		log.Error("Error when performing an evm call", "err", err, "funcName", funcName, "address", scAddress, "args", args, "gas", gas, "gasLeft", gasLeft)
		return gasLeft, err
	}

	if err == nil && finaliseState {
		state.Finalise(true)
	}

	return gasLeft, nil
}

// MakeStaticCallToAddress performs a static (read-only) ABI call to the smart contract address given.
func MakeStaticCallToAddress(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	// Record a metrics data point about execution time.
	timer := metrics.GetOrRegisterTimer("contract_comm/systemcall/"+funcName, nil)
	start := time.Now()
	defer timer.UpdateSince(start)

	vmevm, err := createEVM(header, state)
	if err != nil {
		return 0, err
	}
	gasLeft, err := vmevm.StaticCallFromSystem(scAddress, abi, funcName, args, returnObj, gas)

	if err != nil {
		log.Error("Error when performing an evm static call", "err", err, "funcName", funcName, "address", scAddress, "args", args, "gas", gas, "gasLeft", gasLeft)
		return gasLeft, err
	}

	return gasLeft, nil
}

// GetRegisteredAddress looks up the smart contract address associated with the registry id
func GetRegisteredAddress(registryId [32]byte, header *types.Header, state vm.StateDB) (*common.Address, error) {
	vmevm, err := createEVM(header, state)
	if err != nil {
		return nil, err
	}
	return vm.GetRegisteredAddressWithEvm(registryId, vmevm)
}

// MemoizedGetRegisteredAddress looks up the smart contract address associated with the registry id.
// It will attempt to memoize the call based on the contract address, transaction name, gas allowance, and state root.
func MemoizedGetRegisteredAddress(registryId [32]byte, header *types.Header, state vm.StateDB) (*common.Address, error) {
	vmevm, err := createEVM(header, state)
	if err != nil {
		return nil, err
	}
	return vm.MemoizedGetRegisteredAddressWithEvm(registryId, vmevm)
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

func SetInternalEVMHandler(chain vm.ChainContext) {
	if internalEvmHandlerSingleton == nil {
		log.Trace("Setting the InternalEVMHandler Singleton")
		internalEvmHandler := InternalEVMHandler{
			chain: chain,
		}
		internalEvmHandlerSingleton = &internalEvmHandler
	}
}

// resolveAddressForCall looks up the address of a core contract based on the the registry ID.
func resolveAddressForCall(registryId [32]byte, funcName string, header *types.Header, state vm.StateDB, memoize bool) (common.Address, error) {
	var contractAddress *common.Address
	var err error
	if memoize {
		contractAddress, err = MemoizedGetRegisteredAddress(registryId, header, state)

	} else {
		contractAddress, err = GetRegisteredAddress(registryId, header, state)
	}

	if err != nil {
		if err == errors.ErrSmartContractNotDeployed {
			log.Debug("Contract not yet registered", "function", funcName, "registryId", hexutil.Encode(registryId[:]))
		} else if err == errors.ErrRegistryContractNotDeployed {
			log.Debug("Registry contract not yet deployed", "function", funcName, "registryId", hexutil.Encode(registryId[:]))
		} else {
			log.Error("Error in getting registered address", "function", funcName, "registryId", hexutil.Encode(registryId[:]), "err", err)
		}
		return common.ZeroAddress, err
	}
	return *contractAddress, nil
}
