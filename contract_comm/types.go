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

package contract_comm

import (
	"math/big"
	"reflect"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	ccerrors "github.com/celo-org/celo-blockchain/contract_comm/errors"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

// SystemContractCaller runs core contract calls. The implementation provides the state on which the calls are ran
type SystemContractCaller interface {
	AddressCaller
	RegistryCaller
	GetRegisteredAddress(registryId common.Hash) (*common.Address, error)
}

// AddressCaller makes a call with a specified address
type AddressCaller interface {
	MakeStaticCallWithAddress(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
	MakeCallWithAddress(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error)
}

// RegistryCaller looks up the contract address from the registry ID and then makes an address cal
type RegistryCaller interface {
	MakeStaticCall(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
	MakeCall(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error)
}

// Creates a new EVM on demand.
type evmBuilder interface {
	createEVM() (*vm.EVM, error)
}

// Private struct that implements SystemContractCaller.
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

func (c specificStateCaller) createEVM() (*vm.EVM, error) {
	if internalEvmHandlerSingleton == nil || c.chain == nil {
		return nil, ccerrors.ErrNoInternalEvmHandlerSingleton
	}
	context := vm.NewEVMContext(emptyMessage, c.header, c.chain, nil)
	return vm.NewEVM(context, c.state, c.chain.Config(), *c.chain.GetVMConfig()), nil
}

// TODO(Joshua): Inject chain here instead of using singleton
// currentStateCaller creates an EVM from the current blockchain state
type currentStateCaller struct{}

var internalEvmHandlerSingleton *InternalEVMHandler

// An EVM handler to make calls to smart contracts from within geth
type InternalEVMHandler struct {
	chain vm.ChainContext
}

// Sets the singletone. Currently required. To eliminate must pass install of contract caller to each contract_comm method
func SetInternalEVMHandler(chain vm.ChainContext) {
	if internalEvmHandlerSingleton == nil {
		log.Trace("Setting the InternalEVMHandler Singleton")
		internalEvmHandler := InternalEVMHandler{
			chain: chain,
		}
		internalEvmHandlerSingleton = &internalEvmHandler
	}
}

func (c currentStateCaller) createEVM() (*vm.EVM, error) {
	if internalEvmHandlerSingleton == nil {
		return nil, ccerrors.ErrNoInternalEvmHandlerSingleton
	}
	header := internalEvmHandlerSingleton.chain.CurrentHeader()
	var state vm.StateDB
	state, err := internalEvmHandlerSingleton.chain.State()
	if err != nil {
		log.Error("Error in retrieving the state from the blockchain", "err", err)
		return nil, err
	}

	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	context := vm.NewEVMContext(emptyMessage, header, internalEvmHandlerSingleton.chain, nil)
	return vm.NewEVM(context, state, internalEvmHandlerSingleton.chain.Config(), *internalEvmHandlerSingleton.chain.GetVMConfig()), nil
}

// NewCaller creates a caller object that acts on the supplied statedb in the environment of the header, state
// SetIEVMHandler must be called prior to using this (to maintain backwards compatibility)
func NewCaller(header *types.Header, state vm.StateDB) SystemContractCaller {
	if header == nil || state == nil || reflect.ValueOf(state).IsNil() {
		return NewCurrentStateCaller()
	}
	var chain vm.ChainContext
	if internalEvmHandlerSingleton != nil {
		chain = internalEvmHandlerSingleton.chain
	} else {
		chain = nil
	}
	evmBuilder := specificStateCaller{header: header, state: state, chain: chain}
	return contractCommunicator{builder: evmBuilder}
}

// NewCurrentStateCaller creates a caller object that uses the current chain state
// SetIEVMHandler must be called before this method is able to be used
func NewCurrentStateCaller() SystemContractCaller {
	return contractCommunicator{builder: currentStateCaller{}}
}
