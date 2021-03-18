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

// Core Contract given a specific address
type AddressCaller interface {
	StaticCallFromSystem(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
	// MemoizedStaticCallFromSystem(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
	CallFromSystem(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error)
}

// Core Contract given a registry ID to lookup in the registry
type RegistryCaller interface {
	StaticCallFromSystemWithRegistryLookup(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
	// MemoizedStaticCallFromSystemWithRegistryLookup(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
	CallFromSystemWithRegistryLookup(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error)
}

// evmCaller implements the SystemContractCaller interface
type evmCaller struct {
	evm *vm.EVM
}

// This needs to pass each method through
type currentStateCaller struct{}

var emptyMessage = types.NewMessage(common.HexToAddress("0x0"), nil, 0, common.Big0, 0, common.Big0, nil, nil, common.Big0, []byte{}, false)

// NewCaller creates a caller object that acts on the supplied statedb in the environment of the header, state, and chain
func NewCaller(header *types.Header, state vm.StateDB, chain vm.ChainContext) SystemContractCaller {
	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	context := vm.NewEVMContext(emptyMessage, header, chain, nil)
	evm := vm.NewEVM(context, state, chain.Config(), *chain.GetVMConfig())
	return evmCaller{evm: evm}
}

// NewCurrentStateCaller creates a caller object that uses the current chain state
// SetIEVMHandler must be called before this method is able to be used
func NewCurrentStateCaller() (SystemContractCaller, error) {
	return currentStateCaller{}, nil
}
