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
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

type evmRunnerFactory func(header *types.Header, state vm.StateDB) (vm.EVMRunner, error)

var (
	_newEvmRunner evmRunnerFactory
)

func SetEVMRunnerFactory(factory evmRunnerFactory) {
	if _newEvmRunner == nil {
		log.Trace("Setting the evmRunnerFactory Singleton")
		_newEvmRunner = factory
	}
}

func MustNewEVMRunner(header *types.Header, state vm.StateDB) vm.EVMRunner {
	vmRunner, err := newEVMRunner(header, state)
	if err != nil {
		panic("failed to get vmRunner")
	}
	return vmRunner
}

func newEVMRunner(header *types.Header, state vm.StateDB) (vm.EVMRunner, error) {
	// Normally, when making an evm call, we should use the current block's state.  However,
	// there are times (e.g. retrieving the set of validators when an epoch ends) that we need
	// to call the evm using the currently mined block.  In that case, the header and state params
	// will be non nil.
	if _newEvmRunner == nil {
		return nil, contracts.ErrNoInternalEvmHandlerSingleton
	}

	return _newEvmRunner(header, state)
}

func GetRegisteredAddress(registryId common.Hash, header *types.Header, state vm.StateDB) (common.Address, error) {
	vmRunner, err := newEVMRunner(header, state)
	if err != nil {
		return common.ZeroAddress, err
	}
	return contracts.GetRegisteredAddress(vmRunner, registryId)
}

func MakeStaticCall(registryId common.Hash, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) error {
	vmRunner, err := newEVMRunner(header, state)
	if err != nil {
		return err
	}

	m := contracts.NewRegisteredContractMethod(registryId, &abi, method, gas)
	return m.Query(vmRunner, returnObj, args...)

}

func MakeCall(registryId common.Hash, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state vm.StateDB, finaliseState bool) error {
	vmRunner, err := newEVMRunner(header, state)
	if err != nil {
		return err
	}

	m := contracts.NewRegisteredContractMethod(registryId, &abi, method, gas)
	err = m.Execute(vmRunner, returnObj, value, args...)

	if err == nil && finaliseState {
		state.Finalise(true)
	}

	return err
}

func MakeStaticCallWithAddress(contractAddress common.Address, abi abi.ABI, method string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) error {
	vmRunner, err := newEVMRunner(header, state)
	if err != nil {
		return err
	}

	m := contracts.NewBoundMethod(contractAddress, &abi, method, gas)
	return m.Query(vmRunner, returnObj, args...)
}
