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
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
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
func (c *core) checkMessage(msgCode uint64, msgView *istanbul.View) error {
	if msgView == nil || msgView.Sequence == nil || msgView.Round == nil {
		return errInvalidMessage
	}

	if msgView.Cmp(c.current.View()) < 0 {
		return errOldMessage
	} else if msgView.Sequence.Cmp(c.current.Sequence()) > 0 {
		// sequence is bigger, definitely future message
		return errFutureMessage
	} else {
		// same sequence && msgRound >= currentRound

		// Accept all RoundChange (also future rounds)
		// but check again desired round
		if msgCode == istanbul.MsgRoundChange {
			if msgView.Round.Cmp(c.current.DesiredRound()) < 0 {
				return errOldMessage
			}
			return nil
		}

		// TODO we should check directly against the desired round
		// there's no sense in accepting (or storing) messages on the range [currentRound, desiredRound]

		if msgView.Round.Cmp(c.current.View().Round) > 0 || c.current.State() == StateWaitingForNewRound {
			return errFutureMessage
		}

		// StateAcceptRequest only accepts istanbul.MsgPreprepare
		// other messages are future messages
		if c.current.State() == StateAcceptRequest && msgCode != istanbul.MsgPreprepare {
			return errFutureMessage
		}

		// For states(StatePreprepared, StatePrepared, StateCommitted),
		// can accept all message types if processing with same view
		return nil
	}
}

// MsgBacklog represent a backlog of future messages
// It works by:
//     - allowing storing messages with "store()"
//     - call eventListener when a backlog message becomes "present"
//     - updates its notion of time/state with updateState()
type MsgBacklog interface {
	// store atttemps to store the message in the backlog
	// it might not do so, if the message is too far in the future
	store(msg *istanbul.Message)

	// updateState updates the notion of time/state of the backlog,
	// as a side effect it will call the eventListener for all backlog
	// messages that belong to the current "state"
	updateState(view *istanbul.View, state State)
}

type msgBacklogImpl struct {
	backlogBySeq  map[uint64]*prque.Prque
	msgCountBySrc map[common.Address]int
	msgCount      int

	currentView  *istanbul.View
	currentState State

	backlogsMu   *sync.Mutex
	msgProcessor func(*istanbul.Message)
	checkMessage func(msgCode uint64, msgView *istanbul.View) error
	logger       log.Logger
}

func newMsgBacklog(msgProcessor func(*istanbul.Message), checkMessage func(msgCode uint64, msgView *istanbul.View) error) MsgBacklog {
	initialView := &istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(1),
	}

	return &msgBacklogImpl{
		backlogBySeq:  make(map[uint64]*prque.Prque),
		msgCountBySrc: make(map[common.Address]int),
		msgCount:      0,

		currentView:  initialView,
		currentState: StateAcceptRequest,

		msgProcessor: msgProcessor,
		checkMessage: checkMessage,
		backlogsMu:   new(sync.Mutex),
		logger:       log.New("type", "MsgBacklog"),
	}
}

func (c *msgBacklogImpl) store(msg *istanbul.Message) {
	logger := c.logger.New("func", "store", "from", msg.Address)

	view, err := extractMessageView(msg)

	if err != nil {
		return
	}

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	// Never accept messages too far into the future
	if view.Sequence.Cmp(new(big.Int).Add(c.currentView.Sequence, acceptMaxFutureSequence)) > 0 {
		logger.Debug("Dropping message", "reason", "too far in the future", "m", msg)
		return
	}

	// Check and inc per-validator future message limit
	if c.msgCountBySrc[msg.Address] > acceptMaxFutureMsgsFromOneValidator {
		logger.Debug("Dropping message", "reason", "exceeds per-address cap")
		return
	}

	logger.Trace("Store future message", "m", msg)
	c.msgCountBySrc[msg.Address]++
	c.msgCount++

	// Add message to per-seq list
	backlogForSeq := c.backlogBySeq[view.Sequence.Uint64()]
	if backlogForSeq == nil {
		backlogForSeq = prque.New(nil)
		c.backlogBySeq[view.Sequence.Uint64()] = backlogForSeq
	}

	backlogForSeq.Push(msg, toPriority(msg.Code, view))

	// After insert, remove messages if we have more than "acceptMaxFutureMessages"
	c.removeMessagesOverflow()
}

// removeMessagesOverflow will remove messages if necessary to maintain the number of messages <= acceptMaxFutureMessages
// For that, it will remove messages that further on the future
func (c *msgBacklogImpl) removeMessagesOverflow() {
	// Keep backlog below total max size by pruning future-most sequence first
	// (we always leave one sequence's entire messages and rely on per-validator limits)
	if c.msgCount > acceptMaxFutureMessages {
		backlogSeqs := c.getSortedBacklogSeqs()
		for i := len(backlogSeqs) - 1; i > 0; i-- {
			seq := backlogSeqs[i]
			if seq <= c.currentView.Sequence.Uint64() ||
				c.msgCount < (acceptMaxFutureMessages-acceptMaxFutureMessagesPruneBatch) {
				break
			}
			c.clearBacklogForSeq(seq)
		}
	}
}

// Return slice of sequences present in backlog sorted in ascending order
// Call with backlogsMu held.
func (c *msgBacklogImpl) getSortedBacklogSeqs() []uint64 {
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

// clearBacklogForSeq will remove all entries in the backlog
// for the given seq
func (c *msgBacklogImpl) clearBacklogForSeq(seq uint64) {
	c.processBacklogForSeq(seq, func(_ *istanbul.Message) bool { return true })
}

// processBacklogForSeq will call process() with each entry of the backlog
// for the given seq, until process return "false".
// The entry on which process() returned false will remain in the backlog
func (c *msgBacklogImpl) processBacklogForSeq(seq uint64, process func(*istanbul.Message) bool) {
	backlogForSeq := c.backlogBySeq[seq]
	if backlogForSeq == nil {
		return
	}

	backlogSize := backlogForSeq.Size()
	for i := 0; i < backlogSize; i++ {
		m, priority := backlogForSeq.Pop()
		msg := m.(*istanbul.Message)

		shouldStop := process(msg)

		if shouldStop {
			backlogForSeq.Push(m, priority)
			break
		}

		c.msgCountBySrc[msg.Address]--
		if c.msgCountBySrc[msg.Address] == 0 {
			delete(c.msgCountBySrc, msg.Address)
		}
		c.msgCount--
	}

	if backlogForSeq.Size() == 0 {
		delete(c.backlogBySeq, seq)
	}
}

func (c *msgBacklogImpl) updateState(view *istanbul.View, state State) {
	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	c.currentState = state
	c.currentView = view

	c.processBacklog()
}

func (c *msgBacklogImpl) processBacklog() {
	for _, seq := range c.getSortedBacklogSeqs() {

		logger := c.logger.New("func", "processBacklog", "current_view", c.currentView, "current_state", c.currentState.String(), "for_seq", seq)

		if seq < c.currentView.Sequence.Uint64() {
			// Earlier sequence. Prune all messages.
			c.clearBacklogForSeq(seq)
		} else if seq == c.currentView.Sequence.Uint64() {
			// Current sequence. Process all in order.
			c.processBacklogForSeq(seq, func(msg *istanbul.Message) bool {
				view, err := extractMessageView(msg)
				if err != nil {
					logger.Warn("Error decoding msg", "m", msg, "err", err)
					return false
				}
				if view == nil {
					logger.Warn("Nil view", "m", msg)
					return false
				}

				err = c.checkMessage(msg.Code, view)

				if err == errFutureMessage {
					logger.Debug("Future message in backlog for seq, pushing back to the backlog", "m", msg)
					return true
				}

				if err == nil {
					logger.Trace("Post backlog event", "m", msg)
					go c.msgProcessor(msg)
				} else {
					logger.Trace("Skip the backlog event", "m", msg, "err", err)
				}
				return false
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

func extractMessageView(msg *istanbul.Message) (*istanbul.View, error) {
	var v *istanbul.View
	switch msg.Code {
	case istanbul.MsgPreprepare:
		var p *istanbul.Preprepare
		err := msg.Decode(&p)
		if err != nil {
			return nil, err
		}
		v = p.View
	case istanbul.MsgPrepare:
		var p *istanbul.Subject
		err := msg.Decode(&p)
		if err != nil {
			return nil, err
		}
		v = p.View
	case istanbul.MsgCommit:
		var cs *istanbul.CommittedSubject
		err := msg.Decode(&cs)
		if err != nil {
			return nil, err
		}
		v = cs.Subject.View
	case istanbul.MsgRoundChange:
		var p *istanbul.RoundChange
		err := msg.Decode(&p)
		if err != nil {
			return nil, err
		}
		v = p.View
	}
	return v, nil
}
