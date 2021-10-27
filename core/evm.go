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

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/core/vm/vmcontext"
	"github.com/celo-org/celo-blockchain/params"
)

// ChainContext supports retrieving chain data and consensus parameters
// from the blockchain to be used during transaction processing.
type ChainContext interface {
	// Engine retrieves the blockchain's consensus engine.
	Engine() consensus.Engine

	// GetHeader returns the hash corresponding to the given hash and number.
	GetHeader(common.Hash, uint64) *types.Header

	// GetHeaderByNumber returns the hash corresponding number.
	// in the correct fork.
	GetHeaderByNumber(uint64) *types.Header

	// Config returns the blockchain's chain configuration
	Config() *params.ChainConfig
}

// NewEVMBlockContext creates a new context for use in the EVM.
func NewEVMBlockContext(header *types.Header, chain ChainContext, txFeeRecipient *common.Address, baseFee *big.Int) vm.BlockContext {
	return vmcontext.NewBlockContext(header, chain, txFeeRecipient, baseFee)
}

// NewEVMTxContext creates a new transaction context for a single transaction.
func NewEVMTxContext(msg Message) vm.TxContext {
	return vm.TxContext{
		Origin:   msg.From(),
		GasPrice: new(big.Int).Set(msg.GasPrice()),
	}
}
