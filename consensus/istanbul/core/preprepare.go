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
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

func (c *core) sendPreprepare(request *istanbul.Request, roundChangeCertificate istanbul.RoundChangeCertificate) {
	logger := c.newLogger("func", "sendPreprepare")

	// If I'm the proposer and I have the same sequence with the proposal
	if c.current.Sequence().Cmp(request.Proposal.Number()) == 0 && c.isProposer() {
		m := istanbul.NewPreprepareMessage(&istanbul.Preprepare{
			View:                   c.current.View(),
			Proposal:               request.Proposal,
			RoundChangeCertificate: roundChangeCertificate,
		}, c.address)
		logger.Debug("Sending preprepare", "m", m)
		c.broadcast(m)
	}
}

func (c *core) handlePreprepare(msg *istanbul.Message) error {
	defer c.handlePrePrepareTimer.UpdateSince(time.Now())

	preprepare := msg.Preprepare()
	logger := c.newLogger().New("func", "handlePreprepare", "tag", "handleMsg", "from", msg.Address, "msg_num", preprepare.Proposal.Number(),
		"msg_hash", preprepare.Proposal.Hash(), "msg_seq", preprepare.View.Sequence, "msg_round", preprepare.View.Round)

	// If round > 0, handle the ROUND CHANGE certificate. If round = 0, it should not have a ROUND CHANGE certificate
	if preprepare.View.Round.Cmp(common.Big0) > 0 {
		if !preprepare.HasRoundChangeCertificate() {
			logger.Error("Preprepare for non-zero round did not contain a round change certificate.")
			return errMissingRoundChangeCertificate
		}
		subject := istanbul.Subject{
			View:   preprepare.View,
			Digest: preprepare.Proposal.Hash(),
		}
		err := c.handleRoundChangeCertificate(c.roundChangeSet, c.current, subject, preprepare.RoundChangeCertificate)
		if err != nil {
			logger.Warn("Invalid round change certificate with preprepare.", "err", err)
			return err
		}
		// May have already moved to this round based on quorum round change messages.
		logger.Trace("Trying to move to round change certificate's round", "target round", preprepare.View.Round)
		// If the round change certificate was valid then move to the round of the preprepare.
		err = c.startNewRound(preprepare.View.Round)
		if err != nil {
			logger.Warn("Failed to move to new round", "err", err)
			return err
		}

	} else if preprepare.HasRoundChangeCertificate() {
		logger.Error("Preprepare for round 0 has a round change certificate.")
		return errInvalidProposal
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

	if c.current.State() == StateAcceptRequest {
		logger.Trace("Accepted preprepare", "tag", "stateTransition")
		c.consensusTimestamp = time.Now()

		err := c.current.TransitionToPreprepared(preprepare)
		if err != nil {
			return err
		}

		// Process Backlog Messages
		c.backlog.updateState(c.current.View(), c.current.State())
		c.sendPrepare()
	}

	return nil
}
