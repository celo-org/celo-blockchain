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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

func (c *core) sendPreprepare(request *istanbul.Request, roundChangeCertificate istanbul.RoundChangeCertificate) {
	logger := c.newLogger("func", "sendPreprepare")

	// If I'm the proposer and I have the same sequence with the proposal
	if c.current.Sequence().Cmp(request.Proposal.Number()) == 0 && c.isProposer() {
		curView := c.current.View()
		preprepare, err := Encode(&istanbul.Preprepare{
			View:                   curView,
			Proposal:               request.Proposal,
			RoundChangeCertificate: roundChangeCertificate,
		})
		if err != nil {
			logger.Error("Failed to prepare message")
			return
		}

		msg := &istanbul.Message{
			Code: istanbul.MsgPreprepare,
			Msg:  preprepare,
		}
		logger.Debug("Sending preprepare", "m", msg)
		c.broadcast(msg)
	}
}

func (c *core) handlePreprepare(msg *istanbul.Message) error {
	logger := c.newLogger("func", "handlePreprepare", "tag", "handleMsg", "from", msg.Address)
	logger.Trace("Got preprepare message", "m", msg)

	// Decode PREPREPARE
	var preprepare *istanbul.Preprepare
	err := msg.Decode(&preprepare)
	if err != nil {
		return errFailedDecodePreprepare
	}

	// TODO(tim) Fix and Move checkMessage check up to here

	// Verify that the proposal is for the sequence number of the view we verified.
	if preprepare.View.Sequence.Cmp(preprepare.Proposal.Number()) != 0 {
		logger.Warn("Received preprepare with invalid block number", "number", preprepare.Proposal.Number(), "view_seq", preprepare.View.Sequence, "view_round", preprepare.View.Round)
		return errInvalidProposal
	}

	// Check proposer is valid for the message's view (this may be a subsequent round)
	headBlock, headProposer := c.backend.GetCurrentHeadBlockAndAuthor()
	if headBlock == nil {
		logger.Error("Could not determine head proposer")
		return errNotFromProposer
	}
	proposerForMsgRound := c.selectProposer(c.current.ValidatorSet(), headProposer, preprepare.View.Round.Uint64())
	if proposerForMsgRound.Address() != msg.Address {
		logger.Warn("Ignore preprepare message from non-proposer", "actual_proposer", proposerForMsgRound.Address())
		return errNotFromProposer
	}

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
		// This also moves us to the next round if the certificate is valid.
		err := c.handleRoundChangeCertificate(subject, preprepare.RoundChangeCertificate)
		if err != nil {
			logger.Warn("Invalid round change certificate with preprepare.", "err", err)
			return err
		}
	} else if preprepare.HasRoundChangeCertificate() {
		logger.Error("Preprepare for round 0 has a round change certificate.")
		return errInvalidProposal
	}

	// Ensure we have the same view with the PREPREPARE message.
	if err := c.checkMessage(istanbul.MsgPreprepare, preprepare.View); err != nil {
		if err == errOldMessage {
			// Get validator set for the given proposal
			valSet := c.backend.ParentBlockValidators(preprepare.Proposal)
			prevBlockAuthor := c.backend.AuthorForBlock(preprepare.Proposal.Number().Uint64() - 1)
			proposer := c.selectProposer(valSet, prevBlockAuthor, preprepare.View.Round.Uint64())

			// We no longer broadcast a COMMIT if this is a PREPREPARE from the correct proposer for an existing block.
			// However, we log a WARN for potential future debugging value.
			if proposer.Address() == msg.Address && c.backend.HasBlock(preprepare.Proposal.Hash(), preprepare.Proposal.Number()) {
				logger.Warn("Would have sent a commit message for an old block", "view", preprepare.View, "block_hash", preprepare.Proposal.Hash())
				return nil
			}
		}
		// Probably shouldn't errFutureMessage as we should have moved to that round in handleRoundChangeCertificate
		logger.Trace("Check preprepare failed", "err", err)
		return err
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
