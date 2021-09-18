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

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/contracts/random"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/event"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/davecgh/go-spew/spew"
)

// Backend wraps all methods required for mining.
type Backend interface {
	BlockChain() *core.BlockChain
	TxPool() *core.TxPool
}

// Config is the configuration parameters of mining.
type Config struct {
	Validator common.Address `toml:",omitempty"` // Public address for block signing and randomness (default = first account)
	ExtraData hexutil.Bytes  `toml:",omitempty"` // Block extra data set by the miner
}

// Miner creates blocks and searches for proof-of-work values.
type Miner struct {
	mux       *event.TypeMux
	worker    *worker
	validator common.Address
	eth       Backend
	engine    consensus.Engine
	db        ethdb.Database // Needed for randomness

	exitCh  chan struct{}
	startCh chan struct{}
	stopCh  chan struct{}
}

func New(eth Backend, config *Config, chainConfig *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine, db ethdb.Database) *Miner {
	miner := &Miner{
		eth:     eth,
		mux:     mux,
		engine:  engine,
		exitCh:  make(chan struct{}),
		startCh: make(chan struct{}),
		stopCh:  make(chan struct{}),
		worker:  newWorker(config, chainConfig, engine, eth, mux, db),
		db:      db,
	}
	go miner.update()

	return miner
}

func (miner *Miner) recoverRandomness() {
	// If this is using the istanbul consensus engine, then we need to check
	// for the randomness cache for the randomness beacon protocol
	_, isIstanbul := miner.engine.(consensus.Istanbul)
	if isIstanbul {
		// getCurrentBlockAndState
		currentBlock := miner.eth.BlockChain().CurrentBlock()
		currentHeader := currentBlock.Header()
		currentState, err := miner.eth.BlockChain().StateAt(currentBlock.Root())
		if err != nil {
			log.Error("Error in retrieving state", "block hash", currentHeader.Hash(), "error", err)
			return
		}

		if currentHeader.Number.Uint64() > 0 {
			vmRunner := miner.eth.BlockChain().NewEVMRunner(currentHeader, currentState)
			// Check to see if we already have the commitment cache
			lastCommitment, err := random.GetLastCommitment(vmRunner, miner.validator)
			if err != nil {
				log.Error("Error in retrieving last commitment", "error", err)
				return
			}

			// If there is a non empty last commitment and if we don't have that commitment's
			// cache entry, then we need to recover it.
			if (lastCommitment != common.Hash{}) && (rawdb.ReadRandomCommitmentCache(miner.db, lastCommitment) == common.Hash{}) {
				err := miner.eth.BlockChain().RecoverRandomnessCache(lastCommitment, currentBlock.Hash())
				if err != nil {
					log.Error("Error in recovering randomness cache", "error", err)
					return
				}
			}
		}
	}

}

// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (miner *Miner) update() {
	events := miner.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
	defer func() {
		if !events.Closed() {
			events.Unsubscribe()
		}
	}()

	shouldStart := false
	canStart := true
	dlEventCh := events.Chan()
	for {
		select {
		case ev := <-dlEventCh:
			spew.Dump(ev)
			if ev == nil {
				// Unsubscription done, stop listening
				dlEventCh = nil
				continue
			}
			switch ev.Data.(type) {
			case downloader.StartEvent:
				wasMining := miner.Mining()
				miner.worker.stop()
				canStart = false
				if wasMining {
					// Resume mining after sync was finished
					shouldStart = true
					log.Info("Mining aborted due to sync")
				}
			case downloader.FailedEvent:
				canStart = true
				if shouldStart {
					miner.worker.start()
				}
			case downloader.DoneEvent:
				miner.recoverRandomness()
				canStart = true
				if shouldStart {
					miner.worker.start()
				}
				// Stop reacting to downloader events
				events.Unsubscribe()
			}
		case <-miner.startCh:
			if canStart {
				miner.worker.start()
			}
			shouldStart = true
		case <-miner.stopCh:
			shouldStart = false
			miner.worker.stop()
		case <-miner.exitCh:
			miner.worker.close()
			return
		}
	}
}

func (miner *Miner) Start(validator common.Address, txFeeRecipient common.Address) {
	miner.SetValidator(validator)
	miner.SetTxFeeRecipient(txFeeRecipient)
	miner.startCh <- struct{}{}
}

func (miner *Miner) Stop() {
	miner.stopCh <- struct{}{}
}

func (miner *Miner) Close() {
	close(miner.exitCh)
}

func (miner *Miner) Mining() bool {
	return miner.worker.isRunning()
}

func (miner *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	miner.worker.setExtra(extra)
	return nil
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

// SetValidator sets the miner and worker's address for message and block signing
func (miner *Miner) SetValidator(addr common.Address) {
	miner.validator = addr
	miner.worker.setValidator(addr)
}

// SetTxFeeRecipient sets the address where the miner and worker will receive fees
func (miner *Miner) SetTxFeeRecipient(addr common.Address) {
	miner.worker.setTxFeeRecipient(addr)
}

// SubscribePendingLogs starts delivering logs from pending transactions
// to the given channel.
func (miner *Miner) SubscribePendingLogs(ch chan<- []*types.Log) event.Subscription {
	return miner.worker.pendingLogsFeed.Subscribe(ch)
}
