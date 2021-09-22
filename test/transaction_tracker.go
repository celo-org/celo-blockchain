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

// TransactionTracker tracks processed transactions through a subscription with
// an ethclient. It provides the ability to check whether transactions have
// been processed and to wait till those transactions have been processed.
type TransactionTracker struct {
	client *ethclient.Client
	heads  chan *types.Header
	sub    ethereum.Subscription
	wg     sync.WaitGroup
	// processed maps transaction hashes to the block they were processed in.
	processed   map[common.Hash]*types.Block
	processedMu sync.Mutex
	stopCh      chan struct{}
	newTxs      event.Feed
}

// NewTransactionTracker creates a new transaction tracker, it subscribes to
// new head events on the client and starts processing the events in a
// goroutine.
func NewTransactionTracker() *TransactionTracker {
	return &TransactionTracker{
		heads:     make(chan *types.Header, 10),
		processed: make(map[common.Hash]*types.Block),
	}
}

// GetProcessedTx returns the processed transaction with the given hash or nil
// if the tracker has not seen a processed transaction with the given hash.
func (tr *TransactionTracker) GetProcessedTx(hash common.Hash) *types.Transaction {
	tr.processedMu.Lock()
	defer tr.processedMu.Unlock()
	return tr.processed[hash].Transaction(hash)
}

// GetProcessedTx returns the block that a transaction with the given hash was
// processed in or nil if the tracker has not seen a processed transaction with
// the given hash.
func (tr *TransactionTracker) GetProcessedBlock(hash common.Hash) *types.Block {
	tr.processedMu.Lock()
	defer tr.processedMu.Unlock()
	return tr.processed[hash]
}

func (tr *TransactionTracker) StartTracking(client *ethclient.Client) error {
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
		err := tr.trackTransactions()
		if err != nil {
			fmt.Printf("trackTransactions failed with error: %v\n", err)
		}
	}()
	return nil
}

// trackTransactions reads new heads from the heads channel and for each head
// retrieves the block and places the transactions into the processed map. It
// signals the newTxs condition for every block that contained transactions.
func (tr *TransactionTracker) trackTransactions() error {
	for {
		select {
		case h := <-tr.heads:
			b, err := tr.client.BlockByHash(context.Background(), h.Hash())
			if err != nil {
				return err
			}
			// If we have transactions then process them
			if len(b.Transactions()) > 0 {
				tr.processedMu.Lock()
				for _, t := range b.Transactions() {
					tr.processed[t.Hash()] = b
				}
				// signal
				tr.processedMu.Unlock()
				tr.newTxs.Send(struct{}{})
			}
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
func (tr *TransactionTracker) AwaitTransactions(ctx context.Context, hashes []common.Hash) error {
	hashmap := make(map[common.Hash]struct{}, len(hashes))
	for i := range hashes {
		hashmap[hashes[i]] = struct{}{}
	}
	ch := make(chan struct{}, 10)
	sub := tr.newTxs.Subscribe(ch)
	defer sub.Unsubscribe()
	for {
		tr.processedMu.Lock()
		// Delete processed transactions from hashmap
		for hash := range hashmap {
			_, ok := tr.processed[hash]
			if ok {
				delete(hashmap, hash)
			}
		}
		tr.processedMu.Unlock()
		// If there are no transactions left then they have all been processed.
		if len(hashmap) == 0 {
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
func (tr *TransactionTracker) StopTracking() error {
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
