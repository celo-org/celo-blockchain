// Copyright 2016 The go-ethereum Authors
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

package core

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// ChainContext supports retrieving headers and consensus parameters from the
// current blockchain to be used during transaction processing.
type ChainContext interface {
	// Engine retrieves the chain's consensus engine.
	Engine() consensus.Engine

	// GetHeader returns the hash corresponding to their hash.
	GetHeader(common.Hash, uint64) *types.Header
}

// NewEVMContext creates a new context for use in the EVM.
func NewEVMContext(msg Message, header *types.Header, chain ChainContext, author *common.Address, registeredAddresses *RegisteredAddresses) vm.Context {
	// If we don't have an explicit author (i.e. not mining), extract from the header
	var beneficiary common.Address
	if author == nil {
		beneficiary, _ = chain.Engine().Author(header) // Ignore error, we're past header validation
	} else {
		beneficiary = *author
	}

	var registeredAddressMap map[string]*common.Address
	if registeredAddresses != nil {
		registeredAddressMap = registeredAddresses.GetRegisteredAddressMap()
	}

	return vm.Context{
		CanTransfer:          CanTransfer,
		Transfer:             Transfer,
		GetHash:              GetHashFn(header, chain),
		GetCoinbase:          GetCoinbaseFn(header, chain),
		Origin:               msg.From(),
		Coinbase:             beneficiary,
		BlockNumber:          new(big.Int).Set(header.Number),
		Time:                 new(big.Int).Set(header.Time),
		Difficulty:           new(big.Int).Set(header.Difficulty),
		GasLimit:             header.GasLimit,
		GasPrice:             new(big.Int).Set(msg.GasPrice()),
		RegisteredAddressMap: registeredAddressMap,
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

// GetCoinbaseFn returns a GetCoinbaseFunc which retrieves the coinbase by block number
func GetCoinbaseFn(ref *types.Header, chain ChainContext) func(n uint64) common.Address {
	var cache map[uint64]common.Address

	return func(n uint64) common.Address {
		// If there's no address cache yet, make one
		if cache == nil {
			cache = map[uint64]common.Address{
				ref.Number.Uint64(): ref.Coinbase,
			}
		}
		// Try to fulfill the request from the cache
		if address, ok := cache[n]; ok {
			return address
		}
		// Not cached, iterate the blocks and cache the addresses
		for header := chain.GetHeader(ref.ParentHash, ref.Number.Uint64()-1); header != nil; header = chain.GetHeader(header.ParentHash, header.Number.Uint64()-1) {
			cache[header.Number.Uint64()] = header.Coinbase
			if n == header.Number.Uint64() {
				return header.Coinbase
			}
		}

		// Like GetHashFn we'll just return an empty address if we can't find it
		return common.Address{}
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
	chainConfig      *params.ChainConfig // The config object of the eth object
	chainContext     ChainContext
	regAdd           *RegisteredAddresses
	vmConfig         vm.Config
	getState         func(header *types.Header) (*state.StateDB, error)
	getCurrentHeader func() (*types.Header, error)
}

func (iEvmH *InternalEVMHandler) MakeCall(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state *state.StateDB) (uint64, error) {
	// Normally, when making an evm call, we should use the current block's state.  However,
	// there are times (e.g. retrieving the set of validators when an epoch ends) that we need
	// to call the evm using the currently mined block.  In that case, the header and state params
	// will be non nil.

	if header == nil {
		var err error
		header, err = iEvmH.getCurrentHeader()
		if err != nil {
			return 0, err
		}
	}

	if state == nil {
		var err error
		state, err = iEvmH.getState(header)
		if err != nil {
			log.Error("Error in retrieving the state from the blockchain")
			return 0, err
		}
	}

	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	msg := types.NewMessage(common.HexToAddress("0x0"), nil, 0, common.Big0, 0, common.Big0, nil, []byte{}, false)
	context := NewEVMContext(msg, header, iEvmH.chainContext, nil, iEvmH.regAdd)
	evm := vm.NewEVM(context, state, iEvmH.chainConfig, iEvmH.vmConfig)

	zeroCaller := vm.AccountRef(common.HexToAddress("0x0"))
	return evm.ABIStaticCall(zeroCaller, scAddress, abi, funcName, args, returnObj, gas)
}

func (iEvmH *InternalEVMHandler) SetRegisteredAddresses(regAdd *RegisteredAddresses) {
	iEvmH.regAdd = regAdd
}

func NewInternalEVMHandler(chainConfig *params.ChainConfig, chainContext ChainContext, vmConfig vm.Config, getState func(header *types.Header) (*state.StateDB, error)) *InternalEVMHandler {
	iEvmH := InternalEVMHandler{
		chainConfig:  chainConfig,
		chainContext: chainContext,
		vmConfig:     vmConfig,
		getState:     getState,
	}
	return &iEvmH
}
