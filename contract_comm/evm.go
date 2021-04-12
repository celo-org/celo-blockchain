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

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contract_comm/errors"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

var (
	evmCallerFactory vm.EVMCallerFactory
)

func SetEVMCallerFactory(_evmCallerFactory vm.EVMCallerFactory) {
	if evmCallerFactory == nil {
		log.Trace("Setting the evmCallerFactory Singleton")
		evmCallerFactory = _evmCallerFactory
	}
}

func getCaller(header *types.Header, state vm.StateDB) (vm.EVMCaller, error) {
	// Normally, when making an evm call, we should use the current block's state.  However,
	// there are times (e.g. retrieving the set of validators when an epoch ends) that we need
	// to call the evm using the currently mined block.  In that case, the header and state params
	// will be non nil.
	if evmCallerFactory == nil {
		return nil, errors.ErrNoInternalEvmHandlerSingleton
	}

	return evmCallerFactory.NewEVMCaller(header, state)
}

func GetRegisteredAddress(registryId common.Hash, header *types.Header, state vm.StateDB) (common.Address, error) {
	caller, err := getCaller(header, state)
	if err != nil {
		return common.ZeroAddress, err
	}
	return contracts.GetRegisteredAddress(caller, registryId)
}

func MakeStaticCall(registryId common.Hash, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	caller, err := getCaller(header, state)
	if err != nil {
		return 0, err
	}

	return contracts.QueryCallOnRegisteredContract(registryId, gas, contracts.NewMessage(&abi, method, args...)).Run(caller, returnObj)
}

func MakeCall(registryId common.Hash, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state vm.StateDB, finaliseState bool) (uint64, error) {
	caller, err := getCaller(header, state)
	if err != nil {
		return 0, err
	}

	gasLeft, err := contracts.WriteCallOnRegisteredContract(registryId, gas, value, contracts.NewMessage(&abi, method, args...)).Run(caller, returnObj)

	if err == nil && finaliseState {
		state.Finalise(true)
	}

	return gasLeft, err
}

func MakeStaticCallWithAddress(contractAddress common.Address, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	caller, err := getCaller(header, state)
	if err != nil {
		return 0, err
	}

	return contracts.QueryCallFromVM(contractAddress, gas, contracts.NewMessage(&abi, method, args...)).Run(caller, returnObj)
}
