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

package les

import (
	"context"
	"errors"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/rlp"
)

type lesTxRelay struct {
	txSent    map[common.Hash]*types.Transaction
	txPending map[common.Hash]struct{}
	ps        *peerSet
	peerList  []*peer
	lock      sync.RWMutex
	stop      chan struct{}

	retriever *retrieveManager
}

func newLesTxRelay(ps *peerSet, retriever *retrieveManager) *lesTxRelay {
	r := &lesTxRelay{
		txSent:    make(map[common.Hash]*types.Transaction),
		txPending: make(map[common.Hash]struct{}),
		ps:        ps,
		retriever: retriever,
		stop:      make(chan struct{}),
	}
	ps.notify(r)
	return r
}

func (self *lesTxRelay) Stop() {
	close(self.stop)
}

// registerPeer implements peerSetNotify
func (self *lesTxRelay) registerPeer(_ *peer) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.peerList = self.ps.AllPeers()
}

// unregisterPeer implements peerSetNotify
func (self *lesTxRelay) unregisterPeer(_ *peer) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.peerList = self.ps.AllPeers()
}

func (self *lesTxRelay) CanRelayTransaction(tx *types.Transaction) bool {
	self.lock.Lock()
	defer self.lock.Unlock()

	for _, p := range self.peerList {
		if p.WillAcceptTransaction(tx) {
			return true
		}
	}
	return false
}

// send sends a list of transactions to at most a given number of peers at
// once, never resending any particular transaction to the same peer twice
func (self *lesTxRelay) send(txs types.Transactions) {
	for _, tx := range txs {
		hash := tx.Hash()
		if _, ok := self.txSent[hash]; ok {
			continue
		}

		self.txSent[hash] = tx
		self.txPending[hash] = struct{}{}

		// Send a single transaction per request to avoid failure coupling and
		// because the expected base cost of a SendTxV2 request is 0, so it
		// cost no extra to send multiple requests with one transaction each.
		list := types.Transactions{tx}
		enc, _ := rlp.EncodeToBytes(list)

		// Assemble the request object with callbacks for the distributor.
		reqID := genReqID()
		rq := &distReq{
			getCost: func(dp distPeer) uint64 {
				return dp.(*peer).GetTxRelayCost(len(list), len(enc))
			},
			canSend: func(dp distPeer) bool {
				return dp.(*peer).WillAcceptTransaction(tx)
			},
			request: func(dp distPeer) func() {
				peer := dp.(*peer)
				cost := peer.GetTxRelayCost(len(list), len(enc))
				peer.fcServer.QueuedRequest(reqID, cost)
				return func() { peer.SendTxs(reqID, cost, enc) }
			},
		}

		// Check the response to see if the transaction was successfully added to the peer pool or mined.
		// If an error is returned, the retriever will retry with any remaining suitable peers.
		checkTxStatus := func(p distPeer, msg *Msg) error {
			if msg.MsgType != MsgTxStatus {
				return errors.New("received unexpected message code")
			}
			statuses, ok := msg.Obj.([]light.TxStatus)
			if !ok {
				return errors.New("received invalid transaction status object")
			}
			if len(statuses) != 1 {
				return errors.New("expected single transaction status response")
			}
			status := statuses[0]
			if status.Error != "" {
				return errors.New(status.Error)
			}
			if status.Status == core.TxStatusUnknown {
				return errors.New("transaction status unknown")
			}
			return nil
		}
		go self.retriever.retrieve(context.Background(), reqID, rq, checkTxStatus, self.stop)
	}
}

func (self *lesTxRelay) Send(txs types.Transactions) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.send(txs)
}

func (self *lesTxRelay) NewHead(head common.Hash, mined []common.Hash, rollback []common.Hash) {
	self.lock.Lock()
	defer self.lock.Unlock()

	for _, hash := range mined {
		delete(self.txPending, hash)
	}

	for _, hash := range rollback {
		self.txPending[hash] = struct{}{}
	}

	if len(self.txPending) > 0 {
		txs := make(types.Transactions, len(self.txPending))
		i := 0
		for hash := range self.txPending {
			txs[i] = self.txSent[hash]
			i++
		}
		self.send(txs)
	}
}

func (self *lesTxRelay) Discard(hashes []common.Hash) {
	self.lock.Lock()
	defer self.lock.Unlock()

	for _, hash := range hashes {
		delete(self.txSent, hash)
		delete(self.txPending, hash)
	}
}
