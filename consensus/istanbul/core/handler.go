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
	"fmt"
	"math/big"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/algorithm"
)

// Start implements core.Engine.Start
func (c *core) Start() error {

	roundState, err := c.createRoundState()
	if err != nil {
		return err
	}

	c.current = roundState
	c.oracle = NewRoundStateOracle(roundState, c)
	c.algo = algorithm.NewAlgorithm(c.oracle)
	c.roundChangeSet = newRoundChangeSet(c.current.ValidatorSet())

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
	go c.handleEvents()

	return nil
}

// Stop implements core.Engine.Stop
func (c *core) Stop() error {
	c.stopAllTimers()
	c.unsubscribeEvents()

	// Make sure the handler goroutine exits
	c.handlerWg.Wait()

	c.current = nil
	return nil
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

	c.handlerWg.Add(1)

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

// Returns true if this message is considered a future message.
func IsFutureMsg(currentSequence, desiredRound *big.Int, currentState State, msgView *istanbul.View, msgCode uint64) bool {

	// Future sequences indicate a future message.
	if msgView.Sequence.Cmp(currentSequence) > 0 {
		return true
	}
	// Future rounds inidcate a future message except for round changes
	if msgView.Round.Cmp(desiredRound) > 0 && msgCode != istanbul.MsgRoundChange {
		return true
	}
	// If we are waiting for a proposal or a proposed value then commit and
	// prpepare messages are considered future.
	if currentState.In(StateWaitingForNewRound, StateAcceptRequest) &&
		(msgCode == istanbul.MsgCommit || msgCode == istanbul.MsgPrepare) {
		return true
	}
	return false
}

// buildAlgorithmMsg builds an algorithm message from an istanbul message. It
// takes the oracle because it needs to validate round change certificates and
// generate an associated id. This is pretty clunky, ideally we could represent
// the round change certificate in more detail in the algorithm message and
// then the round change certificate verification logic could be moved to the
// algorithm, but this will be a bit of work.
func buildAlgorithmMsg(msg *istanbul.Message, o *RoundStateOracle) (*algorithm.Msg, error) {
	m := &algorithm.Msg{
		MsgType: algorithm.Type(msg.Code),
	}
	switch msg.Code {
	case istanbul.MsgPreprepare:
		p := msg.Preprepare()
		rccID, err := o.SetRoundChangeCertificate(p)
		if err != nil {
			return nil, fmt.Errorf("failed to set round change certificate in oracle: %w", err)
		}
		m.Height = p.View.Sequence.Uint64()
		m.Round = p.View.Round.Uint64()
		m.Val = algorithm.Value(p.Proposal.Hash())
		m.RoundChangeCert = rccID
	case istanbul.MsgPrepare:
		p := msg.Prepare()
		m.Height = p.View.Sequence.Uint64()
		m.Round = p.View.Round.Uint64()
		m.Val = algorithm.Value(p.Digest)
	case istanbul.MsgCommit:
		c := msg.Commit()
		m.Height = c.Subject.View.Sequence.Uint64()
		m.Round = c.Subject.View.Round.Uint64()
		m.Val = algorithm.Value(c.Subject.Digest)
	case istanbul.MsgRoundChange:
		r := msg.RoundChange()
		m.Height = r.View.Sequence.Uint64()
		m.Round = r.View.Round.Uint64()
	default:
		return nil, errInvalidMessage
	}
	return m, nil
}

func view(m *algorithm.Msg) *istanbul.View {
	return &istanbul.View{
		Round:    new(big.Int).SetUint64(m.Round),
		Sequence: new(big.Int).SetUint64(m.Height),
	}
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
	_, validator := c.current.ValidatorSet().GetByAddress(msg.Address)
	if validator == nil {
		logger.Error("Invalid address in message", "m", msg)
		return istanbul.ErrUnauthorizedAddress
	}

	// Update logger context
	logger = logger.New("from", msg.Address, "curr_view", c.current.View())

	m, err := buildAlgorithmMsg(msg, c.oracle)
	if err != nil {
		return err
	}
	logger = logger.New("msg", m)
	logger.Debug("Handling consensus message")

	// Prior views are always old.
	if view(m).Cmp(c.current.View()) < 0 {
		switch msg.Code {
		case istanbul.MsgCommit:
			// Discard messages from previous views, unless they are commits from the previous sequence,
			// with the same round as what we wound up finalizing, as we would be able to include those
			// to create the ParentAggregatedSeal for our next proposal.
			lastSubject, err := c.backend.LastSubject()
			if err != nil {
				return err
			} else if view(m).Cmp(lastSubject.View) != 0 {
				return errOldMessage
			} else if lastSubject.View.Sequence.Cmp(common.Big0) == 0 {
				// Don't handle commits for the genesis block, will cause underflows
				return errOldMessage
			}
			return c.handleCheckedCommitForPreviousSequence(msg, msg.Commit())
		case istanbul.MsgRoundChange:
			// If the RC message is for the current sequence but a prior round, help the sender fast forward
			// by sending back to it (not broadcasting) a round change message for our desired round.
			if m.Height == c.current.Sequence().Uint64() {
				logger.Trace("Sending round change for desired round to node with a previous desired round", "msg_round", m.Round)
				c.sendRoundChangeAgain(msg.Address)
			}
		}
		return errOldMessage
	}

	// Check if the message is a future message and if so add it to the backlog
	if IsFutureMsg(c.current.Sequence(), c.current.DesiredRound(), c.current.State(), view(m), msg.Code) {
		// Store in backlog (if it's not from self)
		if msg.Address != c.address {
			c.backlog.store(msg)
		}
		return errFutureMessage
	}

	numberOfCommits := c.current.Commits().Size()
	minQuorumSize := c.current.ValidatorSet().MinQuorumSize()
	// At this point we know that messages are either for the current sequence
	// and desired round or its a round change for the current sequence and
	// desired or future round.
	switch msg.Code {
	case istanbul.MsgPreprepare:
		preprepare := msg.Preprepare()
		// Verify that the proposal is for the sequence number of the view we verified.
		if m.Height != preprepare.Proposal.Number().Uint64() {
			logger.Warn("Received preprepare with invalid block number")
			return errInvalidProposal
		}

		// Check proposer is valid for the message's view (this may be a subsequent round)
		headBlock, headProposer := c.backend.GetCurrentHeadBlockAndAuthor()
		if headBlock == nil {
			logger.Error("Could not determine head proposer")
			return errNotFromProposer
		}
		proposerForMsgRound := c.selectProposer(c.current.ValidatorSet(), headProposer, m.Round)
		if proposerForMsgRound.Address() != msg.Address {
			logger.Warn("Ignore preprepare message from non-proposer", "actual_proposer", proposerForMsgRound.Address())
			return errNotFromProposer
		}

		defer c.handlePrePrepareTimer.UpdateSince(time.Now())

		if m.Round > 0 {
			logger.Trace("Trying to move to round change certificate's round", "target round", m.Round)
			err := c.startNewRound(new(big.Int).SetUint64(m.Round))
			if err != nil {
				logger.Warn("Failed to move to new round", "err", err)
				return err
			}
		}
		// Verify the proposal we received
		if duration, err := c.verifyProposal(preprepare.Proposal); err != nil {
			logger.Warn("Failed to verify proposal", "err", err, "duration", duration)
			// if it's a future block, we will handle it again after the duration
			if err == consensus.ErrFutureBlock {
				c.stopFuturePreprepareTimer()
				c.futurePreprepareTimer = time.AfterFunc(duration, func() {
					c.sendEvent(backlogEvent{
						msg: msg,
					})
				})
			}
			return err
		}
	case istanbul.MsgPrepare:
		d := c.current.Subject().Digest
		if common.Hash(m.Val) != d {
			logger.Warn("Inconsistent digest between PREPARE and proposal", "expected", d, "got", m.Val)
			return errInconsistentSubject
		}
		defer c.handlePrepareTimer.UpdateSince(time.Now())
		// Decode PREPARE message

		// Add the PREPARE message to current round state
		if err := c.current.AddPrepare(msg); err != nil {
			logger.Error("Failed to add PREPARE message to round state", "err", err)
			return err
		}

		preparesAndCommits := c.current.GetPrepareOrCommitSize()
		logger.Trace("Accepted prepare", "prepares_and_commits", preparesAndCommits, "commits", c.current.Commits().Size(), "prepares", c.current.Prepares().Size())
		// Update metrics.
		if !c.consensusTimestamp.IsZero() {
			c.consensusPrepareTimeGauge.Update(time.Since(c.consensusTimestamp).Nanoseconds())
		}
	case istanbul.MsgCommit:
		commit := msg.Commit()

		if err := c.verifyCommittedSeal(commit, validator); err != nil {
			return errInvalidCommittedSeal
		}

		newValSet, err := c.backend.NextBlockValidators(c.current.Proposal())
		if err != nil {
			return err
		}

		if err := c.verifyEpochValidatorSetSeal(commit, c.current.Proposal().Number().Uint64(), newValSet, validator); err != nil {
			return errInvalidEpochValidatorSetSeal
		}

		// ensure that the commit is for the current proposal
		d := c.current.Subject().Digest
		if common.Hash(m.Val) != d {
			logger.Warn("Inconsistent subjects between commit and proposal", "expected", d, "got", m.Val)
			return errInconsistentSubject
		}

		defer c.handleCommitTimer.UpdateSince(time.Now())
		// Add the COMMIT message to current round state
		if err := c.current.AddCommit(msg); err != nil {
			logger.Error("Failed to record commit message", "err", err)
			return err
		}
		logger.Trace("Accepted commit for current sequence", "Number of commits", numberOfCommits)
	case istanbul.MsgRoundChange:

		rc := msg.RoundChange()

		// Verify the PREPARED certificate if present.
		if rc.HasPreparedCertificate() {
			_, err := c.verifyPreparedCertificate(rc.PreparedCertificate, m.Round)
			if err != nil {
				return err
			}
		}

		// Add the ROUND CHANGE message to its message set.
		if err := c.roundChangeSet.Add(view(m).Round, msg); err != nil {
			logger.Warn("Failed to add round change message", "err", err)
			return err
		}
		logger.Trace("Got round change message", "rcs", c.roundChangeSet.String())
	default:
		return errInvalidMessage
	}

	// handle the message
	toSend, round, desiredRound := c.algo.HandleMessage(m)
	switch {
	case toSend != nil && toSend.MsgType == algorithm.Prepare:
		logger.Trace("Accepted preprepare", "tag", "stateTransition")
		c.consensusTimestamp = time.Now()

		err := c.current.TransitionToPreprepared(msg.Preprepare())
		if err != nil {
			return err
		}

		// Process Backlog Messages
		c.backlog.updateState(c.current.View(), c.current.State())
		c.sendPrepare()
	// If the target round is set to 0 then we have committed
	case round != nil && *round == 0:
		logger.Trace("Got a quorum of commits", "tag", "stateTransition", "commits", numberOfCommits, "quorum", minQuorumSize)
		err := c.commit()
		if err != nil {
			logger.Error("Failed to commit()", "err", err)
			return err
		}
	case toSend != nil && toSend.MsgType == algorithm.Commit:
		// Change to prepared state if we've received enough PREPARE messages
		// and we are not yet in the prepared state.
		err := c.current.TransitionToPrepared(minQuorumSize)
		if err != nil {
			logger.Error("Failed to create and set prepared certificate", "err", err)
			return err
		}
		// Process Backlog Messages
		c.backlog.updateState(c.current.View(), c.current.State())
		logger.Trace("Got quorum prepares or commits", "tag", "stateTransition", "commits", c.current.Commits, "prepares", c.current.Prepares)
		c.sendCommit()
	case round != nil:
		logger.Debug("Got quorum round change messages, starting new round.", "quorumRound", round)
		return c.startNewRound(new(big.Int).SetUint64(*round))
	case desiredRound != nil:
		logger.Debug("Got f+1 round change messages, sending own round change message and waiting for next round.", "ffRound", desiredRound)
		c.waitForDesiredRound(new(big.Int).SetUint64(*desiredRound))
	}
	return nil

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
