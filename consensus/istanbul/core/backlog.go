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
	// msgPreprepare > msgCommit > msgPrepare
	msgPriority = map[uint64]int{
		msgPreprepare: 1,
		msgCommit:     2,
		msgPrepare:    3,
	}

	acceptMaxFutureViews                = big.NewInt(10)
	acceptMaxFutureMsgsFromOneValidator = 1000
	acceptMaxFutureMessages             = 100 * 100
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
	if view.Sequence.Cmp(new(big.Int).Add(c.currentView().Sequence, acceptMaxFutureViews)) > 0 {
		return errTooFarInTheFutureMessage
	}

	if msgCode == msgRoundChange {
		if view.Sequence.Cmp(c.currentView().Sequence) > 0 {
			return errFutureMessage
		} else if view.Cmp(c.currentView()) < 0 {
			return errOldMessage
		}
		return nil
	}

	if view.Cmp(c.currentView()) > 0 {
		return errFutureMessage
	}

	if view.Cmp(c.currentView()) < 0 {
		return errOldMessage
	}

	if c.waitingForRoundChange {
		return errFutureMessage
	}

	// StateAcceptRequest only accepts msgPreprepare
	// other messages are future messages
	if c.state == StateAcceptRequest {
		if msgCode > msgPreprepare {
			return errFutureMessage
		}
		return nil
	}

	// For states(StatePreprepared, StatePrepared, StateCommitted),
	// can accept all message types if processing with same view
	return nil
}

func (c *core) storeBacklog(msg *message, src istanbul.Validator) {
	logger := c.logger.New("from", src, "state", c.state)

	if src.Address() == c.Address() {
		logger.Warn("Backlog from self")
		return
	}

	var v *istanbul.View
	switch msg.Code {
	case msgPreprepare:
		var p *istanbul.Preprepare
		err := msg.Decode(&p)
		if err != nil {
			return
		}
		v = p.View
	// for msgRoundChange, msgPrepare and msgCommit cases
	default:
		var p *istanbul.Subject
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
		logger.Trace("Dropping: backlog exceeds per-src cap", "src", src)
		return
	}
	c.backlogCountByVal[src.Address()]++
	c.backlogTotal++

	// Add message to per-seq list
	backlogForSeq := c.backlogBySeq[v.Sequence.Uint64()]
	if backlogForSeq == nil {
		backlogForSeq = prque.New(nil)
		c.backlogBySeq[v.Sequence.Uint64()] = backlogForSeq
	}

	backlogForSeq.Push(msg, toPriority(msg.Code, v))

	// Keep backlog below total max size by pruning most distant sequence first
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
func (c *core) drainBacklogForSeq(seq uint64, cb func(*message, istanbul.Validator)) {
	backlogForSeq := c.backlogBySeq[seq]
	if backlogForSeq == nil {
		return
	}

	for !backlogForSeq.Empty() {
		m := backlogForSeq.PopItem()
		msg := m.(*message)
		if cb != nil {
			_, src := c.valSet.GetByAddress(msg.Address)
			if src != nil {
				cb(msg, src)
			}
		}
		c.backlogCountByVal[msg.Address]--
		c.backlogTotal--
	}
	delete(c.backlogBySeq, seq)
}

func (c *core) processBacklog() {

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	for _, seq := range c.getSortedBacklogSeqs() {

		logger := c.logger.New("state", c.state, "seq", seq)

		if seq < c.currentView().Sequence.Uint64() {
			// Earlier sequence. Prune all messages.
			c.drainBacklogForSeq(seq, nil)
		} else if seq == c.currentView().Sequence.Uint64() {
			// Current sequence. Process all in order.
			c.drainBacklogForSeq(seq, func(msg *message, src istanbul.Validator) {
				var view *istanbul.View
				switch msg.Code {
				case msgPreprepare:
					var m *istanbul.Preprepare
					err := msg.Decode(&m)
					if err == nil {
						view = m.View
					}
					// for msgRoundChange, msgPrepare and msgCommit cases
				default:
					var sub *istanbul.Subject
					err := msg.Decode(&sub)
					if err == nil {
						view = sub.View
					}
				}
				if view == nil {
					logger.Debug("Nil view", "msg", msg)
					return
				}
				err := c.checkMessage(msg.Code, view)
				if err != nil {
					if err == errFutureMessage {
						logger.Warn("Unexpected future message!", "msg", msg)
						//backlog.Push(msg, prio)
					}
					logger.Trace("Skip the backlog event", "msg", msg, "err", err)
					return
				}
				logger.Trace("Post backlog event", "msg", msg)

				go c.sendEvent(backlogEvent{
					src: src,
					msg: msg,
				})
			})
		} else {
			// got on to future messages.
			return
		}
	}
}

func toPriority(msgCode uint64, view *istanbul.View) int64 {
	if msgCode == msgRoundChange {
		// msgRoundChange comes first
		return 0
	}
	// FIXME: round will be reset as 0 while new sequence
	// 10 * Round limits the range of message code is from 0 to 9
	return -int64(view.Round.Uint64()*10 + uint64(msgPriority[msgCode]))
}
