// Copyright 2019 The go-ethereum Authors
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
	"sync/atomic"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

// statePrefetcher is a basic Prefetcher, which blindly executes a block on top
// of an arbitrary state with the goal of prefetching potentially useful state
// data from disk before the main block processor start executing.
type statePrefetcher struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// newStatePrefetcher initialises a new statePrefetcher.
func newStatePrefetcher(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *statePrefetcher {
	return &statePrefetcher{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Prefetch processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb, but any changes are discarded. The
// only goal is to pre-cache transaction signatures and state trie nodes.
func (p *statePrefetcher) Prefetch(block *types.Block, statedb *state.StateDB, cfg vm.Config, interrupt *uint32) {
	var (
		header   = block.Header()
		vmRunner = p.bc.NewEVMRunner(header, statedb)
		gaspool  = new(GasPool).AddGas(blockchain_parameters.GetBlockGasLimitOrDefault(vmRunner))
		baseFee  *big.Int
		sysCtx   *SysContractCallCtx
	)
	// Iterate over and process the individual transactions
	byzantium := p.config.IsByzantium(block.Number())
	espresso := p.bc.chainConfig.IsEHardfork(block.Number())
	if espresso {
		sysCtx = NewSysContractCallCtx(p.bc.NewEVMRunner(header, statedb))
	}
	for i, tx := range block.Transactions() {
		// If block precaching was interrupted, abort
		if interrupt != nil && atomic.LoadUint32(interrupt) == 1 {
			return
		}
		// Block precaching permitted to continue, execute the transaction
		statedb.Prepare(tx.Hash(), i)
		if espresso {
			baseFee = sysCtx.GetGasPriceMinimum(tx.FeeCurrency())
		}
		if err := precacheTransaction(p.config, p.bc, nil, gaspool, statedb, header, tx, cfg, baseFee); err != nil {
			return // Ugh, something went horribly wrong, bail out
		}
		// If we're pre-byzantium, pre-load trie nodes for the intermediate root
		if !byzantium {
			statedb.IntermediateRoot(true)
		}
	}
	// If were post-byzantium, pre-load trie nodes for the final root hash
	if byzantium {
		statedb.IntermediateRoot(true)
	}
}

// precacheTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. The goal is not to execute
// the transaction successfully, rather to warm up touched data slots.
func precacheTransaction(config *params.ChainConfig, bc *BlockChain, author *common.Address, gaspool *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, cfg vm.Config, baseFee *big.Int) error {
	// Convert the transaction into an executable message and pre-cache its sender
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number), baseFee)
	if err != nil {
		return err
	}
	// Create the EVM and execute the transaction
	context := NewEVMBlockContext(header, bc, author)
	txContext := NewEVMTxContext(msg)
	vm := vm.NewEVM(context, txContext, statedb, config, cfg)

	var sysCtx *SysContractCallCtx
	if config.IsEHardfork(header.Number) {
		sysVmRunner := bc.NewEVMRunner(header, statedb)
		sysCtx = NewSysContractCallCtx(sysVmRunner)
	}
	_, err = ApplyMessage(vm, msg, gaspool, bc.NewEVMRunner(header, statedb), sysCtx)
	return err
}
