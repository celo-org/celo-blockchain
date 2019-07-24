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

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/contract_comm/errors"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

var (
	emptyMessage   = types.NewMessage(common.HexToAddress("0x0"), nil, 0, common.Big0, 0, common.Big0, nil, nil, []byte{}, false)
	IevmHSingleton *InternalEVMHandler
)

// TODO(kevjue) - Figure out a way to not have duplicated code between this file and core/evm.go
// ChainContext supports retrieving chain data and consensus parameters
// from the block chain to be used during transaction processing.
type ChainContext interface {
	// Engine retrieves the blockchain's consensus engine.
	Engine() consensus.Engine

	// GetHeader returns the hash corresponding to their hash.
	GetHeader(common.Hash, uint64) *types.Header

	// GetVMConfig returns the node's vm configuration
	GetVMConfig() *vm.Config

	CurrentHeader() *types.Header

	State() (*state.StateDB, error)

	// Config returns the blockchain's chain configuration
	Config() *params.ChainConfig
}

// NewEVMContext creates a new context for use in the EVM.
func NewEVMContext(msg types.Message, header *types.Header, chain ChainContext, author *common.Address) vm.Context {
	// If we don't have an explicit author (i.e. not mining), extract from the header
	var beneficiary common.Address
	if author == nil {
		beneficiary, _ = chain.Engine().Author(header) // Ignore error, we're past header validation
	} else {
		beneficiary = *author
	}

	return vm.Context{
		CanTransfer: CanTransfer,
		Transfer:    Transfer,
		GetHash:     GetHashFn(header, chain),
		Origin:      msg.From(),
		Coinbase:    beneficiary,
		BlockNumber: new(big.Int).Set(header.Number),
		Time:        new(big.Int).Set(header.Time),
		Difficulty:  new(big.Int).Set(header.Difficulty),
		GasLimit:    header.GasLimit,
		GasPrice:    new(big.Int).Set(msg.GasPrice()),
	}
}

// GetHashFn returns a GetHashFunc which retrieves header hashes by number
func GetHashFn(ref *types.Header, chain ChainContext) func(n uint64) common.Hash {
	var cache map[uint64]common.Hash

	return func(n uint64) common.Hash {
		// If there's no hash cache yet, make one
		if cache == nil {
			cache = map[uint64]common.Hash{
				ref.Number.Uint64() - 1: ref.ParentHash,
			}
		}
		// Try to fulfill the request from the cache
		if hash, ok := cache[n]; ok {
			return hash
		}
		// Not cached, iterate the blocks and cache the hashes
		for header := chain.GetHeader(ref.ParentHash, ref.Number.Uint64()-1); header != nil; header = chain.GetHeader(header.ParentHash, header.Number.Uint64()-1) {
			cache[header.Number.Uint64()-1] = header.ParentHash
			if n == header.Number.Uint64()-1 {
				return header.ParentHash
			}
		}
		return common.Hash{}
	}
}

// CanTransfer checks whether there are enough funds in the address' account to make a transfer.
// This does not take the necessary gas in to account to make the transfer valid.
func CanTransfer(db vm.StateDB, addr common.Address, amount *big.Int) bool {
	return db.GetBalance(addr).Cmp(amount) >= 0
}

// Transfer subtracts amount from sender and adds amount to recipient using the given Db
func Transfer(db vm.StateDB, sender, recipient common.Address, amount *big.Int) {
	db.SubBalance(sender, amount)
	db.AddBalance(recipient, amount)
}

// An EVM handler to make calls to smart contracts from within geth
type InternalEVMHandler struct {
	chain ChainContext
}

func MakeStaticCall(scRegistryId string, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state *state.StateDB) (uint64, error) {
	scAddress, err := GetContractAddress(scRegistryId, header, state)
	if err == errors.ErrSmartContractNotDeployed || err == errors.ErrRegistryContractNotDeployed {
		log.Warn("Contract deployment in progress")
		return 0, err
	}
	if err != nil {
		return 0, err
	}
	return executeEVMFunction(*scAddress, abi, funcName, args, returnObj, gas, nil, header, state, false)
}

func MakeCall(scRegistryId string, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state *state.StateDB) (uint64, error) {
	scAddress, err := GetContractAddress(scRegistryId, header, state)
	if err == errors.ErrSmartContractNotDeployed || err == errors.ErrRegistryContractNotDeployed {
		log.Warn("Contract deployment in progress")
		return 0, err
	}
	if err != nil {
		return 0, err
	}
	return executeEVMFunction(*scAddress, abi, funcName, args, returnObj, gas, value, header, state, true)
}

func MakeStaticCallWithAddress(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state *state.StateDB) (uint64, error) {
	return executeEVMFunction(scAddress, abi, funcName, args, returnObj, gas, nil, header, state, false)
}

func MakeCallWithAddress(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state *state.StateDB) (uint64, error) {
	return executeEVMFunction(scAddress, abi, funcName, args, returnObj, gas, value, header, state, true)
}

func GetContractAddress(registryId string, header *types.Header, state *state.StateDB) (*common.Address, error) {
	vmevm, err := createVMEVM(header, state)
	if err != nil {
		return nil, err
	}
	scAddress, err := vm.GetRegisteredAddressWithEvm(registryId, vmevm)
	return scAddress, err
}

func createVMEVM(header *types.Header, state *state.StateDB) (*vm.EVM, error) {
	// Normally, when making an evm call, we should use the current block's state.  However,
	// there are times (e.g. retrieving the set of validators when an epoch ends) that we need
	// to call the evm using the currently mined block.  In that case, the header and state params
	// will be non nil.
	log.Trace("createEVM called")
	if IevmHSingleton == nil {
		return nil, errors.ErrNoIevmHSingleton
	}

	if header == nil {
		header = IevmHSingleton.chain.CurrentHeader()
	}

	if state == nil {
		var err error
		state, err = IevmHSingleton.chain.State()
		if err != nil {
			log.Error("Error in retrieving the state from the blockchain", "err", err)
			return nil, err
		}
	}

	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	context := NewEVMContext(emptyMessage, header, IevmHSingleton.chain, nil)
	evm := vm.NewEVM(context, state, IevmHSingleton.chain.Config(), *IevmHSingleton.chain.GetVMConfig())

	return evm, nil
}

func executeEVMFunction(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state *state.StateDB, mutateState bool) (uint64, error) {
	vmevm, err := createVMEVM(header, state)
	if err != nil {
		return 0, err
	}

	var gasLeft uint64

	if mutateState {
		gasLeft, err = vmevm.CallFromSystem(scAddress, abi, funcName, args, returnObj, gas, value)
	} else {
		gasLeft, err = vmevm.StaticCallFromSystem(scAddress, abi, funcName, args, returnObj, gas)
	}
	if err != nil {
		log.Error("Error when invoking evm function", "err", err)
		return gasLeft, err
	}

	if mutateState {
		state.Finalise(true)
	}

	return gasLeft, nil
}

func SetInternalEVMHandler(chain ChainContext) {
	if IevmHSingleton == nil {
		log.Trace("Setting the InternalEVMHandler Singleton")
		iEvmH := InternalEVMHandler{
			chain: chain,
		}
		IevmHSingleton = &iEvmH
	}
}
