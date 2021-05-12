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

package miner

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/contract_comm/currency"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/event"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
	"github.com/celo-org/celo-blockchain/params"
)

const (
	// resultQueueSize is the size of channel listening to sealing result.
	resultQueueSize = 10

	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10

	// staleThreshold is the maximum depth of the acceptable stale block.
	staleThreshold = 7
)

// Gauge used to measure block finalization time from created to after written to chain.
var blockFinalizationTimeGauge = metrics.NewRegisteredGauge("miner/block/finalizationTime", nil)

// task contains all information for consensus engine sealing and result submitting.
type task struct {
	receipts  []*types.Receipt
	state     *state.StateDB
	block     *types.Block
	createdAt time.Time
}

// worker is the main object which takes care of submitting new work to consensus engine
// and gathering the sealing result.
type worker struct {
	config      *Config
	chainConfig *params.ChainConfig
	engine      consensus.Engine
	eth         Backend
	chain       *core.BlockChain

	// Feeds
	pendingLogsFeed event.Feed

	// Subscriptions
	mux          *event.TypeMux
	chainHeadCh  chan core.ChainHeadEvent
	chainHeadSub event.Subscription

	// Channels
	resultCh chan *types.Block
	startCh  chan struct{}
	exitCh   chan struct{}

	// Previous sent task
	prevTaskStopCh chan struct{}
	prevSealHash   common.Hash

	mu             sync.RWMutex // The lock used to protect the validator, txFeeRecipient and extra fields
	validator      common.Address
	txFeeRecipient common.Address
	extra          []byte

	pendingMu    sync.RWMutex
	pendingTasks map[common.Hash]*task

	snapshotMu    sync.RWMutex // The lock used to protect the block snapshot and state snapshot
	snapshotBlock *types.Block
	snapshotState *state.StateDB

	// atomic status counters
	running int32 // The indicator whether the consensus engine is running or not.

	// Test hooks
	newTaskHook  func(*task)      // Method to call upon receiving a new sealing task.
	skipSealHook func(*task) bool // Method to decide whether skipping the sealing.
	fullTaskHook func()           // Method to call before pushing the full sealing task.

	// Needed for randomness
	db ethdb.Database

	blockConstructGauge metrics.Gauge
}

func newWorker(config *Config, chainConfig *params.ChainConfig, engine consensus.Engine, eth Backend, mux *event.TypeMux, db ethdb.Database, init bool) *worker {
	worker := &worker{
		config:              config,
		chainConfig:         chainConfig,
		engine:              engine,
		eth:                 eth,
		mux:                 mux,
		chain:               eth.BlockChain(),
		pendingTasks:        make(map[common.Hash]*task),
		chainHeadCh:         make(chan core.ChainHeadEvent, chainHeadChanSize),
		resultCh:            make(chan *types.Block, resultQueueSize),
		exitCh:              make(chan struct{}),
		startCh:             make(chan struct{}, 1),
		db:                  db,
		blockConstructGauge: metrics.NewRegisteredGauge("miner/worker/block_construct", nil),
	}
	// Subscribe events for blockchain
	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)

	go worker.mainLoop()
	go worker.resultLoop()

	// Submit first work to initialize pending state.
	if init {
		worker.startCh <- struct{}{}
	}
	return worker
}

// setValidator sets the validator address that signs messages and commits randomness
func (w *worker) setValidator(addr common.Address) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.validator = addr
}

// setTxFeeRecipient sets the address to receive tx fees, stored in header.Coinbase
func (w *worker) setTxFeeRecipient(addr common.Address) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.txFeeRecipient = addr
}

// setExtra sets the content used to initialize the block extra field.
func (w *worker) setExtra(extra []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.extra = extra
}

// pending returns the pending state and corresponding block.
func (w *worker) pending() (*types.Block, *state.StateDB) {
	// return a snapshot to avoid contention on currentMu mutex
	w.snapshotMu.RLock()
	defer w.snapshotMu.RUnlock()
	if w.snapshotState == nil {
		return nil, nil
	}
	return w.snapshotBlock, w.snapshotState.Copy()
}

// pendingBlock returns pending block.
func (w *worker) pendingBlock() *types.Block {
	// return a snapshot to avoid contention on currentMu mutex
	w.snapshotMu.RLock()
	defer w.snapshotMu.RUnlock()
	return w.snapshotBlock
}

// start sets the running status as 1 and triggers new work submitting.
func (w *worker) start() {
	atomic.StoreInt32(&w.running, 1)
	w.startCh <- struct{}{}

	if istanbul, ok := w.engine.(consensus.Istanbul); ok {
		istanbul.SetBlockProcessors(w.chain.HasBadBlock,
			func(block *types.Block, state *state.StateDB) (types.Receipts, []*types.Log, uint64, error) {
				return w.chain.Processor().Process(block, state, *w.chain.GetVMConfig())
			},
			func(block *types.Block, state *state.StateDB, receipts types.Receipts, usedGas uint64) error {
				return w.chain.Validator().ValidateState(block, state, receipts, usedGas)
			})
		if istanbul.IsPrimary() {
			istanbul.StartValidating()
		}
	}
}

// stop sets the running status as 0.
func (w *worker) stop() {
	w.interruptSealingTask()
	atomic.StoreInt32(&w.running, 0)

	if istanbul, ok := w.engine.(consensus.Istanbul); ok {
		istanbul.StopValidating()
	}
}

// isRunning returns an indicator whether worker is running or not.
func (w *worker) isRunning() bool {
	return atomic.LoadInt32(&w.running) == 1
}

// close terminates all background threads maintained by the worker.
// Note the worker does not support being closed multiple times.
func (w *worker) close() {
	w.interruptSealingTask()
	close(w.exitCh)
}

func (w *worker) createTxCmp() func(tx1 *types.Transaction, tx2 *types.Transaction) int {
	// TODO specify header & state
	currencyManager := currency.NewManager(nil, nil)

	return func(tx1 *types.Transaction, tx2 *types.Transaction) int {
		return currencyManager.CmpValues(tx1.GasPrice(), tx1.FeeCurrency(), tx2.GasPrice(), tx2.FeeCurrency())
	}
}

// mainLoop is a standalone goroutine to regenerate the sealing task based on the received event.
func (w *worker) mainLoop() {
	defer w.chainHeadSub.Unsubscribe()

	// clearPending cleans the stale pending tasks.
	clearPending := func(number uint64) {
		w.pendingMu.Lock()
		for h, t := range w.pendingTasks {
			if t.block.NumberU64()+staleThreshold <= number {
				delete(w.pendingTasks, h)
			}
		}
		w.pendingMu.Unlock()
	}

	// Context and cancel function for the currently executing block construction
	var ctx context.Context
	var cancel context.CancelFunc
	var wg sync.WaitGroup

	for {
		select {
		case <-w.startCh:
			clearPending(w.chain.CurrentBlock().NumberU64())
			if cancel != nil {
				cancel()
			}
			if h, ok := w.engine.(consensus.Handler); ok {
				h.NewWork()
			}
			ctx, cancel = context.WithCancel(context.Background())
			wg.Add(1)
			go func() {
				w.constructAndSubmitNewBlock(ctx)
				wg.Done()
			}()

		case head := <-w.chainHeadCh:
			headNumber := head.Block.NumberU64()
			clearPending(headNumber)
			if cancel != nil {
				cancel()
			}
			if h, ok := w.engine.(consensus.Handler); ok {
				h.NewWork()
			}
			ctx, cancel = context.WithCancel(context.Background())
			wg.Add(1)
			go func() {
				w.constructAndSubmitNewBlock(ctx)
				wg.Done()
			}()

		// System stopped
		case <-w.exitCh:
			if cancel != nil {
				cancel()
			}
			return
		case <-w.chainHeadSub.Err():
			if cancel != nil {
				cancel()
			}
			return
		}
	}
}

// interrupt aborts the in-flight sealing task.
func (w *worker) interruptSealingTask() {
	if w.prevTaskStopCh != nil {
		close(w.prevTaskStopCh)
		w.prevTaskStopCh = nil
	}
}

func (w *worker) handleTask(task *task) {
	if w.newTaskHook != nil {
		w.newTaskHook(task)
	}

	// Reject duplicate sealing work due to resubmitting.
	sealHash := w.engine.SealHash(task.block.Header())
	if sealHash == w.prevSealHash {
		return
	}
	// Interrupt previous sealing operation
	w.interruptSealingTask()
	w.prevTaskStopCh, w.prevSealHash = make(chan struct{}), sealHash

	if w.skipSealHook != nil && w.skipSealHook(task) {
		return
	}

	w.pendingMu.Lock()
	w.pendingTasks[w.engine.SealHash(task.block.Header())] = task
	w.pendingMu.Unlock()

	if err := w.engine.Seal(w.chain, task.block, w.resultCh, w.prevTaskStopCh); err != nil {
		log.Warn("Block sealing failed", "err", err)
	}
}

// resultLoop is a standalone goroutine to handle sealing result submitting
// and flush relative data to the database.
func (w *worker) resultLoop() {
	for {
		select {
		case block := <-w.resultCh:
			// Short circuit when receiving empty result.
			if block == nil {
				continue
			}
			// Short circuit when receiving duplicate result caused by resubmitting.
			if w.chain.HasBlock(block.Hash(), block.NumberU64()) {
				continue
			}
			var (
				sealhash = w.engine.SealHash(block.Header())
				hash     = block.Hash()
			)
			w.pendingMu.RLock()
			task, exist := w.pendingTasks[sealhash]
			w.pendingMu.RUnlock()
			if !exist {
				log.Error("Block found but no relative pending task", "number", block.Number(), "sealhash", sealhash, "hash", hash)
				continue
			}
			// Different block could share same sealhash, deep copy here to prevent write-write conflict.
			var (
				receipts = make([]*types.Receipt, len(task.receipts))
				logs     []*types.Log
			)
			for i, receipt := range task.receipts {
				// add block location fields
				receipt.BlockHash = hash
				receipt.BlockNumber = block.Number()
				receipt.TransactionIndex = uint(i)

				receipts[i] = new(types.Receipt)
				*receipts[i] = *receipt
				// Update the block hash in all logs since it is now available and not when the
				// receipt/log of individual transactions were created.
				for _, log := range receipt.Logs {
					log.BlockHash = hash
					// Handle block finalization receipt
					if (log.TxHash == common.Hash{}) {
						log.TxHash = hash
					}
				}
				logs = append(logs, receipt.Logs...)
			}
			// Commit block and state to database.
			_, err := w.chain.WriteBlockWithState(block, receipts, logs, task.state, true)
			if err != nil {
				log.Error("Failed writing block to chain", "err", err)
				continue
			}
			blockFinalizationTimeGauge.Update(time.Now().UnixNano() - int64(block.Time())*1000000000)
			log.Info("Successfully sealed new block", "number", block.Number(), "sealhash", sealhash, "hash", hash,
				"elapsed", common.PrettyDuration(time.Since(task.createdAt)))

			// Broadcast the block and announce chain insertion event
			w.mux.Post(core.NewMinedBlockEvent{Block: block})

		case <-w.exitCh:
			return
		}
	}
}

// updateSnapshot updates pending snapshot block and state.
// Note this function assumes the current variable is thread safe.
func (w *worker) updateSnapshot(b *blockState) {
	w.snapshotMu.Lock()
	defer w.snapshotMu.Unlock()

	w.snapshotBlock = types.NewBlock(
		b.header,
		b.txs,
		b.receipts,
		b.randomness,
	)

	w.snapshotState = b.state.Copy()
}
