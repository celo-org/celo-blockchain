package test

import (
	"context"
	"errors"
	"fmt"
	"sync"

	ethereum "github.com/celo-org/celo-blockchain"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/event"
)

var (
	errStopped = errors.New("transaction tracker closed")
)

// Tracker tracks processed blocks and transactions through a subscription with
// an ethclient. It provides the ability to check whether blocks or
// transactions have been processed and to wait till those blocks or
// transactions have been processed.
type Tracker struct {
	client *ethclient.Client
	heads  chan *types.Header
	sub    ethereum.Subscription
	wg     sync.WaitGroup
	// processedTxs maps transaction hashes to the block they were processed in.
	processedTxs map[common.Hash]*types.Block
	// processedBlocks maps block number to processed blocks.
	processedBlocks map[uint64]*types.Block
	processedMu     sync.Mutex
	stopCh          chan struct{}
	newBlock        event.Feed
}

// NewTracker creates a new tracker.
func NewTracker() *Tracker {
	return &Tracker{
		heads:           make(chan *types.Header, 10),
		processedTxs:    make(map[common.Hash]*types.Block),
		processedBlocks: make(map[uint64]*types.Block),
	}
}

// GetProcessedTx returns the processed transaction with the given hash or nil
// if the tracker has not seen a processed transaction with the given hash.
func (tr *Tracker) GetProcessedTx(hash common.Hash) *types.Transaction {
	tr.processedMu.Lock()
	defer tr.processedMu.Unlock()
	return tr.processedTxs[hash].Transaction(hash)
}

// GetProcessedBlockForTx returns the block that a transaction with the given
// hash was processed in or nil if the tracker has not seen a processed
// transaction with the given hash.
func (tr *Tracker) GetProcessedBlockForTx(hash common.Hash) *types.Block {
	tr.processedMu.Lock()
	defer tr.processedMu.Unlock()
	return tr.processedTxs[hash]
}

// GetProcessedBlock returns processed block with the given num or nil if the
// tracker has not seen a processed block with that num.
func (tr *Tracker) GetProcessedBlock(num uint64) *types.Block {
	tr.processedMu.Lock()
	defer tr.processedMu.Unlock()
	return tr.processedBlocks[num]
}

// StartTracking subscribes to new head events on the client and starts
// processing the events in a goroutine.
func (tr *Tracker) StartTracking(client *ethclient.Client) error {
	if tr.sub != nil {
		return errors.New("attempted to start already started tracker")
	}
	// The subscription client will buffer 20000 notifications before closing
	// the subscription, if that happens the Err() chan will return
	// ErrSubscriptionQueueOverflow
	sub, err := client.SubscribeNewHead(context.Background(), tr.heads)
	if err != nil {
		return err
	}
	tr.client = client
	tr.sub = sub
	tr.stopCh = make(chan struct{})

	tr.wg.Add(1)
	go func() {
		defer tr.wg.Done()
		err := tr.track()
		if err != nil {
			fmt.Printf("track failed with error: %v\n", err)
		}
	}()
	return nil
}

// track reads new heads from the heads channel and for each head retrieves the
// block, places the block in processedBlocks and places the transactions into
// processedTxs. It signals the sub Subscription for each retrieved block.
func (tr *Tracker) track() error {
	for {
		select {
		case h := <-tr.heads:
			b, err := tr.client.BlockByHash(context.Background(), h.Hash())
			if err != nil {
				return err
			}
			tr.processedMu.Lock()
			tr.processedBlocks[b.NumberU64()] = b
			// If we have transactions then process them
			if len(b.Transactions()) > 0 {
				for _, t := range b.Transactions() {
					tr.processedTxs[t.Hash()] = b
				}
			}
			tr.processedMu.Unlock()
			// signal
			tr.newBlock.Send(struct{}{})
		case err := <-tr.sub.Err():
			// Will be nil if closed by calling Unsubscribe()
			return err
		case <-tr.stopCh:
			return nil
		}
	}
}

// AwaitTransactions waits for the transactions listed in hashes to be
// processed, it will return the ctx.Err() if ctx expires before all the
// transactions in hashes were processed or ErrStopped if StopTracking is
// called before all the transactions in hashes were processed.
func (tr *Tracker) AwaitTransactions(ctx context.Context, hashes []common.Hash) error {
	hashmap := make(map[common.Hash]struct{}, len(hashes))
	for i := range hashes {
		hashmap[hashes[i]] = struct{}{}
	}
	condition := func() bool {
		for hash := range hashmap {
			_, ok := tr.processedTxs[hash]
			if ok {
				delete(hashmap, hash)
			}
		}
		// If there are no transactions left then they have all been processed.
		return len(hashmap) == 0
	}
	return tr.await(ctx, condition)
}

// AwaitBlock waits for a block with the given num to be processed, it will
// return the ctx.Err() if ctx expires before a block with that number has been
// processed or ErrStopped if StopTracking is called before a block with that
// number is processed.
func (tr *Tracker) AwaitBlock(ctx context.Context, num uint64) error {
	condition := func() bool {
		return tr.processedBlocks[num] != nil
	}
	return tr.await(ctx, condition)
}

// await waits for the provided condition to return true, it rechecks the
// condition every time a new block is received by the Tracker. Await returns
// nil when the condition returns true, otherwise it will return ctx.Err() if
// ctx expires before the condition returns true or ErrStopped if StopTracking
// is called before the condition returns true.
func (tr *Tracker) await(ctx context.Context, condition func() bool) error {
	ch := make(chan struct{}, 10)
	sub := tr.newBlock.Subscribe(ch)
	defer sub.Unsubscribe()
	for {
		tr.processedMu.Lock()
		found := condition()
		tr.processedMu.Unlock()
		// If we found what we are looking for then return.
		if found {
			return nil
		}
		select {
		case <-ch:
			continue
		case <-ctx.Done():
			return ctx.Err()
		case <-tr.stopCh:
			return errStopped
		}
	}
}

// StopTracking shuts down all the goroutines in the tracker.
func (tr *Tracker) StopTracking() error {
	if tr.sub == nil {
		return errors.New("attempted to stop already stopped tracker")
	}
	tr.sub.Unsubscribe()
	close(tr.stopCh)
	tr.wg.Wait()
	// Set this to nil to mark the tracker as stopped. This must be done after
	// waiting for wg, to avoid a data race in trackTransactions.
	tr.sub = nil
	tr.wg = sync.WaitGroup{}
	return nil
}
