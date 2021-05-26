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
	"bytes"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/misc"
	"github.com/celo-org/celo-blockchain/contract_comm/currency"
	"github.com/celo-org/celo-blockchain/contract_comm/random"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/event"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
	"github.com/celo-org/celo-blockchain/params"
	mapset "github.com/deckarep/golang-set"
)

const (
	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096

	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10

	// chainSideChanSize is the size of channel listening to ChainSideEvent.
	chainSideChanSize = 10

	// resubmitAdjustChanSize is the size of resubmitting interval adjustment channel.
	resubmitAdjustChanSize = 10

	// miningLogAtDepth is the number of confirmations before logging successful mining.
	miningLogAtDepth = 7

	// minRecommitInterval is the minimal time interval to recreate the mining block with
	// any newly arrived transactions.
	minRecommitInterval = 1 * time.Second

	// maxRecommitInterval is the maximum time interval to recreate the mining block with
	// any newly arrived transactions.
	maxRecommitInterval = 15 * time.Second

	// intervalAdjustRatio is the impact a single interval adjustment has on sealing work
	// resubmitting interval.
	intervalAdjustRatio = 0.1

	// intervalAdjustBias is applied during the new resubmit interval calculation in favor of
	// increasing upper limit or decreasing lower limit so that the limit can be reachable.
	intervalAdjustBias = 200 * 1000.0 * 1000.0
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

// environment is the worker's current environment and holds all of the current state information.
type environment struct {
	signer types.Signer

	state     *state.StateDB // apply state changes here
	ancestors mapset.Set     // ancestor set (used for checking parent validity)
	tcount    int            // tx count in cycle
	gasPool   *core.GasPool  // available gas used to pack transactions
	gasLimit  uint64

	header     *types.Header
	txs        []*types.Transaction
	receipts   []*types.Receipt
	randomness *types.Randomness // The types.Randomness of the last block by mined by this worker.
}

// task contains all information for consensus engine sealing and result submitting.
type task struct {
	receipts  []*types.Receipt
	state     *state.StateDB
	block     *types.Block
	createdAt time.Time
}

const (
	commitInterruptNone int32 = iota
	commitInterruptNewHead
	commitInterruptResubmit
)

// newWorkReq represents a request for new sealing work submitting with relative interrupt notifier.
type newWorkReq struct {
	interrupt *int32
	noempty   bool
	timestamp int64
}

// intervalAdjust represents a resubmitting interval adjustment.
type intervalAdjust struct {
	ratio float64
	inc   bool
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
	chainSideCh  chan core.ChainSideEvent
	chainSideSub event.Subscription

	// Channels
	newWorkCh          chan *newWorkReq
	taskCh             chan *task
	startCh            chan struct{}
	exitCh             chan struct{}
	resubmitIntervalCh chan time.Duration
	resubmitAdjustCh   chan *intervalAdjust

	current     *environment       // An environment for current running cycle.
	unconfirmed *unconfirmedBlocks // A set of locally mined blocks pending canonicalness confirmations.

	mu             sync.RWMutex // The lock used to protect the validator, txFeeRecipient and extra fields
	validator      common.Address
	txFeeRecipient common.Address
	extra          []byte

	snapshotMu    sync.RWMutex // The lock used to protect the block snapshot and state snapshot
	snapshotBlock *types.Block
	snapshotState *state.StateDB

	// atomic status counters
	running int32 // The indicator whether the consensus engine is running or not.
	newTxs  int32 // New arrival transaction count since last sealing work submitting.

	// noempty is the flag used to control whether the feature of pre-seal empty
	// block is enabled. The default value is false(pre-seal is enabled by default).
	// But in some special scenario the consensus engine will seal blocks instantaneously,
	// in this case this feature will add all empty blocks into canonical chain
	// non-stop and no real transaction will be included.
	noempty uint32

	// External functions
	isLocalBlock func(block *types.Block) bool // Function used to determine whether the specified block is mined by local miner.

	// Test hooks
	newTaskHook  func(*task)                        // Method to call upon receiving a new sealing task.
	skipSealHook func(*task) bool                   // Method to decide whether skipping the sealing.
	fullTaskHook func()                             // Method to call before pushing the full sealing task.
	resubmitHook func(time.Duration, time.Duration) // Method to call upon updating resubmitting interval.

	// Needed for randomness
	db ethdb.Database

	blockConstructGauge metrics.Gauge
}

func newWorker(config *Config, chainConfig *params.ChainConfig, engine consensus.Engine, eth Backend, mux *event.TypeMux, isLocalBlock func(*types.Block) bool, db ethdb.Database, init bool) *worker {
	worker := &worker{
		config:              config,
		chainConfig:         chainConfig,
		engine:              engine,
		eth:                 eth,
		mux:                 mux,
		chain:               eth.BlockChain(),
		isLocalBlock:        isLocalBlock,
		unconfirmed:         newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth),
		txsCh:               make(chan core.NewTxsEvent, txChanSize),
		chainHeadCh:         make(chan core.ChainHeadEvent, chainHeadChanSize),
		chainSideCh:         make(chan core.ChainSideEvent, chainSideChanSize),
		newWorkCh:           make(chan *newWorkReq),
		taskCh:              make(chan *task),
		exitCh:              make(chan struct{}),
		startCh:             make(chan struct{}, 1),
		resubmitIntervalCh:  make(chan time.Duration),
		resubmitAdjustCh:    make(chan *intervalAdjust, resubmitAdjustChanSize),
		db:                  db,
		blockConstructGauge: metrics.NewRegisteredGauge("miner/worker/block_construct", nil),
	}
	// Subscribe NewTxsEvent for tx pool
	worker.txsSub = eth.TxPool().SubscribeNewTxsEvent(worker.txsCh)
	// Subscribe events for blockchain
	worker.chainHeadSub = eth.BlockChain().SubscribeChainHeadEvent(worker.chainHeadCh)
	worker.chainSideSub = eth.BlockChain().SubscribeChainSideEvent(worker.chainSideCh)

	// Sanitize recommit interval if the user-specified one is too short.
	recommit := worker.config.Recommit
	if recommit < minRecommitInterval {
		log.Warn("Sanitizing miner recommit interval", "provided", recommit, "updated", minRecommitInterval)
		recommit = minRecommitInterval
	}

	go worker.mainLoop()
	go worker.newWorkLoop(recommit)
	go worker.taskLoop()

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

// setRecommitInterval updates the interval for miner sealing work recommitting.
func (w *worker) setRecommitInterval(interval time.Duration) {
	w.resubmitIntervalCh <- interval
}

// disablePreseal disables pre-sealing mining feature
func (w *worker) disablePreseal() {
	atomic.StoreUint32(&w.noempty, 1)
}

// enablePreseal enables pre-sealing mining feature
func (w *worker) enablePreseal() {
	atomic.StoreUint32(&w.noempty, 0)
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

func (w *worker) createTxCmp() func(tx1 *types.Transaction, tx2 *types.Transaction) int {
	// TODO specify header & state
	currencyManager := currency.NewManager(nil, nil)

	return func(tx1 *types.Transaction, tx2 *types.Transaction) int {
		return currencyManager.CmpValues(tx1.GasPrice(), tx1.FeeCurrency(), tx2.GasPrice(), tx2.FeeCurrency())
	}
}

// newWorkLoop is a standalone goroutine to submit new mining work upon received events.
func (w *worker) newWorkLoop(recommit time.Duration) {
	var (
		interrupt   *int32
		minRecommit = recommit // minimal resubmit interval specified by user.
		timestamp   int64      // timestamp for each round of mining.
	)

	timer := time.NewTimer(0)
	defer timer.Stop()
	<-timer.C // discard the initial tick

	// commit aborts in-flight transaction execution with given signal and resubmits a new one.
	commit := func(noempty bool, s int32) {
		if interrupt != nil {
			atomic.StoreInt32(interrupt, s)
		}
		interrupt = new(int32)
		w.newWorkCh <- &newWorkReq{interrupt: interrupt, noempty: noempty, timestamp: timestamp}
		timer.Reset(recommit)
		atomic.StoreInt32(&w.newTxs, 0)
	}
	// recalcRecommit recalculates the resubmitting interval upon feedback.
	recalcRecommit := func(target float64, inc bool) {
		var (
			prev = float64(recommit.Nanoseconds())
			next float64
		)
		if inc {
			next = prev*(1-intervalAdjustRatio) + intervalAdjustRatio*(target+intervalAdjustBias)
			// Recap if interval is larger than the maximum time interval
			if next > float64(maxRecommitInterval.Nanoseconds()) {
				next = float64(maxRecommitInterval.Nanoseconds())
			}
		} else {
			next = prev*(1-intervalAdjustRatio) + intervalAdjustRatio*(target-intervalAdjustBias)
			// Recap if interval is less than the user specified minimum
			if next < float64(minRecommit.Nanoseconds()) {
				next = float64(minRecommit.Nanoseconds())
			}
		}
		recommit = time.Duration(int64(next))
	}

	for {
		select {
		case <-w.startCh:
			timestamp = time.Now().Unix()
			commit(false, commitInterruptNewHead)

		case <-w.chainHeadCh:
			timestamp = time.Now().Unix()
			commit(false, commitInterruptNewHead)

		case <-timer.C:
			// If mining is running resubmit a new work cycle periodically to pull in
			// higher priced transactions. Disable this overhead for pending blocks.
			if w.isRunning() && !w.isIstanbulEngine() {
				// Short circuit if no new transaction arrives.
				if atomic.LoadInt32(&w.newTxs) == 0 {
					timer.Reset(recommit)
					continue
				}
				commit(true, commitInterruptResubmit)
			}

		case interval := <-w.resubmitIntervalCh:
			// Adjust resubmit interval explicitly by user.
			if interval < minRecommitInterval {
				log.Warn("Sanitizing miner recommit interval", "provided", interval, "updated", minRecommitInterval)
				interval = minRecommitInterval
			}
			log.Info("Miner recommit interval update", "from", minRecommit, "to", interval)
			minRecommit, recommit = interval, interval

			if w.resubmitHook != nil {
				w.resubmitHook(minRecommit, recommit)
			}

		case adjust := <-w.resubmitAdjustCh:
			// Adjust resubmit interval by feedback.
			if adjust.inc {
				before := recommit
				recalcRecommit(float64(recommit.Nanoseconds())/adjust.ratio, true)
				log.Trace("Increase miner recommit interval", "from", before, "to", recommit)
			} else {
				before := recommit
				recalcRecommit(float64(minRecommit.Nanoseconds()), false)
				log.Trace("Decrease miner recommit interval", "from", before, "to", recommit)
			}

			if w.resubmitHook != nil {
				w.resubmitHook(minRecommit, recommit)
			}

		case <-w.exitCh:
			return
		}
	}
}

// mainLoop is a standalone goroutine to regenerate the sealing task based on the received event.
func (w *worker) mainLoop() {
	defer w.txsSub.Unsubscribe()
	defer w.chainHeadSub.Unsubscribe()
	defer w.chainSideSub.Unsubscribe()

	for {
		select {
		case req := <-w.newWorkCh:
			if h, ok := w.engine.(consensus.Handler); ok {
				h.NewWork()
			}
			w.commitNewWork(req.interrupt, req.noempty, req.timestamp)

		case ev := <-w.chainSideCh:
			// TOOO(nategraf): Remove this subcription, as there is no work to be done here.
			log.Debug("Message in chan chainSideCh", "hash", ev.Block.Hash(), "number", ev.Block.Number(), "root", ev.Block.Root())

		case ev := <-w.txsCh:
			// Apply transactions to the pending state if we're not mining.
			//
			// Note all transactions received may not be continuous with transactions
			// already included in the current mining block. These transactions will
			// be automatically eliminated.
			if !w.isRunning() && w.current != nil {
				// If block is already full, abort
				if gp := w.current.gasPool; gp != nil && gp.Gas() < params.TxGas {
					continue
				}
				w.mu.RLock()
				txFeeRecipient := w.txFeeRecipient
				if !w.chainConfig.IsDonut(w.current.header.Number) && w.txFeeRecipient != w.validator {
					txFeeRecipient = w.validator
					log.Warn("TxFeeRecipient and Validator flags set before split etherbase fork is active. Defaulting to the given validator address for the coinbase.")
				}
				w.mu.RUnlock()

				txs := make(map[common.Address]types.Transactions)
				for _, tx := range ev.Txs {
					acc, _ := types.Sender(w.current.signer, tx)
					txs[acc] = append(txs[acc], tx)
				}

				txset := types.NewTransactionsByPriceAndNonce(w.current.signer, txs, w.createTxCmp())
				tcount := w.current.tcount
				w.commitTransactions(txset, txFeeRecipient, nil)
				// Only update the snapshot if any new transactons were added
				// to the pending block
				if tcount != w.current.tcount {
					w.updateSnapshot()
				}
			}
			atomic.AddInt32(&w.newTxs, int32(len(ev.Txs)))

		// System stopped
		case <-w.exitCh:
			return
		case <-w.txsSub.Err():
			return
		case <-w.chainHeadSub.Err():
			return
		case <-w.chainSideSub.Err():
			return
		}
	}
}

// taskLoop is a standalone goroutine to fetch sealing task from the generator and
// push them to consensus engine.
func (w *worker) taskLoop() {

	for {
		select {
		case task := <-w.taskCh:
			if w.newTaskHook != nil {
				w.newTaskHook(task)
			}

			if w.skipSealHook != nil && w.skipSealHook(task) {
				continue
			}

			if err := w.engine.Seal(w.chain, task.block); err != nil {
				log.Warn("Block sealing failed", "err", err)
			}
		case <-w.exitCh:
			return
		}
	}
}

// makeCurrent creates a new environment for the current cycle.
func (w *worker) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := w.chain.StateAt(parent.Root())
	if err != nil {
		return err
	}

	env := &environment{
		signer:    types.NewEIP155Signer(w.chainConfig.ChainID),
		state:     state,
		ancestors: mapset.NewSet(),
		header:    header,
		gasLimit:  core.CalcGasLimit(parent, state),
	}

	// when 08 is processed ancestors contain 07 (quick block)
	for _, ancestor := range w.chain.GetBlocksFromHash(parent.Hash(), 7) {
		env.ancestors.Add(ancestor.Hash())
	}

	// Keep track of transactions which return errors so they can be removed
	env.tcount = 0
	w.current = env
	return nil
}

// updateSnapshot updates pending snapshot block and state.
// Note this function assumes the current variable is thread safe.
func (w *worker) updateSnapshot() {
	w.snapshotMu.Lock()
	defer w.snapshotMu.Unlock()

	w.snapshotBlock = types.NewBlock(
		w.current.header,
		w.current.txs,
		w.current.receipts,
		w.current.randomness,
	)

	w.snapshotState = w.current.state.Copy()
}

func (w *worker) commitTransaction(tx *types.Transaction, txFeeRecipient common.Address) ([]*types.Log, error) {
	snap := w.current.state.Snapshot()

	receipt, err := core.ApplyTransaction(w.chainConfig, w.chain, &txFeeRecipient, w.current.gasPool, w.current.state, w.current.header, tx, &w.current.header.GasUsed, *w.chain.GetVMConfig())
	if err != nil {
		w.current.state.RevertToSnapshot(snap)
		return nil, err
	}
	w.current.txs = append(w.current.txs, tx)
	w.current.receipts = append(w.current.receipts, receipt)

	return receipt.Logs, nil
}

func (w *worker) commitTransactions(txs *types.TransactionsByPriceAndNonce, txFeeRecipient common.Address, interrupt *int32) bool {
	// Short circuit if current is nil
	if w.current == nil {
		return true
	}

	if w.current.gasPool == nil {
		w.current.gasPool = new(core.GasPool).AddGas(w.current.gasLimit)
	}

	var coalescedLogs []*types.Log

loop:
	for {
		// In the following three cases, we will interrupt the execution of the transaction.
		// (1) new head block event arrival, the interrupt signal is 1
		// (2) worker start or restart, the interrupt signal is 1
		// (3) worker recreate the mining block with any newly arrived transactions, the interrupt signal is 2.
		// For the first two cases, the semi-finished work will be discarded.
		// For the third case, the semi-finished work will be submitted to the consensus engine.
		if interrupt != nil && atomic.LoadInt32(interrupt) != commitInterruptNone {
			// Notify resubmit loop to increase resubmitting interval due to too frequent commits.
			if atomic.LoadInt32(interrupt) == commitInterruptResubmit {
				ratio := float64(w.current.gasLimit-w.current.gasPool.Gas()) / float64(w.current.gasLimit)
				if ratio < 0.1 {
					ratio = 0.1
				}
				w.resubmitAdjustCh <- &intervalAdjust{
					ratio: ratio,
					inc:   true,
				}
			}
			return atomic.LoadInt32(interrupt) == commitInterruptNewHead
		}
		// If we don't have enough gas for any further transactions then we're done
		if w.current.gasPool.Gas() < params.TxGas {
			log.Trace("Not enough gas for further transactions", "have", w.current.gasPool, "want", params.TxGas)
			break
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		// Short-circuit if the transaction requires more gas than we have in the pool.
		// If we didn't short-circuit here, we would get core.ErrGasLimitReached below.
		// Short-circuiting here saves us the trouble of checking the GPM and so on when the tx can't be included
		// anyway due to the block not having enough gas left.
		if w.current.gasPool.Gas() < tx.Gas() {
			log.Trace("Skipping transaction which requires more gas than is left in the block", "hash", tx.Hash(), "gas", w.current.gasPool.Gas(), "txgas", tx.Gas())
			txs.Pop()
			continue
		}
		// Error may be ignored here. The error has already been checked
		// during transaction acceptance is the transaction pool.
		//
		// We use the eip155 signer regardless of the current hf.
		from, _ := types.Sender(w.current.signer, tx)
		// Check whether the tx is replay protected. If we're not in the EIP155 hf
		// phase, start ignoring the sender until we do.
		if tx.Protected() && !w.chainConfig.IsEIP155(w.current.header.Number) {
			log.Trace("Ignoring reply protected transaction", "hash", tx.Hash(), "eip155", w.chainConfig.EIP155Block)

			txs.Pop()
			continue
		}
		// Start executing the transaction
		w.current.state.Prepare(tx.Hash(), common.Hash{}, w.current.tcount)

		logs, err := w.commitTransaction(tx, txFeeRecipient)
		switch err {
		case core.ErrGasLimitReached:
			// Pop the current out-of-gas transaction without shifting in the next from the account
			log.Trace("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case core.ErrNonceTooLow:
			// New head notification data race between the transaction pool and miner, shift
			log.Trace("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case core.ErrNonceTooHigh:
			// Reorg notification data race between the transaction pool and miner, skip account =
			log.Trace("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case core.ErrGasPriceDoesNotExceedMinimum:
			// We are below the GPM, so we can stop (the rest of the transactions will either have
			// even lower gas price or won't be mineable yet due to their nonce)
			log.Trace("Skipping remaining transaction below the gas price minimum")
			break loop

		case nil:
			// Everything ok, collect the logs and shift in the next transaction from the same account
			coalescedLogs = append(coalescedLogs, logs...)
			w.current.tcount++
			txs.Shift()

		default:
			// Strange error, discard the transaction and get the next in line (note, the
			// nonce-too-high clause will prevent us from executing in vain).
			log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

	if !w.isRunning() && len(coalescedLogs) > 0 {
		// We don't push the pendingLogsEvent while we are mining. The reason is that
		// when we are mining, the worker will regenerate a mining block every 3 seconds.
		// In order to avoid pushing the repeated pendingLog, we disable the pending log pushing.

		// make a copy, the state caches the logs and these logs get "upgraded" from pending to mined
		// logs by filling in the block hash when the block was mined by the local miner. This can
		// cause a race condition if a log was "upgraded" before the PendingLogsEvent is processed.
		cpy := make([]*types.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		w.pendingLogsFeed.Send(cpy)
	}
	// Notify resubmit loop to decrease resubmitting interval if current interval is larger
	// than the user-specified one.
	if interrupt != nil {
		w.resubmitAdjustCh <- &intervalAdjust{inc: false}
	}
	return false
}

// commitNewWork generates several new sealing tasks based on the parent block.
func (w *worker) commitNewWork(interrupt *int32, noempty bool, timestamp int64) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	tstart := time.Now()
	parent := w.chain.CurrentBlock()

	if parent.Time() >= uint64(timestamp) {
		timestamp = int64(parent.Time() + 1)
	}
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); timestamp > now+1 {
		wait := time.Duration(timestamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	num := parent.Number()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		Extra:      w.extra,
		Time:       uint64(timestamp),
	}

	txFeeRecipient := w.txFeeRecipient
	if !w.chainConfig.IsDonut(header.Number) && w.txFeeRecipient != w.validator {
		txFeeRecipient = w.validator
		log.Warn("TxFeeRecipient and Validator flags set before split etherbase fork is active. Defaulting to the given validator address for the coinbase.")
	}

	// Only set the coinbase if our consensus engine is running (avoid spurious block rewards)
	if w.isRunning() {
		if txFeeRecipient == (common.Address{}) {
			log.Error("Refusing to mine without etherbase")
			return
		}
		header.Coinbase = txFeeRecipient
	}
	if err := w.engine.Prepare(w.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return
	}
	// Start record block construction time after `engine.Prepare` to exclude the sleep time
	defer func(start time.Time) { w.blockConstructGauge.Update(time.Since(start).Nanoseconds()) }(time.Now())

	// If we are care about TheDAO hard-fork check whether to override the extra-data or not
	if daoBlock := w.chainConfig.DAOForkBlock; daoBlock != nil {
		// Check whether the block is among the fork extra-override range
		limit := new(big.Int).Add(daoBlock, params.DAOForkExtraRange)
		if header.Number.Cmp(daoBlock) >= 0 && header.Number.Cmp(limit) < 0 {
			// Depending whether we support or oppose the fork, override differently
			if w.chainConfig.DAOForkSupport {
				header.Extra = common.CopyBytes(params.DAOForkBlockExtra)
			} else if bytes.Equal(header.Extra, params.DAOForkBlockExtra) {
				header.Extra = []byte{} // If miner opposes, don't let it use the reserved extra-data
			}
		}
	}
	// Could potentially happen if starting to mine in an odd state.
	err := w.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
	// Create the current work task and check any fork transitions needed
	env := w.current
	if w.chainConfig.DAOForkSupport && w.chainConfig.DAOForkBlock != nil && w.chainConfig.DAOForkBlock.Cmp(header.Number) == 0 {
		misc.ApplyDAOHardFork(env.state)
	}

	if !noempty && !w.isIstanbulEngine() && atomic.LoadUint32(&w.noempty) == 0 {
		// Create an empty block based on temporary copied state for sealing in advance without waiting block
		// execution finished.
		w.commit(nil, false, tstart)
	}

	istanbulEmptyBlockCommit := func() {
		if !noempty && w.isIstanbulEngine() {
			w.commit(nil, false, tstart)
		}
	}

	w.updateSnapshot()

	// Play our part in generating the random beacon.
	if w.isRunning() && random.IsRunning() {
		istanbul, ok := w.engine.(consensus.Istanbul)
		if !ok {
			log.Crit("Istanbul consensus engine must be in use for the randomness beacon")
		}

		lastCommitment, err := random.GetLastCommitment(w.validator, w.current.header, w.current.state)
		if err != nil {
			log.Error("Failed to get last commitment", "err", err)
			return
		}

		lastRandomness := common.Hash{}
		if (lastCommitment != common.Hash{}) {
			lastRandomnessParentHash := rawdb.ReadRandomCommitmentCache(w.db, lastCommitment)
			if (lastRandomnessParentHash == common.Hash{}) {
				log.Error("Failed to get last randomness cache entry")
				return
			}

			var err error
			lastRandomness, _, err = istanbul.GenerateRandomness(lastRandomnessParentHash)
			if err != nil {
				log.Error("Failed to generate last randomness", "err", err)
				return
			}
		}

		_, newCommitment, err := istanbul.GenerateRandomness(w.current.header.ParentHash)
		if err != nil {
			log.Error("Failed to generate new randomness", "err", err)
			return
		}

		err = random.RevealAndCommit(lastRandomness, newCommitment, w.validator, w.current.header, w.current.state)
		if err != nil {
			log.Error("Failed to reveal and commit randomness", "randomness", lastRandomness.Hex(), "commitment", newCommitment.Hex(), "err", err)
			return
		}
		// always true (EIP158)
		w.current.state.IntermediateRoot(true)

		w.current.randomness = &types.Randomness{Revealed: lastRandomness, Committed: newCommitment}
	} else {
		w.current.randomness = &types.EmptyRandomness
	}

	// Fill the block with all available pending transactions.
	pending, err := w.eth.TxPool().Pending()

	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		istanbulEmptyBlockCommit()
		return
	}

	// Short circuit if there is no available pending transactions.
	if len(pending) == 0 {
		istanbulEmptyBlockCommit()
		return
	}
	// Split the pending transactions into locals and remotes
	localTxs, remoteTxs := make(map[common.Address]types.Transactions), pending
	for _, account := range w.eth.TxPool().Locals() {
		if txs := remoteTxs[account]; len(txs) > 0 {
			delete(remoteTxs, account)
			localTxs[account] = txs
		}
	}

	txComparator := w.createTxCmp()
	if len(localTxs) > 0 {
		txs := types.NewTransactionsByPriceAndNonce(w.current.signer, localTxs, txComparator)
		if w.commitTransactions(txs, txFeeRecipient, interrupt) {
			return
		}
	}
	if len(remoteTxs) > 0 {
		txs := types.NewTransactionsByPriceAndNonce(w.current.signer, remoteTxs, txComparator)
		if w.commitTransactions(txs, txFeeRecipient, interrupt) {
			return
		}
	}
	w.commit(w.fullTaskHook, true, tstart)
}

// commit runs any post-transaction state modifications, assembles the final block
// and commits new work if consensus engine is running.
func (w *worker) commit(interval func(), update bool, start time.Time) error {
	// Deep copy receipts here to avoid interaction between different tasks.
	receipts := make([]*types.Receipt, len(w.current.receipts))
	for i, l := range w.current.receipts {
		receipts[i] = new(types.Receipt)
		*receipts[i] = *l
	}
	s := w.current.state.Copy()

	block, err := w.engine.FinalizeAndAssemble(w.chain, w.current.header, s, w.current.txs, w.current.receipts, w.current.randomness)

	// Set the validator set diff in the new header if we're using Istanbul and it's the last block of the epoch
	if istanbul, ok := w.engine.(consensus.Istanbul); ok {
		if err := istanbul.UpdateValSetDiff(w.chain, block.MutableHeader(), s); err != nil {
			log.Error("Unable to update Validator Set Diff", "err", err)
			return err
		}
	}

	if len(s.GetLogs(common.Hash{})) > 0 {
		receipt := types.NewReceipt(nil, false, 0)
		receipt.Logs = s.GetLogs(common.Hash{})
		for i := range receipt.Logs {
			receipt.Logs[i].TxIndex = uint(len(receipts))
		}
		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		receipts = append(receipts, receipt)
	}

	if err != nil {
		log.Error("Unable to finalize block", "err", err)
		return err
	}
	if w.isRunning() {
		if interval != nil {
			interval()
		}
		select {
		case w.taskCh <- &task{receipts: receipts, state: s, block: block, createdAt: time.Now()}:
			w.unconfirmed.Shift(block.NumberU64() - 1)

			feesWei := new(big.Int)
			for i, tx := range block.Transactions() {
				feesWei.Add(feesWei, new(big.Int).Mul(new(big.Int).SetUint64(receipts[i].GasUsed), tx.GasPrice()))
			}
			feesEth := new(big.Float).Quo(new(big.Float).SetInt(feesWei), new(big.Float).SetInt(big.NewInt(params.Ether)))

			log.Info("Commit new mining work", "number", block.Number(), "txs", w.current.tcount, "gas", block.GasUsed(),
				"fees", feesEth, "elapsed", common.PrettyDuration(time.Since(start)))
		case <-w.exitCh:
			log.Info("Worker has exited")
		}
	}
	if update {
		w.updateSnapshot()
	}
	return nil
}

func (w *worker) isIstanbulEngine() bool {
	// TODO find a better way to do this
	_, isIstanbul := w.engine.(consensus.Istanbul)
	return isIstanbul
}
