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
	"github.com/celo-org/celo-blockchain/core/vm/vmcontext"
	"github.com/celo-org/celo-blockchain/log"
)

var (
	evmFactory vm.EVMFactory
)

func SetEVMFactory(factory vm.EVMFactory) {
	if evmFactory == nil {
		log.Trace("Setting the evmFactory Singleton")
		evmFactory = factory
	}
}

func getProvider(header *types.Header, state vm.StateDB) (vm.EVMProvider, error) {

	// This check and returned error allows the TestGolangBindings test to
	// pass. It would be nicer if the test actually set the provider.
	if evmFactory == nil {
		return nil, errors.ErrNoInternalEvmHandlerSingleton
	}
	if header == nil {
		header = evmFactory.Chain().CurrentHeader()
		var err error
		state, err = evmFactory.Chain().State()
		if err != nil {
			return nil, err
		}
	}
	return vmcontext.NewEVMProvider(evmFactory, header, state), nil
}

func GetRegisteredAddress(registryId common.Hash, header *types.Header, state vm.StateDB) (common.Address, error) {
	provider, err := getProvider(header, state)
	if err != nil {
		return common.Address{}, err
	}
	return contracts.GetRegisteredAddress(provider.EVM(), registryId)
}

func MakeStaticCall(registryId common.Hash, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	provider, err := getProvider(header, state)
	if err != nil {
		return 0, err
	}

	contractAddress, err := contracts.ResolveAddressForCall(provider.EVM(), registryId, method)
	if err != nil {
		return 0, err
	}
	return contracts.QueryCallFromVM(contractAddress, gas, contracts.NewMessage(&abi, method, args...)).Run(provider.EVM(), returnObj)
}

func MakeCall(registryId common.Hash, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state vm.StateDB, finaliseState bool) (uint64, error) {
	provider, err := getProvider(header, state)
	if err != nil {
		return 0, err
	}

	contractAddress, err := contracts.ResolveAddressForCall(provider.EVM(), registryId, method)
	if err != nil {
		return 0, err
	}
	gasLeft, err := contracts.WriteCallFromVM(contractAddress, gas, value, contracts.NewMessage(&abi, method, args...)).Run(provider.EVM(), returnObj)

	if err == nil && finaliseState {
		state.Finalise(true)
	}

	return gasLeft, err
}

func MakeStaticCallWithAddress(contractAddress common.Address, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	provider, err := getProvider(header, state)
	if err != nil {
		return 0, err
	}

	return contracts.QueryCallFromVM(contractAddress, gas, contracts.NewMessage(&abi, method, args...)).Run(provider.EVM(), returnObj)
}
