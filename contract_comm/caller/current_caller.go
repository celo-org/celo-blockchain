// Copyright 2021 The Celo Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package caller

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

var internalEvmHandlerSingleton *InternalEVMHandler

// An EVM handler to make calls to smart contracts from within geth
type InternalEVMHandler struct {
	chain vm.ChainContext
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

func createCurrentEVMCaller() evmCaller {
	header := internalEvmHandlerSingleton.chain.CurrentHeader()
	state, _ := internalEvmHandlerSingleton.chain.State()

	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	context := vm.NewEVMContext(emptyMessage, header, internalEvmHandlerSingleton.chain, nil)
	evm := vm.NewEVM(context, state, internalEvmHandlerSingleton.chain.Config(), *internalEvmHandlerSingleton.chain.GetVMConfig())

	return evmCaller{evm: evm}
}

func (c currentStateCaller) StaticCallFromSystem(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error) {
	nc := createCurrentEVMCaller()
	return nc.StaticCallFromSystem(contractAddress, abi, funcName, args, returnObj, gas)
}

func (c currentStateCaller) CallFromSystem(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error) {
	nc := createCurrentEVMCaller()
	return nc.CallFromSystem(contractAddress, abi, funcName, args, returnObj, gas, value)
}

func (c currentStateCaller) StaticCallFromSystemWithRegistryLookup(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error) {
	nc := createCurrentEVMCaller()
	return nc.StaticCallFromSystemWithRegistryLookup(registryId, abi, funcName, args, returnObj, gas)
}

func (c currentStateCaller) CallFromSystemWithRegistryLookup(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error) {
	nc := createCurrentEVMCaller()
	return nc.CallFromSystemWithRegistryLookup(registryId, abi, funcName, args, returnObj, gas, value)
}

func (c currentStateCaller) GetRegisteredAddress(registryId common.Hash) (*common.Address, error) {
	nc := createCurrentEVMCaller()
	return nc.GetRegisteredAddress(registryId)
}
