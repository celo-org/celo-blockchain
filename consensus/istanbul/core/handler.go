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

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
)

// Start implements core.Engine.Start
func (c *core) Start() error {
	rsdb, err := newRoundStateDB(c.config.RoundStateDBPath, nil)
	if err != nil {
		log.Crit("Failed to open RoundStateDB", "err", err)
	}
	c.rsdb = rsdb
	roundState, err := c.createRoundState()
	if err != nil {
		return err
	}

	c.current = roundState
	c.roundChangeSetV2 = newRoundChangeSetV2(c.current.ValidatorSet())

	// Reset the Round Change timer for the current round to timeout.
	// (If we've restored RoundState such that we are in StateWaitingForRoundChange,
	// this may also start a timer to send a repeat round change message.)
	c.resetRoundChangeTimer()

	// Process backlog
	c.processPendingRequests()
	c.backlog.updateState(c.CurrentView(), c.current.State())

	// Tests will handle events itself, so we have to make subscribeEvents()
	// be able to call in test.
	c.subscribeEvents()

	c.handlerWg.Add(1)
	go c.handleEvents()

	return nil
}

// Stop implements core.Engine.Stop
func (c *core) Stop() error {
	c.stopAllTimers()
	c.unsubscribeEvents()

	// Make sure the handler goroutine exits
	c.handlerWg.Wait()

	err := c.rsdb.Close()
	c.currentMu.Lock()
	defer c.currentMu.Unlock()
	c.current = nil
	return err
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
		timeoutAndMoveToNextRoundEvent{},
		resendRoundChangeEvent{},
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
	defer c.handlerWg.Done()

	for {
		logger := c.newLogger("func", "handleEvents")
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
				if err := c.handleMsg(ev.Payload); err != nil && err != errFutureMessage && err != errOldMessage {
					logger.Warn("Error in handling istanbul message", "err", err)
				}
			case backlogEvent:
				if payload, err := ev.msg.Payload(); err != nil {
					logger.Error("Error in retrieving payload from istanbul message that was sent from a backlog event", "err", err)
				} else {
					if err := c.handleMsg(payload); err != nil && err != errFutureMessage && err != errOldMessage {
						logger.Warn("Error in handling istanbul message that was sent from a backlog event", "err", err)
					}
				}
			}
		case event, ok := <-c.timeoutSub.Chan():
			if !ok {
				return
			}
			switch ev := event.Data.(type) {
			case timeoutAndMoveToNextRoundEvent:
				if err := c.handleTimeoutAndMoveToNextRound(ev.view); err != nil {
					logger.Error("Error on handleTimeoutAndMoveToNextRound", "err", err)
				}
			case resendRoundChangeEvent:
				if err := c.handleResendRoundChangeEvent(ev.view); err != nil {
					logger.Error("Error on handleResendRoundChangeEvent", "err", err)
				}
			}
		case event, ok := <-c.finalCommittedSub.Chan():
			if !ok {
				return
			}
			switch event.Data.(type) {
			case istanbul.FinalCommittedEvent:
				if err := c.handleFinalCommitted(); err != nil {
					logger.Error("Error on handleFinalCommit", "err", err)
				}
			}
		}
	}
}

// sendEvent sends events to mux
func (c *core) sendEvent(ev interface{}) {
	c.backend.EventMux().Post(ev)
}

func (c *core) handleMsg(payload []byte) error {
	logger := c.newLogger("func", "handleMsg")

	// Decode message and check its signature
	msg := new(istanbul.Message)
	logger.Debug("Got new message", "payload", hexutil.Encode(payload))
	if err := msg.FromPayload(payload, c.validateFn); err != nil {
		logger.Debug("Failed to decode message from payload", "err", err)
		return err
	}

	// Only accept message if the address is valid
	_, src := c.current.ValidatorSet().GetByAddress(msg.Address)
	if src == nil {
		logger.Error("Invalid address in message", "m", msg)
		return istanbul.ErrUnauthorizedAddress
	}

	return c.handleCheckedMsg(msg, src)
}

func (c *core) handleCheckedMsg(msg *istanbul.Message, src istanbul.Validator) error {
	logger := c.newLogger("func", "handleCheckedMsg", "from", msg.Address)

	// Store the message if it's a future message
	catchFutureMessages := func(err error) error {
		if err == errFutureMessage {
			// Store in backlog (if it's not from self)
			if msg.Address != c.address {
				c.backlog.store(msg)
			}
		}
		return err
	}

	switch msg.Code {
	case istanbul.MsgPreprepareV2:
		return catchFutureMessages(c.handlePreprepareV2(msg))
	case istanbul.MsgPrepare:
		return catchFutureMessages(c.handlePrepare(msg))
	case istanbul.MsgCommit:
		return catchFutureMessages(c.handleCommit(msg))
	case istanbul.MsgRoundChangeV2:
		return catchFutureMessages(c.handleRoundChangeV2(msg))
	default:
		logger.Error("Invalid message", "m", msg)
	}

	return errInvalidMessage
}

func (c *core) handleTimeoutAndMoveToNextRound(timedOutView *istanbul.View) error {
	logger := c.newLogger("func", "handleTimeoutAndMoveToNextRound", "timed_out_seq", timedOutView.Sequence, "timed_out_round", timedOutView.Round)

	// Avoid races where message is enqueued then a later event advances sequence or desired round.
	if c.current.Sequence().Cmp(timedOutView.Sequence) != 0 || c.current.DesiredRound().Cmp(timedOutView.Round) != 0 {
		logger.Trace("Timed out but now on a different view")
		return nil
	}

	logger.Debug("Timed out, trying to wait for next round")
	nextRound := new(big.Int).Add(timedOutView.Round, common.Big1)
	return c.waitForDesiredRound(nextRound)
}

func (c *core) handleResendRoundChangeEvent(desiredView *istanbul.View) error {
	logger := c.newLogger("func", "handleResendRoundChangeEvent", "set_at_seq", desiredView.Sequence, "set_at_desiredRound", desiredView.Round)

	// Avoid races where message is enqueued then a later event advances sequence or desired round.
	if c.current.Sequence().Cmp(desiredView.Sequence) != 0 || c.current.DesiredRound().Cmp(desiredView.Round) != 0 {
		logger.Trace("Timed out but now on a different view")
		return nil
	}

	c.resendRoundChangeMessage()
	return nil
}
