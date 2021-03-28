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

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/contract_comm/errors"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/core/vm/context"
	"github.com/celo-org/celo-blockchain/log"
)

var (
	emptyMessage                = types.NewMessage(common.HexToAddress("0x0"), nil, 0, common.Big0, 0, common.Big0, nil, nil, common.Big0, []byte{}, false, false)
	internalEvmHandlerSingleton *InternalEVMHandler
)

// An EVM handler to make calls to smart contracts from within geth
type InternalEVMHandler struct {
	chain vm.ChainContext
}

func MakeStaticCall(registryId common.Hash, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	backend, err := NewBackend(header, state)
	if err != nil {
		return 0, err
	}

	return Query(backend, registryId, &abi, method, args, returnObj, gas)
}

func Query(backend contracts.Backend, registryId common.Hash, abi *abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error) {

	contractAddress, err := resolveAddressForCall(backend, registryId, method)
	if err != nil {
		return 0, err
	}

	contract := contracts.NewContract(abi, contractAddress, contracts.SystemCaller)
	gasLeft, err := contract.Query(contracts.QueryOpts{MaxGas: gas, Backend: backend}, returnObj, method, args...)

	if err != nil {
		log.Error("Error when invoking evm function", "err", err, "function", method, "address", contractAddress, "args", args, "gas", gas, "gasLeft", gasLeft)
		return gasLeft, err
	}

	return gasLeft, nil
}

func MakeCall(registryId common.Hash, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state vm.StateDB, finaliseState bool) (uint64, error) {
	backend, err := NewBackend(header, state)
	if err != nil {
		return 0, err
	}

	gasLeft, err := Execute(backend, registryId, &abi, method, args, returnObj, gas, value)

	if err == nil && finaliseState {
		state.Finalise(true)
	}

	return gasLeft, err
}

func Execute(backend contracts.Backend, registryId common.Hash, abi *abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error) {
	contractAddress, err := resolveAddressForCall(backend, registryId, method)
	if err != nil {
		return 0, err
	}

	contract := contracts.NewContract(abi, contractAddress, contracts.SystemCaller)
	gasLeft, err := contract.Execute(contracts.ExecOpts{MaxGas: gas, Backend: backend, Value: value}, returnObj, method, args...)

	if err != nil {
		log.Error("Error when invoking evm function", "err", err, "function", method, "address", contractAddress, "args", args, "gas", gas, "gasLeft", gasLeft)
		return gasLeft, err
	}

	return gasLeft, err
}

func MakeStaticCallWithAddress(scAddress common.Address, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	backend, err := NewBackend(header, state)
	if err != nil {
		return 0, err
	}

	return QueryWithAddress(backend, scAddress, &abi, method, args, returnObj, gas)
}

func QueryWithAddress(backend contracts.Backend, contractAddress common.Address, abi *abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error) {
	contract := contracts.NewContract(abi, contractAddress, contracts.SystemCaller)
	gasLeft, err := contract.Query(contracts.QueryOpts{MaxGas: gas, Backend: backend}, returnObj, method, args...)

	if err != nil {
		log.Error("Error when invoking evm function", "err", err, "function", method, "address", contractAddress, "args", args, "gas", gas, "gasLeft", gasLeft)
		return gasLeft, err
	}

	return gasLeft, nil
}

func GetRegisteredAddress(registryId common.Hash, header *types.Header, state vm.StateDB) (common.Address, error) {
	vmevm, err := NewBackend(header, state)
	if err != nil {
		return common.ZeroAddress, err
	}
	return contracts.GetRegisteredAddress(vmevm, registryId)
}

func NewCachedBackend(header *types.Header, state vm.StateDB) (contracts.Backend, error) {
	backend, err := NewBackend(header, state)
	if err != nil {
		return nil, err
	}

	return contracts.WithGlobalCache(backend), nil
}

func NewBackend(header *types.Header, state vm.StateDB) (contracts.Backend, error) {
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
	context := context.New(emptyMessage, header, internalEvmHandlerSingleton.chain, nil)
	evm := contracts.NewEVMBackend(context, state, internalEvmHandlerSingleton.chain.Config(), *internalEvmHandlerSingleton.chain.GetVMConfig())

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

func resolveAddressForCall(backend contracts.Backend, registryId common.Hash, method string) (common.Address, error) {
	contractAddress, err := contracts.GetRegisteredAddress(backend, registryId)

	if err != nil {
		if err == errors.ErrSmartContractNotDeployed {
			log.Debug("Contract not yet registered", "function", method, "registryId", hexutil.Encode(registryId[:]))
		} else if err == errors.ErrRegistryContractNotDeployed {
			log.Debug("Registry contract not yet deployed", "function", method, "registryId", hexutil.Encode(registryId[:]))
		} else {
			log.Error("Error in getting registered address", "function", method, "registryId", hexutil.Encode(registryId[:]), "err", err)
		}
		return common.ZeroAddress, err
	}
	return contractAddress, nil
}
