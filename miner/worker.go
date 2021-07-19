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
	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096

	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10
)

// callBackEngine is a subset of the consensus.Istanbul interface. It is used over consensus.Istanbul to enable sealing
// for the MockEngine (which implements this and the engine interface, but not the full istanbul interface).
type callBackEngine interface {
	// SetCallBacks sets call back functions
	SetCallBacks(hasBadBlock func(common.Hash) bool,
		processBlock func(*types.Block, *state.StateDB) (types.Receipts, []*types.Log, uint64, error),
		validateState func(*types.Block, *state.StateDB, types.Receipts, uint64) error,
		onNewConsensusBlock func(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB)) error
}

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
	txsCh        chan core.NewTxsEvent
	txsSub       event.Subscription
	chainHeadCh  chan core.ChainHeadEvent
	chainHeadSub event.Subscription

	// Channels
	startCh chan struct{}
	exitCh  chan struct{}

	mu             sync.RWMutex // The lock used to protect the validator, txFeeRecipient and extra fields
	validator      common.Address
	txFeeRecipient common.Address
	extra          []byte

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

	blockConstructGauge         metrics.Gauge
	blockConstructTxsGauge      metrics.Gauge
	blockConstructFinalizeGauge metrics.Gauge
}

func newWorker(config *Config, chainConfig *params.ChainConfig, engine consensus.Engine, eth Backend, mux *event.TypeMux, db ethdb.Database) *worker {
	worker := &worker{
		config:                      config,
		chainConfig:                 chainConfig,
		engine:                      engine,
		eth:                         eth,
		mux:                         mux,
		chain:                       eth.BlockChain(),
		txsCh:                       make(chan core.NewTxsEvent, txChanSize),
		chainHeadCh:                 make(chan core.ChainHeadEvent, chainHeadChanSize),
		exitCh:                      make(chan struct{}),
		startCh:                     make(chan struct{}, 1),
		db:                          db,
		blockConstructGauge:         metrics.NewRegisteredGauge("miner/worker/block_construct", nil),
		blockConstructTxsGauge:      metrics.NewRegisteredGauge("miner/worker/block_construct_txs", nil),
		blockConstructFinalizeGauge: metrics.NewRegisteredGauge("miner/worker/block_construct_finalize", nil),
	}
	// Subscribe NewTxsEvent for tx pool
	worker.txsSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh)
	// Subscribe events for blockchain
	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)

	go worker.mainLoop()

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

	if cbEngine, ok := w.engine.(callBackEngine); ok {
		cbEngine.SetCallBacks(w.chain.HasBadBlock,
			func(block *types.Block, state *state.StateDB) (types.Receipts, []*types.Log, uint64, error) {
				return w.chain.Processor().Process(block, state, *w.chain.GetVMConfig())
			},
			w.chain.Validator().ValidateState,
			func(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB) {
				if err := w.chain.InsertPreprocessedBlock(block, receipts, logs, state); err != nil {
					if err == core.ErrNotHeadBlock {
						log.Warn("Tried to insert duplicated produced block", "blockNumber", block.Number(), "hash", block.Hash(), "err", err)
					} else {
						log.Error("Failed to insert produced block", "blockNumber", block.Number(), "hash", block.Hash(), "err", err)
					}
					return
				}
				log.Info("Successfully produced new block", "number", block.Number(), "hash", block.Hash())

				if err := w.mux.Post(core.NewMinedBlockEvent{Block: block}); err != nil {
					log.Error("Error when posting NewMinedBlockEvent", "err", err)
				}
			})
	}

	if istanbul, ok := w.engine.(consensus.Istanbul); ok {

		if istanbul.IsPrimary() {
			istanbul.StartValidating()
		}
	}
}

// stop sets the running status as 0.
func (w *worker) stop() {
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
	close(w.exitCh)
}

// constructAndSubmitNewBlock constructs a new block and if the worker is running, submits
// a task to the engine
func (w *worker) constructAndSubmitNewBlock(ctx context.Context) {
	start := time.Now()

	// Initialize the block.
	b, err := prepareBlock(w)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
	w.updatePendingBlock(b)

	// TODO: worker based adaptive sleep with this delay
	// wait for the timestamp of header, use this to adjust the block period
	delay := time.Until(time.Unix(int64(b.header.Time), 0))
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return
	}

	defer func(start time.Time) { w.blockConstructGauge.Update(time.Since(start).Nanoseconds()) }(time.Now())
	err = b.selectAndApplyTransactions(ctx, w)
	if err != nil {
		log.Error("Failed to apply transactions to the block", "err", err)
		return
	}
	w.updatePendingBlock(b)

	block, err := b.finalizeAndAssemble(w)
	if err != nil {
		log.Error("Failed to finalize and assemble the block", "err", err)
		return
	}
	w.updatePendingBlock(b)
	if w.isRunning() {
		if w.fullTaskHook != nil {
			w.fullTaskHook()
		}
		w.submitTaskToEngine(&task{receipts: b.receipts, state: b.state, block: block, createdAt: time.Now()})

		feesCelo := totalFees(block, b.receipts)
		log.Info("Commit new mining work", "number", block.Number(), "txs", b.tcount, "gas", block.GasUsed(),
			"fees", feesCelo, "elapsed", common.PrettyDuration(time.Since(start)))

	}
}

// constructPendingStateBlock constructs a new block and keeps applying new transactions to it.
// until it is full or the context is cancelled.
func (w *worker) constructPendingStateBlock(ctx context.Context, txsCh chan core.NewTxsEvent) {
	// Initialize the block.
	b, err := prepareBlock(w)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
	w.updatePendingBlock(b)

	err = b.selectAndApplyTransactions(ctx, w)
	if err != nil {
		log.Error("Failed to apply transactions to the block", "err", err)
		return
	}
	w.updatePendingBlock(b)

	w.mu.RLock()
	txFeeRecipient := w.txFeeRecipient
	if !w.chainConfig.IsDonut(b.header.Number) && w.txFeeRecipient != w.validator {
		txFeeRecipient = w.validator
		log.Warn("TxFeeRecipient and Validator flags set before split etherbase fork is active. Defaulting to the given validator address for the coinbase.")
	}
	w.mu.RUnlock()

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-txsCh:
			if !w.isRunning() {
				// If block is already full, abort
				if gp := b.gasPool; gp != nil && gp.Gas() < params.TxGas {
					return
				}

				txs := make(map[common.Address]types.Transactions)
				for _, tx := range ev.Txs {
					acc, _ := types.Sender(b.signer, tx)
					txs[acc] = append(txs[acc], tx)
				}

				txset := types.NewTransactionsByPriceAndNonce(b.signer, txs, createTxCmp(w.chain, b.header, b.state))
				tcount := b.tcount
				b.commitTransactions(ctx, w, txset, txFeeRecipient)
				// Only update the snapshot if any new transactons were added
				// to the pending block
				if tcount != b.tcount {
					w.updatePendingBlock(b)
				}
			}
		}
	}

}

// mainLoop is a standalone goroutine to create tasks and submit to the engine.
func (w *worker) mainLoop() {
	defer w.chainHeadSub.Unsubscribe()
	defer w.txsSub.Unsubscribe()
	// Context and cancel function for the currently executing block construction
	// Cancel needs to be called in each exit path to make the linter happy
	// because go struggles with analyzing lexical scoping.
	var taskCtx context.Context
	var cancel context.CancelFunc
	var wg sync.WaitGroup

	txsCh := make(chan core.NewTxsEvent, txChanSize)

	generateNewBlock := func() {
		if cancel != nil {
			cancel()
		}
		wg.Wait()
		taskCtx, cancel = context.WithCancel(context.Background())
		wg.Add(1)

		if w.isRunning() {
			// engine.NewWork posts the FinalCommitted Event to IBFT to signal the start of the next round
			if h, ok := w.engine.(consensus.Handler); ok {
				h.NewWork()
			}

			go func() {
				w.constructAndSubmitNewBlock(taskCtx)
				wg.Done()
			}()
		} else {
			go func() {
				w.constructPendingStateBlock(taskCtx, txsCh)
				wg.Done()
			}()
		}
	}

	for {
		select {
		case <-w.startCh:
			generateNewBlock()

		case <-w.chainHeadCh:
			generateNewBlock()

		case ev := <-w.txsCh:
			// Drain tx sub channel as a validator,
			// otherwise pass it to the full node loop
			// if the full node loop's channel is full, just drop the transaction
			if !w.isRunning() {
				select {
				case txsCh <- ev:
				default:
				}
			}
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
		case <-w.txsSub.Err():
			if cancel != nil {
				cancel()
			}
			return
		}
	}
}

func (w *worker) submitTaskToEngine(task *task) {
	if w.newTaskHook != nil {
		w.newTaskHook(task)
	}

	if w.skipSealHook != nil && w.skipSealHook(task) {
		return
	}

	if err := w.engine.Seal(w.chain, task.block); err != nil {
		log.Warn("Block sealing failed", "err", err)
	}
}

// updatePendingBlock updates pending snapshot block and state.
func (w *worker) updatePendingBlock(b *blockState) {
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
