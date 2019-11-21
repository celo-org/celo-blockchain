// Copyright 2017 The go-ethereum Authors
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
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"math/big"
	"sort"
)

var (
	// msgPriority is defined for calculating processing priority to speedup consensus
	// istanbul.MsgPreprepare > istanbul.MsgCommit > istanbul.MsgPrepare
	msgPriority = map[uint64]int{
		istanbul.MsgPreprepare: 1,
		istanbul.MsgCommit:     2,
		istanbul.MsgPrepare:    3,
	}

	// Do not accept messages for views more than this many sequences in the future.
	acceptMaxFutureSequence             = big.NewInt(10)
	acceptMaxFutureMsgsFromOneValidator = 1000
	acceptMaxFutureMessages             = 10 * 1000
	acceptMaxFutureMessagesPruneBatch   = 100
)

// checkMessage checks the message state
// return errInvalidMessage if the message is invalid
// return errFutureMessage if the message view is larger than current view
// return errOldMessage if the message view is smaller than current view
func (c *core) checkMessage(msgCode uint64, view *istanbul.View) error {
	if view == nil || view.Sequence == nil || view.Round == nil {
		return errInvalidMessage
	}

	// Never accept messages too far into the future
	if view.Sequence.Cmp(new(big.Int).Add(c.currentView().Sequence, acceptMaxFutureSequence)) > 0 {
		return errTooFarInTheFutureMessage
	}

	// Round change messages should be in the same sequence but be >= the desired round
	if msgCode == istanbul.MsgRoundChange {
		if view.Sequence.Cmp(c.current.Sequence()) > 0 {
			return errFutureMessage
		} else if view.Round.Cmp(c.current.DesiredRound()) < 0 {
			return errOldMessage
		}
		return nil
	}

	if view.Cmp(c.current.View()) > 0 {
		return errFutureMessage
	}

	// Discard messages from previous views, unless they are commits from the previous sequence,
	// with the same round as what we wound up finalizing, as we would be able to include those
	// to create the ParentAggregatedSeal for our next proposal.
	if view.Cmp(c.current.View()) < 0 {
		if msgCode == istanbul.MsgCommit {

			lastSubject, err := c.backend.LastSubject()
			if err != nil {
				return err
			}
			if view.Cmp(lastSubject.View) == 0 {
				return nil
			}
		}
		return errOldMessage
	}

	// Round change messages are already let through.
	if c.state == StateWaitingForNewRound {
		return errFutureMessage
	}

	// StateAcceptRequest only accepts istanbul.MsgPreprepare
	// other messages are future messages
	if c.state == StateAcceptRequest && msgCode > istanbul.MsgPreprepare {
		return errFutureMessage
	}

	// For states(StatePreprepared, StatePrepared, StateCommitted),
	// can accept all message types if processing with same view
	return nil
}

func (c *core) storeBacklog(msg *istanbul.Message) {
	logger := c.newLogger("func", "storeBacklog", "from", msg.Address)

	if msg.Address == c.address {
		logger.Warn("Backlog from self")
		return
	}

	var v *istanbul.View
	switch msg.Code {
	case istanbul.MsgPreprepare:
		var p *istanbul.Preprepare
		err := msg.Decode(&p)
		if err != nil {
			return
		}
		v = p.View
	case istanbul.MsgPrepare:
		fallthrough
	case istanbul.MsgCommit:
		var p *istanbul.Subject
		err := msg.Decode(&p)
		if err != nil {
			return
		}
		v = p.View
	case istanbul.MsgRoundChange:
		var p *istanbul.RoundChange
		err := msg.Decode(&p)
		if err != nil {
			return
		}
		v = p.View
	}

	logger.Trace("Store future message", "msg", msg)

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	// Check and inc per-validator future message limit
	if c.backlogCountByVal[msg.Address] > acceptMaxFutureMsgsFromOneValidator {
		logger.Trace("Dropping: backlog exceeds per-src cap", "msg.address", msg.Address)
		return
	}
	c.backlogCountByVal[msg.Address]++
	c.backlogTotal++

	// Add message to per-seq list
	backlogForSeq := c.backlogBySeq[v.Sequence.Uint64()]
	if backlogForSeq == nil {
		backlogForSeq = prque.New(nil)
		c.backlogBySeq[v.Sequence.Uint64()] = backlogForSeq
	}

	backlogForSeq.Push(msg, toPriority(msg.Code, v))

	// Keep backlog below total max size by pruning future-most sequence first
	// (we always leave one sequence's entire messages and rely on per-validator limits)
	if c.backlogTotal > acceptMaxFutureMessages {
		backlogSeqs := c.getSortedBacklogSeqs()
		for i := len(backlogSeqs) - 1; i > 0; i-- {
			seq := backlogSeqs[i]
			if seq <= c.currentView().Sequence.Uint64() ||
				c.backlogTotal < (acceptMaxFutureMessages-acceptMaxFutureMessagesPruneBatch) {
				break
			}
			c.drainBacklogForSeq(seq, nil)
		}
	}
}

// Return slice of sequences present in backlog sorted in ascending order
// Call with backlogsMu held.
func (c *core) getSortedBacklogSeqs() []uint64 {
	backlogSeqs := make([]uint64, len(c.backlogBySeq))
	i := 0
	for k := range c.backlogBySeq {
		backlogSeqs[i] = k
		i++
	}
	sort.Slice(backlogSeqs, func(i, j int) bool {
		return backlogSeqs[i] < backlogSeqs[j]
	})
	return backlogSeqs
}

// Drain a backlog for a given sequence, passing each to optional callback.
// Call with backlogsMu held.
func (c *core) drainBacklogForSeq(seq uint64, cb func(*istanbul.Message, istanbul.Validator)) {
	backlogForSeq := c.backlogBySeq[seq]
	if backlogForSeq == nil {
		return
	}

	// TODO(asa): Only loop N times
	for !backlogForSeq.Empty() {
		m := backlogForSeq.PopItem()
		msg := m.(*istanbul.Message)
		if cb != nil {
			cb(msg)
		}
		// TODO(asa): If falls to 0, remove entry from map
		c.backlogCountByVal[msg.Address]--
		c.backlogTotal--
	}
	// TODO(asa): Check if empty
	delete(c.backlogBySeq, seq)
}

func (c *core) processBacklog() {

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	for _, seq := range c.getSortedBacklogSeqs() {

		logger := c.newLogger("func", "processBacklog", "from", src)

		if seq < c.currentView().Sequence.Uint64() {
			// Earlier sequence. Prune all messages.
			c.drainBacklogForSeq(seq, nil)
		} else if seq == c.currentView().Sequence.Uint64() {
			// Current sequence. Process all in order.
			c.drainBacklogForSeq(seq, func(msg *istanbul.Message) {
				var view *istanbul.View
				switch msg.Code {
				case istanbul.MsgPreprepare:
					var m *istanbul.Preprepare
					err := msg.Decode(&m)
					if err == nil {
						view = m.View
					}
				case istanbul.MsgPrepare:
					fallthrough
				case istanbul.MsgCommit:
					var sub *istanbul.Subject
					err := msg.Decode(&sub)
					if err == nil {
						view = sub.View
					}
				case istanbul.MsgRoundChange:
					var rc *istanbul.RoundChange
					err := msg.Decode(&rc)
					if err == nil {
						view = rc.View
					}
				}
				if view == nil {
					logger.Error("Nil view", "msg", msg)
					// continue
					return
				}
				err := c.checkMessage(msg.Code, view)
				if err == nil {
					logger.Trace("Post backlog event", "msg", msg)

					go c.sendEvent(backlogEvent{
						src: src,
						msg: msg,
					})
				} else if err == errFutureMessage {
					// TODO(asa): Why is this unexpected? It could be for a future round...
					// FIXME: By pushing back to the backlog, we will never finish draining...
					logger.Warn("Unexpected future message, pushing back to the backlog", "msg", msg)
					c.storeBacklog(msg)
				} else {
					logger.Trace("Skip the backlog event", "msg", msg, "err", err)
				}
			})
		}
	}
}

func toPriority(msgCode uint64, view *istanbul.View) int64 {
	if msgCode == istanbul.MsgRoundChange {
		// msgRoundChange comes first
		return 0
	}
	// 10 * Round limits the range possible message codes to [0, 9]
	// FIXME: Check for integer overflow
	return -int64(view.Round.Uint64()*10 + uint64(msgPriority[msgCode]))
}
