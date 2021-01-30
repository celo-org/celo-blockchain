// Copyright 2015 The go-ethereum Authors
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
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/misc"
	"github.com/celo-org/celo-blockchain/contract_comm/random"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/metrics"
	"github.com/celo-org/celo-blockchain/params"
	lru "github.com/hashicorp/golang-lru"
)

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to the processor (coinbase).
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts types.Receipts
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*types.Log
		gp       = new(GasPool).AddGas(CalcGasLimit(block, statedb))
	)
	// Mutate the block and state according to any hard-fork specs
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}

	if random.IsRunning() {
		author, err := p.bc.Engine().Author(header)
		if err != nil {
			return nil, nil, 0, err
		}

		err = random.RevealAndCommit(block.Randomness().Revealed, block.Randomness().Committed, author, header, statedb)
		if err != nil {
			return nil, nil, 0, err
		}
		// always true (EIP158)
		statedb.IntermediateRoot(true)
	}
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		statedb.Prepare(tx.Hash(), block.Hash(), i)
		receipt, err := ApplyTransaction(p.config, p.bc, nil, gp, statedb, header, tx, usedGas, cfg)
		if err != nil {
			return nil, nil, 0, err
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	statedb.Prepare(common.Hash{}, block.Hash(), len(block.Transactions()))
	p.engine.Finalize(p.bc, header, statedb, block.Transactions())

	if len(statedb.GetLogs(common.Hash{})) > 0 {
		receipt := types.NewReceipt(nil, false, 0)
		receipt.Logs = statedb.GetLogs(common.Hash{})
		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		for i := range receipt.Logs {
			receipt.Logs[i].TxIndex = uint(len(receipts))
			receipt.Logs[i].TxHash = block.Hash()
		}
		receipts = append(receipts, receipt)
	}

	return receipts, allLogs, *usedGas, nil
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(config *params.ChainConfig, bc vm.ChainContext, txFeeRecipient *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return nil, err
	}

	// Create a new context to be used in the EVM environment
	context := vm.NewEVMContext(msg, header, bc, txFeeRecipient)
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	vmenv := vm.NewEVM(context, statedb, config, cfg)

	// Apply the transaction to the current state (included in the env)
	result, err := ApplyMessage(vmenv, msg, gp)
	if err != nil {
		return nil, err
	}
	// Update the state with pending changes
	var root []byte
	if config.IsByzantium(header.Number) {
		statedb.Finalise(true)
	} else {
		root = statedb.IntermediateRoot(config.IsEIP158(header.Number)).Bytes()
	}
	*usedGas += result.UsedGas

	// Create a new receipt for the transaction, storing the intermediate root and gas used by the tx
	// based on the eip phase, we're passing whether the root touch-delete accounts.
	receipt := types.NewReceipt(root, result.Failed(), *usedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = result.UsedGas
	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
	}
	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = statedb.GetLogs(tx.Hash())
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	receipt.BlockHash = statedb.BlockHash()
	receipt.BlockNumber = header.Number
	receipt.TransactionIndex = uint(statedb.TxIndex())

	return receipt, err
}

// ---------------------------------------------------------
// CachingStateProcessor
// ---------------------------------------------------------

const (
	stateCacheLimit = 256
)

// StateProcessResult represents processing results from StateProcessor.
type StateProcessResult struct {
	State    *state.StateDB
	Receipts types.Receipts
	Logs     []*types.Log
	UsedGas  uint64
	err      error
}

type StateProcessKey struct {
	SealHash common.Hash
	Root     common.Hash
}

// CachingStateProcessor incorporates StateProcessor and StateProcessCache.
type CachingStateProcessor struct {
	processor *StateProcessor // Actual underlying processor
	cache     *lru.Cache      // Caching StateProcessor, only use sealHash -> StateProcessResult as key-value pair.

	processorRequestGauge metrics.Gauge // Gauge for total number of requests to StateProcessor.Process
	cacheHitGauge         metrics.Gauge // Gauge for cache hit
	cacheLenGauge         metrics.Gauge // Gauge for cache length
}

// NewCachingStateProcessor initialises a new CachingStateProcessor.
func NewCachingStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *CachingStateProcessor {
	cache, _ := lru.New(stateCacheLimit)
	return &CachingStateProcessor{
		processor:             NewStateProcessor(config, bc, engine),
		cache:                 cache,
		processorRequestGauge: metrics.NewRegisteredGauge("core/caching_state_processor/processorRequest", nil),
		cacheHitGauge:         metrics.NewRegisteredGauge("core/caching_state_processor/cacheHit", nil),
		cacheLenGauge:         metrics.NewRegisteredGauge("core/caching_state_processor/cacheLen", nil),
	}
}

// Process first queries the cache, if miss, then invokes the StateProcessor.
func (cp *CachingStateProcessor) Process(block *types.Block, state *state.StateDB, cfg vm.Config) (receipts types.Receipts, logs []*types.Log, usedGas uint64, err error) {
	cp.processorRequestGauge.Inc(1)
	cp.cacheLenGauge.Update(int64(cp.cache.Len()))

	// First query the cache
	key := &StateProcessKey{
		SealHash: cp.processor.engine.SealHash(block.Header()),
		Root:     state.IntermediateRoot(true),
	}
	r, ok := cp.Get(key)
	if ok {
		*state = *r.State // replace where state points to
		return r.Receipts, r.Logs, r.UsedGas, r.err
	}
	// If no cache hit, then do actual processing
	receipts, logs, usedGas, err = cp.processor.Process(block, state, cfg)
	cp.cache.Add(key,
		&StateProcessResult{
			State:    state,
			Receipts: receipts,
			Logs:     logs,
			UsedGas:  usedGas,
			err:      err,
		})
	return
}

// Add adds a deep copied StateProcessResult.
func (cp *CachingStateProcessor) Add(key *StateProcessKey, result *StateProcessResult) {
	cp.cache.Add(key, deepCopy(result))
}

// Get retrieves a StateProcessResult, and returns a deep copy of it.
func (cp *CachingStateProcessor) Get(key *StateProcessKey) (*StateProcessResult, bool) {
	if value, ok := cp.cache.Get(key); ok {
		r := value.(*StateProcessResult)
		return deepCopy(r), true
	}
	return nil, false
}

//deepCopy will deep copy a StateProcessResult.
func deepCopy(result *StateProcessResult) *StateProcessResult {
	var (
		cpyReceipts = make([]*types.Receipt, len(result.Receipts))
		cpyLogs     []*types.Log
	)
	// Deep copy receipts
	for i, receipt := range result.Receipts {
		cpyReceipts[i] = new(types.Receipt)
		*cpyReceipts[i] = *receipt
		// Deep copy logs
		for _, log := range receipt.Logs {
			cpyLog := new(types.Log)
			*cpyLog = *log
			cpyLogs = append(cpyLogs, cpyLog)
		}
	}
	return &StateProcessResult{
		State:    result.State.Copy(),
		Receipts: cpyReceipts,
		Logs:     cpyLogs,
		UsedGas:  result.UsedGas,
		err:      result.err,
	}
}

//// StateProcessCache defines a cache for StateProcessResult which results from StateProcessor
//type Cache interface {
//	// Add puts StateProcessResult, using block's sealHash as key
//	Add(sealHash common.Hash, result *StateProcessResult)
//
//	// Get retrieves StateProcessResult, using block's sealHash as key
//	Get(sealHash common.Hash) (*StateProcessResult, bool)
//}

//// StateProcessCache implements a thread safe StateProcessCache
//type StateProcessCache struct {
//	cache   map[common.Hash]*StateProcessResult
//	cacheMu *sync.RWMutex
//}

//// NewBlockProcessResultCache returns a StateProcessCache
//func NewBlockProcessResultCache() *StateProcessCache {
//	return &StateProcessCache{
//		cache:   make(map[common.Hash]*StateProcessResult),
//		cacheMu: new(sync.RWMutex),
//	}
//}

//// Add adds a value to the cache
//func (pc *StateProcessCache) Add(sealHash common.Hash, result *StateProcessResult) {
//	pc.cacheMu.Lock()
//	defer pc.cacheMu.Unlock()
//
//	pc.clear()
//	pc.cache[sealHash] = result
//}
//
//// Get gets a value from the cache
//func (pc *StateProcessCache) Get(sealHash common.Hash) (result *StateProcessResult, ok bool) {
//	pc.cacheMu.RLock()
//	defer pc.cacheMu.RUnlock()
//
//	result, ok = pc.cache[sealHash]
//	return
//}
//
//// clear removes stale entries in the cache
//func (pc *StateProcessCache) clear() {
//	for hash, result := range pc.cache {
//		if result.blockNumber+staleThreshold <= number {
//			delete(w.pendingTasks, h)
//		}
//	}
//}
