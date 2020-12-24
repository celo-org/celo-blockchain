// Copyright 2014 The go-ethereum Authors
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

// Package miner implements Ethereum block creation and mining.
package miner

import (
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/contract_comm/random"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// Backend wraps all methods required for mining.
type Backend interface {
	AccountManager() *accounts.Manager
	BlockChain() *core.BlockChain
	TxPool() *core.TxPool
}

// Config is the configuration parameters of mining.
type Config struct {
	Etherbase           common.Address `toml:",omitempty"` // Public address for block mining rewards (default = first account)
	Notify              []string       `toml:",omitempty"` // HTTP URL list to be notified of new work packages(only useful in ethash).
	ExtraData           hexutil.Bytes  `toml:",omitempty"` // Block extra data set by the miner
	GasFloor            uint64         // Target gas floor for mined blocks.
	GasCeil             uint64         // Target gas ceiling for mined blocks.
	GasPrice            *big.Int       // Minimum gas price for mining a transaction
	Recommit            time.Duration  // The time interval for miner to re-create mining work.
	Noverify            bool           // Disable remote mining solution verification(only useful in ethash).
	VerificationService string         // Celo verification service URL
}

// Miner creates blocks and searches for proof-of-work values.
type Miner struct {
	mux      *event.TypeMux
	worker   *worker
	coinbase common.Address
	eth      Backend
	engine   consensus.Engine
	exitCh   chan struct{}
	db       ethdb.Database // Needed for randomness

	canStart    int32 // can start indicates whether we can start the mining operation
	shouldStart int32 // should start indicates whether we should start after sync
}

func New(eth Backend, config *Config, chainConfig *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine, isLocalBlock func(block *types.Block) bool, db ethdb.Database) *Miner {
	miner := &Miner{
		eth:      eth,
		mux:      mux,
		engine:   engine,
		exitCh:   make(chan struct{}),
		worker:   newWorker(config, chainConfig, engine, eth, mux, isLocalBlock, db, true),
		db:       db,
		canStart: 1,
	}
	go miner.update()

	return miner
}

// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (miner *Miner) update() {
	events := miner.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
	defer events.Unsubscribe()

	for {
		select {
		case ev := <-events.Chan():
			if ev == nil {
				return
			}
			switch ev.Data.(type) {
			case downloader.StartEvent:
				atomic.StoreInt32(&miner.canStart, 0)
				if miner.Mining() {
					miner.Stop()
					atomic.StoreInt32(&miner.shouldStart, 1)
					log.Info("Mining aborted due to sync")
				}
			case downloader.DoneEvent, downloader.FailedEvent:
				shouldStart := atomic.LoadInt32(&miner.shouldStart) == 1

				atomic.StoreInt32(&miner.canStart, 1)
				atomic.StoreInt32(&miner.shouldStart, 0)
				if shouldStart {
					istEngine, isIstanbul := miner.engine.(consensus.Istanbul)

					// Check to see if the the latest commitment is in the cache, if it's using the istanbul consensus engine
					if !isIstanbul || miner.commitmentCacheSaved(istEngine) {
						miner.Start(miner.coinbase)
					} else {
						log.Error("Couldn't recover the validator's committed randomness.  Validator will NOT be able to propose blocks, and mining will NOT start")
					}
				}
				// stop immediately and ignore all further pending events
				return
			}
		case <-miner.exitCh:
			return
		}
	}
}

func (miner *Miner) Start(coinbase common.Address) {
	atomic.StoreInt32(&miner.shouldStart, 1)
	miner.SetEtherbase(coinbase)

	if atomic.LoadInt32(&miner.canStart) == 0 {
		log.Info("Network syncing, will start miner afterwards")
		return
	}
	miner.worker.start()
}

func (miner *Miner) Stop() {
	miner.worker.stop()
	atomic.StoreInt32(&miner.shouldStart, 0)
}

func (miner *Miner) Close() {
	miner.worker.close()
	close(miner.exitCh)
}

func (miner *Miner) Mining() bool {
	return miner.worker.isRunning()
}

func (miner *Miner) HashRate() uint64 {
	if pow, ok := miner.engine.(consensus.PoW); ok {
		return uint64(pow.Hashrate())
	}
	return 0
}

func (miner *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	miner.worker.setExtra(extra)
	return nil
}

// SetRecommitInterval sets the interval for sealing work resubmitting.
func (miner *Miner) SetRecommitInterval(interval time.Duration) {
	miner.worker.setRecommitInterval(interval)
}

// Pending returns the currently pending block and associated state.
func (miner *Miner) Pending() (*types.Block, *state.StateDB) {
	return miner.worker.pending()
}

// PendingBlock returns the currently pending block.
//
// Note, to access both the pending block and the pending state
// simultaneously, please use Pending(), as the pending state can
// change between multiple method calls
func (miner *Miner) PendingBlock() *types.Block {
	return miner.worker.pendingBlock()
}

func (miner *Miner) SetEtherbase(addr common.Address) {
	miner.coinbase = addr
	miner.worker.setEtherbase(addr)
}

// SubscribePendingLogs starts delivering logs from pending transactions
// to the given channel.
func (self *Miner) SubscribePendingLogs(ch chan<- []*types.Log) event.Subscription {
	return self.worker.pendingLogsFeed.Subscribe(ch)
}

// commitmentCacheSaved will check to see if the last commitment cache entry is saved.
// If not, it will search for it and save it.
func (miner *Miner) commitmentCacheSaved(istEngine consensus.Istanbul) bool {
	// Subscribe to new block notifications
	newBlockCh := make(chan core.ChainEvent)
	bc := miner.eth.BlockChain()
	newBlockSub := bc.SubscribeChainEvent(newBlockCh)
	defer newBlockSub.Unsubscribe()

	// getCurrentBlockAndState
	currentBlock := bc.CurrentBlock()
	currentHeader := currentBlock.Header()
	currentState, err := bc.StateAt(currentBlock.Root())
	if err != nil {
		log.Error("Error in retrieving state", "block hash", currentHeader.Hash(), "error", err)
		return false
	}

	// Check to see if we already have the commitment cache
	lastCommitment, err := random.GetLastCommitment(miner.coinbase, currentHeader, currentState)
	if err != nil {
		log.Error("Error in retrieving last commitment", "error", err)
		return false
	}

	// If there was no previous commitment from this validator, then return true.
	if (lastCommitment == common.Hash{}) {
		return true
	}

	if (rawdb.ReadRandomCommitmentCache(miner.db, lastCommitment) != common.Hash{}) {
		return true
	}

	// goroutine communication channels and waitgroup
	stopCh := make(chan struct{})
	errCh := make(chan error)
	foundParentHashCh := make(chan common.Hash)
	var wg sync.WaitGroup

	// Stacked defers are exececuted in LIFO order
	defer wg.Wait()
	defer close(stopCh)

	if currentHeader.Number.Uint64() > 0 {
		blockHashIter := currentHeader.Hash()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				blockHeader := bc.GetHeaderByHash(blockHashIter)

				// We got to the genisis block, so this goroutine didn't find the latest
				// block authored by this validator.
				if blockHeader.Number.Uint64() == 0 {
					foundParentHashCh <- common.Hash{}
					return
				}

				blockAuthor, err := miner.engine.Author(blockHeader)
				if err != nil {
					log.Error("Error is retrieving block author", "block number", blockHeader.Number.Uint64(), "block hash", blockHeader.Hash(), "error", err)
					errCh <- err
					return
				}

				if blockAuthor == miner.coinbase {
					foundParentHashCh <- blockHeader.ParentHash
					return
				}

				// Check to see if this goroutine should stop
				select {
				case <-stopCh:
					return
				default:
				}

				blockHashIter = blockHeader.ParentHash
			}
		}()
	}

	parentHash := common.Hash{}

waitLoop:
	for {
		select {

		// The first block that this validator's address authored may be in a block w/ number
		// that is greater than the head of all this node's peer when the downloader.DoneEvent
		// event is fired.  This select case will handle the case when this validator
		// eventually syncs that block.
		case newBlock := <-newBlockCh:
			blockAuthor, err := miner.engine.Author(newBlock.Block.Header())
			if err != nil {
				log.Error("Error is retrieving block author", "block number", newBlock.Block.Number().Uint64(), "block hash", newBlock.Block.Hash(), "error", err)
				return false
			}

			// The blockchain object should of saved the commitment cache when
			// we received a newBlock event.  This code here is assuming that,
			// and not double checking that the cache entry is there.
			if blockAuthor == miner.coinbase {
				parentHash = newBlock.Block.ParentHash()
				break waitLoop
			}

		case parentHash = <-foundParentHashCh:
			break waitLoop

		case <-errCh:
			return false
		}
	}

	if (parentHash != common.Hash{}) {
		// Calculate the randomness commitment
		// The calculation is stateless (e.g. it's just a hash operation of a string), so any passed in block header and state
		// will do. Will use the previously fetched current header and state.
		_, randomCommitment, err := istEngine.GenerateRandomness(parentHash, currentHeader, currentState)
		if err != nil {
			log.Error("Couldn't generate the randomness from the parent hash", "parent hash", parentHash, "err", err)
			return false
		}

		rawdb.WriteRandomCommitmentCache(miner.db, randomCommitment, parentHash)

		return true
	} else {
		return false
	}
}
