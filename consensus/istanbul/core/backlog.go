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
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

var (
	// msgPriority is defined for calculating processing priority to speedup consensus
	// istanbul.MsgPreprepare > istanbul.MsgCommit > istanbul.MsgPrepare
	msgPriority = map[uint64]int{
		istanbul.MsgPreprepare: 1,
		istanbul.MsgCommit:     2,
		istanbul.MsgPrepare:    3,
	}
)

// checkMessage checks the message state
// return errInvalidMessage if the message is invalid
// return errFutureMessage if the message view is larger than current view
// return errOldMessage if the message view is smaller than current view
func (c *core) checkMessage(msgCode uint64, view *istanbul.View) error {
	if view == nil || view.Sequence == nil || view.Round == nil {
		return errInvalidMessage
	}

	// Round change messages should be in the same sequence but be >= the desired round
	if msgCode == istanbul.MsgRoundChange {
		if view.Sequence.Cmp(c.currentView().Sequence) > 0 {
			return errFutureMessage
		} else if view.Round.Cmp(c.current.DesiredRound()) < 0 {
			return errOldMessage
		}
		return nil
	}

	if view.Cmp(c.currentView()) > 0 {
		return errFutureMessage
	}

	// as soon as we move on to the next sequence, we discard any messages from
	// a past sequence as invalid old messages unless they are Commits of the
	// previous sequence
	if view.Cmp(c.currentView()) < 0 {
		// Let commits from the previous sequence through so that the handler
		// adds them to the view's parent commits. The round does not matter in this case,
		// as we only care that the message's committed seal matches the previous block.
		viewSequencePlusOne := big.NewInt(0).Add(view.Sequence, common.Big1)
		// if the received view's sequence +1 equals the current view, then the
		// received view corresponds to the parent block
		if msgCode == istanbul.MsgCommit && c.currentView().Sequence.Cmp(viewSequencePlusOne) == 0 {
			return nil
		}
		return errOldMessage
	}

	// Round change messages are already let through.
	if c.state == StateWaitingForNewRound {
		return errFutureMessage
	}

	// StateAcceptRequest only accepts istanbul.MsgPreprepare
	// other messages are future messages
	if c.state == StateAcceptRequest {
		if msgCode > istanbul.MsgPreprepare {
			return errFutureMessage
		}
		return nil
	}

	// For states(StatePreprepared, StatePrepared, StateCommitted),
	// can accept all message types if processing with same view
	return nil
}

func (c *core) storeBacklog(msg *istanbul.Message, src istanbul.Validator) {
	logger := c.logger.New("from", msg.Address, "state", c.state, "func", "storeBacklog")
	if c.current != nil {
		logger = logger.New("cur_seq", c.current.Sequence(), "cur_round", c.current.Round())
	} else {
		logger = logger.New("cur_seq", 0, "cur_round", -1)
	}

	if msg.Address == c.Address() {
		logger.Warn("Backlog from self")
		return
	}

	logger.Trace("Store future message")

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	backlog := c.backlogs[src]
	if backlog == nil {
		backlog = prque.New(nil)
	}
	switch msg.Code {
	case istanbul.MsgPreprepare:
		var p *istanbul.Preprepare
		err := msg.Decode(&p)
		if err == nil {
			backlog.Push(msg, toPriority(msg.Code, p.View))
		}
	case istanbul.MsgPrepare:
		fallthrough
	case istanbul.MsgCommit:
		var p *istanbul.Subject
		err := msg.Decode(&p)
		if err == nil {
			backlog.Push(msg, toPriority(msg.Code, p.View))
		}
	case istanbul.MsgRoundChange:
		var p *istanbul.RoundChange
		err := msg.Decode(&p)
		if err == nil {
			backlog.Push(msg, toPriority(msg.Code, p.View))
		}
	}
	c.backlogs[src] = backlog
}

func (c *core) processBacklog() {
	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	for src, backlog := range c.backlogs {
		if backlog == nil {
			continue
		}

		logger := c.logger.New("from", src, "state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "processBacklog")
		isFuture := false

		// We stop processing if
		//   1. backlog is empty
		//   2. The first message in queue is a future message
		for !(backlog.Empty() || isFuture) {
			m, prio := backlog.Pop()
			msg := m.(*istanbul.Message)
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
				logger.Debug("Nil view", "msg", msg)
				continue
			}
			// Push back if it's a future message
			err := c.checkMessage(msg.Code, view)
			if err != nil {
				if err == errFutureMessage {
					logger.Trace("Stop processing backlog", "msg", msg)
					backlog.Push(msg, prio)
					isFuture = true
					break
				}
				logger.Trace("Skip the backlog event", "msg", msg, "err", err)
				continue
			}
			logger.Trace("Post backlog event", "msg", msg)

			go c.sendEvent(backlogEvent{
				src: src,
				msg: msg,
			})
		}
	}
}

func toPriority(msgCode uint64, view *istanbul.View) int64 {
	if msgCode == istanbul.MsgRoundChange {
		// For istanbul.MsgRoundChange, set the message priority based on its sequence
		return -int64(view.Sequence.Uint64() * 1000)
	}
	// FIXME: round will be reset as 0 while new sequence
	// 10 * Round limits the range of message code is from 0 to 9
	// 1000 * Sequence limits the range of round is from 0 to 99
	return -int64(view.Sequence.Uint64()*1000 + view.Round.Uint64()*10 + uint64(msgPriority[msgCode]))
}
