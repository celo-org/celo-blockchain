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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

// Start implements core.Engine.Start
func (c *core) Start() error {
	// Start a new round from last sequence + 1
	c.startNewRound(common.Big0)

	// Tests will handle events itself, so we have to make subscribeEvents()
	// be able to call in test.
	c.subscribeEvents()
	go c.handleEvents()

	return nil
}

// Stop implements core.Engine.Stop
func (c *core) Stop() error {
	c.stopTimer()
	c.unsubscribeEvents()

	// Make sure the handler goroutine exits
	c.handlerWg.Wait()
	return nil
}

func (c *core) CurrentView() *istanbul.View {
	return c.currentView()
}

// ----------------------------------------------------------------------------

// Subscribe both internal and external events
func (c *core) subscribeEvents() {
	c.events = c.backend.EventMux().Subscribe(
		// external events
		istanbul.RequestEvent{},
		istanbul.MessageEvent{},
		// internal events
		backlogEvent{},
	)
	c.timeoutSub = c.backend.EventMux().Subscribe(
		timeoutEvent{},
	)
	c.finalCommittedSub = c.backend.EventMux().Subscribe(
		istanbul.FinalCommittedEvent{},
	)
}

// Unsubscribe all events
func (c *core) unsubscribeEvents() {
	c.events.Unsubscribe()
	c.timeoutSub.Unsubscribe()
	c.finalCommittedSub.Unsubscribe()
}

func (c *core) handleEvents() {
	// Clear state
	defer func() {
		c.current = nil
		c.handlerWg.Done()
	}()

	c.handlerWg.Add(1)

	for {
		select {
		case event, ok := <-c.events.Chan():
			if !ok {
				return
			}
			// A real event arrived, process interesting content
			switch ev := event.Data.(type) {
			case istanbul.RequestEvent:
				r := &istanbul.Request{
					Proposal: ev.Proposal,
				}
				err := c.handleRequest(r)
				if err == errFutureMessage {
					c.storeRequestMsg(r)
				}
			case istanbul.MessageEvent:
				if err := c.handleMsg(ev.Payload); err != nil {
					c.logger.Debug("Error in handling istanbul message", "err", err)
				}
			case backlogEvent:
				// No need to check signature for internal messages
				if err := c.handleCheckedMsg(ev.msg, ev.src); err != nil {
					c.logger.Error("Error in handling istanbul message that was sent from a backlog event", "err", err)
				}
			}
		case event, ok := <-c.timeoutSub.Chan():
			if !ok {
				return
			}
			switch ev := event.Data.(type) {
			case timeoutEvent:
				c.handleTimeoutMsg(ev.view)
			}
		case event, ok := <-c.finalCommittedSub.Chan():
			if !ok {
				return
			}
			switch event.Data.(type) {
			case istanbul.FinalCommittedEvent:
				c.handleFinalCommitted()
			}
		}
	}
}

// sendEvent sends events to mux
func (c *core) sendEvent(ev interface{}) {
	c.backend.EventMux().Post(ev)
}

func (c *core) handleMsg(payload []byte) error {
	logger := c.logger.New("func", "handleMsg")
	if c.current != nil {
		logger = logger.New("cur_seq", c.current.Sequence(), "cur_round", c.current.Round())
	} else {
		logger = logger.New("cur_seq", 0, "cur_round", -1)
	}

	// Decode message and check its signature
	msg := new(istanbul.Message)
	if err := msg.FromPayload(payload, c.validateFn); err != nil {
		logger.Error("Failed to decode message from payload", "err", err)
		return err
	}

	// Only accept message if the address is valid
	_, src := c.valSet.GetByAddress(msg.Address)
	if src == nil {
		logger.Error("Invalid address in message", "msg", msg)
		return istanbul.ErrUnauthorizedAddress
	}

	return c.handleCheckedMsg(msg, src)
}

func (c *core) handleCheckedMsg(msg *istanbul.Message, src istanbul.Validator) error {
	logger := c.logger.New("address", c.address, "from", msg.Address, "func", "handleCheckedMsg")
	if c.current != nil {
		logger = logger.New("cur_seq", c.current.Sequence(), "cur_round", c.current.Round())
	} else {
		logger = logger.New("cur_seq", 0, "cur_round", -1)
	}

	// Store the message if it's a future message
	testBacklog := func(err error) error {
		if err == errFutureMessage {
			c.storeBacklog(msg, src)
		}

		return err
	}

	switch msg.Code {
	case istanbul.MsgPreprepare:
		return testBacklog(c.handlePreprepare(msg))
	case istanbul.MsgPrepare:
		return testBacklog(c.handlePrepare(msg))
	case istanbul.MsgCommit:
		return testBacklog(c.handleCommit(msg))
	case istanbul.MsgRoundChange:
		return testBacklog(c.handleRoundChange(msg))
	default:
		logger.Error("Invalid message", "msg", msg)
	}

	return errInvalidMessage
}

func (c *core) handleTimeoutMsg(timeoutView *istanbul.View) {
	logger := c.logger.New("func", "handleTimeoutMsg")
	if c.current != nil {
		logger = logger.New("cur_seq", c.current.Sequence(), "cur_round", c.current.Round())
	} else {
		logger = logger.New("cur_seq", 0, "cur_round", -1)
	}

	// Don't round change on old timeouts
	if timeoutView.Cmp(c.currentView()) < 0 {
		return
	}
	// Send a round change message and transition to a waiting state if we have not done so already.
	// Do not move to next round until quorum round change message or valid round change certificate.
	if !c.waitingForNewRound {
		logger.Trace("round change timeout, sending round change message", "round", c.current.Round())
		c.sendNextRoundChange()
		c.waitingForNewRound = true
	} else {
		logger.Trace("round change timeout, already waiting for next round", "round", c.current.Round())
	}
}
