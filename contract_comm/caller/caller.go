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
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
)

// SystemContractCaller Provides an interfaces for making core contract calls as the system against the supplied provider
type SystemContractCaller interface {
	AddressCaller
	RegistryCaller
	GetRegisteredAddress(registryId common.Hash) (*common.Address, error)
	// FinalizeState() error // TODO(joshua): This is used with random instead of finaliseState bool on MakeCall
}

// CachingSystemCaller extends the SystemContractCaller with a memoizing static call function
type CachingSystemCaller interface {
	SystemContractCaller
	MemoizedStaticCallFromSystem(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
	MemoizedStaticCallFromSystemWithRegistryLookup(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
}

// Core Contract given a specific address
type AddressCaller interface {
	StaticCallFromSystem(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
	CallFromSystem(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error)
}

// Core Contract given a registry ID to lookup in the registry
type RegistryCaller interface {
	StaticCallFromSystemWithRegistryLookup(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
	CallFromSystemWithRegistryLookup(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error)
}

// Creates a new EVM on demand.
type evmBuilder interface {
	createEVM() *vm.EVM
}

// Private struct that implements CachingSystemCaller.
type contractCommunicator struct {
	builder evmBuilder
}

var emptyMessage = types.NewMessage(common.HexToAddress("0x0"), nil, 0, common.Big0, 0, common.Big0, nil, nil, common.Big0, []byte{}, false)

// specificStateCaller implements evmBuilder for a given state. This allows core contract calls against and arbitrary state
type specificStateCaller struct {
	header *types.Header
	state  vm.StateDB
	chain  vm.ChainContext
}

func (c specificStateCaller) createEVM() *vm.EVM {
	context := vm.NewEVMContext(emptyMessage, c.header, c.chain, nil)
	return vm.NewEVM(context, c.state, c.chain.Config(), *c.chain.GetVMConfig())
}

// TODO(Joshua): Inject chain here instead of using singleton
type currentStateCaller struct{}

var internalEvmHandlerSingleton *InternalEVMHandler

// An EVM handler to make calls to smart contracts from within geth
type InternalEVMHandler struct {
	chain vm.ChainContext
}

func SetInternalEVMHandler(chain vm.ChainContext) {
	if internalEvmHandlerSingleton == nil {
		// log.Trace("Setting the InternalEVMHandler Singleton")
		internalEvmHandler := InternalEVMHandler{
			chain: chain,
		}
		internalEvmHandlerSingleton = &internalEvmHandler
	}
}

func (c currentStateCaller) createEVM() *vm.EVM {
	header := internalEvmHandlerSingleton.chain.CurrentHeader()
	var state vm.StateDB
	state, _ = internalEvmHandlerSingleton.chain.State()

	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	context := vm.NewEVMContext(emptyMessage, header, internalEvmHandlerSingleton.chain, nil)
	return vm.NewEVM(context, state, internalEvmHandlerSingleton.chain.Config(), *internalEvmHandlerSingleton.chain.GetVMConfig())
}

// NewCaller creates a caller object that acts on the supplied statedb in the environment of the header, state, and chain
func NewCaller(header *types.Header, state vm.StateDB, chain vm.ChainContext) SystemContractCaller {
	evmBuilder := specificStateCaller{header: header, state: state, chain: chain}
	return contractCommunicator{builder: evmBuilder}
}

// NewCurrentStateCaller creates a caller object that uses the current chain state
// SetIEVMHandler must be called before this method is able to be used
func NewCurrentStateCaller() CachingSystemCaller {
	return contractCommunicator{builder: currentStateCaller{}}
}
