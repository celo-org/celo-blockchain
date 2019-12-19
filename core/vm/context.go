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

package vm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	//FromFrontier() (common.Address, error)
	To() *common.Address

	GasPrice() *big.Int
	Gas() uint64

	// FeeCurrency specifies the currency for gas and gateway fees.
	// nil correspond to Celo Gold (native currency).
	// All other values should correspond to ERC20 contract addresses extended to be compatible with gas payments.
	FeeCurrency() *common.Address
	GatewayFeeRecipient() *common.Address
	GatewayFee() *big.Int
	Value() *big.Int

	Nonce() uint64
	CheckNonce() bool
	Data() []byte
}

// ChainContext supports retrieving chain data and consensus parameters
// from the blockchain to be used during transaction processing.
type ChainContext interface {
	// Engine retrieves the blockchain's consensus engine.
	Engine() consensus.Engine

	// GetHeader returns the hash corresponding to the given hash and number.
	GetHeader(common.Hash, uint64) *types.Header

	// GetVMConfig returns the node's vm configuration
	GetVMConfig() *Config

	CurrentHeader() *types.Header

	State() (*state.StateDB, error)

	// Config returns the blockchain's chain configuration
	Config() *params.ChainConfig
}

// NewEVMContext creates a new context for use in the EVM.
func NewEVMContext(msg Message, header *types.Header, chain ChainContext, author *common.Address) Context {
	// If we don't have an explicit author (i.e. not mining), extract from the header
	var beneficiary common.Address
	if author == nil {
		beneficiary, _ = chain.Engine().Author(header) // Ignore error, we're past header validation
	} else {
		beneficiary = *author
	}

	var engine consensus.Engine
	if chain != nil {
		engine = chain.Engine()
	}

	return Context{
		CanTransfer:         CanTransfer,
		Transfer:            Transfer,
		GetHash:             GetHashFn(header, chain),
		GetParentSealBitmap: GetParentSealBitmapFn(header, chain),
		VerifySeal:          VerifySealFn(header, chain),
		Origin:              msg.From(),
		Coinbase:            beneficiary,
		BlockNumber:         new(big.Int).Set(header.Number),
		Time:                new(big.Int).Set(header.Time),
		Difficulty:          new(big.Int).Set(header.Difficulty),
		GasLimit:            header.GasLimit,
		GasPrice:            new(big.Int).Set(msg.GasPrice()),
		Engine:              engine,
	}
}

// GetHashFn returns a GetHashFunc which retrieves header hashes by number
func GetHashFn(ref *types.Header, chain ChainContext) func(uint64) common.Hash {
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
		// Not cached, iterate the blocks and cache the hashes (up to a limit of 256)
		for i, header := 0, chain.GetHeader(ref.ParentHash, ref.Number.Uint64()-1); header != nil && i <= 256; i, header = i+1, chain.GetHeader(header.ParentHash, header.Number.Uint64()-1) {
			cache[header.Number.Uint64()-1] = header.ParentHash
			if n == header.Number.Uint64()-1 {
				return header.ParentHash
			}
		}
		return common.Hash{}
	}
}

// CanTransfer checks whether there are enough funds in the address' account to make a transfer.
// This does not take the necessary gas into account to make the transfer valid.
func CanTransfer(db StateDB, addr common.Address, amount *big.Int) bool {
	return db.GetBalance(addr).Cmp(amount) >= 0
}

// Transfer subtracts amount from sender and adds amount to recipient using the given Db
func Transfer(db StateDB, sender, recipient common.Address, amount *big.Int) {
	db.SubBalance(sender, amount)
	db.AddBalance(recipient, amount)
}

// GetParentSealFn returns a GetParentSeal function that returns the aggregated parent seal at the given block number.
// Note: Unless previously cached, retrieves every block between the reference block and n.
func GetParentSealBitmapFn(ref *types.Header, chain ChainContext) func(uint64) *big.Int {
	var cache map[uint64]*big.Int

	return func(n uint64) *big.Int {
		// If the block is the unsealed reference block or later, return nil.
		if n >= ref.Number.Uint64() {
			return nil
		}

		// If there's no cache yet, make one
		if cache == nil {
			cache = make(map[uint64]*big.Int)
		} else {
			// Try to fulfill the request from the cache
			if bitmap, ok := cache[n]; ok {
				return bitmap
			}
		}

		// Not cached, iterate the blocks and cache the hashes (not limited here)
		for header := chain.GetHeader(ref.ParentHash, ref.Number.Uint64()-1); header != nil; header = chain.GetHeader(header.ParentHash, header.Number.Uint64()-1) {
			var bitmap *big.Int
			istanbulExtra, err := types.ExtractIstanbulExtra(header)
			if err == nil {
				bitmap = istanbulExtra.AggregatedSeal.Bitmap
				cache[header.Number.Uint64()] = bitmap
			}
			if n == header.Number.Uint64() {
				return bitmap
			}
		}
		return nil
	}
}

// VerifySealFn returns a function which returns true when the given header has a verifiable seal.
func VerifySealFn(ref *types.Header, chain ChainContext) func(*types.Header) bool {
	return func(header *types.Header) bool {
		// If the block is later than the unsealed reference block, return false.
		if header.Number.Cmp(ref.Number) > 0 {
			return false
		}

		// FIXME: Implementation currently relies on the Istanbul engine's internal view of the
		// chain, so return false if this is not an Istanbul chain. As a consequence of this the
		// seal is always verified against the canonical chain, which makes behavior undefined if
		// this function is evaluated on a chain which does not have the highest total difficulty.
		if chain.Config().Istanbul == nil {
			return false
		}

		// Submit the header to the engine's seal verification function.
		return chain.Engine().VerifySeal(nil, header) == nil
	}
}
