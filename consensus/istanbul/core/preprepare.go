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
	"github.com/ethereum/go-ethereum/log"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

func (c *core) sendPreprepare(request *istanbul.Request, roundChangeCertificate istanbul.RoundChangeCertificate) {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "sendPreprepare")

	// If I'm the proposer and I have the same sequence with the proposal
	if c.current.Sequence().Cmp(request.Proposal.Number()) == 0 && c.isProposer() {
		preprepare, err := c.getPreprepareMessage(request, roundChangeCertificate, logger)
		if err != nil {
			logger.Error("Failed to prepare message")
			return
		}

		msg := &istanbul.Message{
			Code: istanbul.MsgPreprepare,
			Msg:  preprepare,
		}
		logger.Trace("Sending pre-prepare", "msg", msg)
		c.broadcast(msg)
	}
}

func (c *core) getPreprepareMessage(
	request *istanbul.Request,
	roundChangeCertificate istanbul.RoundChangeCertificate,
	logger log.Logger) ([]byte, error) {
	curView := c.currentView()
	messageType := istanbul.MsgPreprepare
	roundNumber := c.current.Round()
	sequenceNumber := c.current.Sequence()
	existingPreparedMessage, err := c.getPreprepareMessageFromDisk(
		messageType, roundNumber, sequenceNumber)
	if err != nil {
		logger.Error("Failed to get prepared message from disk", "view", curView)
		return nil, err
	}

	if existingPreparedMessage != nil {
		logger.Info("Got previously prepared messaged from the disk", "msg", existingPreparedMessage)
		return existingPreparedMessage, nil
	}

	preprepare, err := Encode(&istanbul.Preprepare{
		View:                   curView,
		Proposal:               request.Proposal,
		RoundChangeCertificate: roundChangeCertificate,
	})
	if err != nil {
		logger.Error("Failed to encode", "view", curView)
		return nil, err
	}
	err2 := c.savePrepareMessageToDisk(messageType, roundNumber, sequenceNumber, preprepare)
	if err2 != nil {
		logger.Error("Failed to write prepare message to the disk", "msg", preprepare)
		return nil, err2
	}
	return preprepare, nil
}

func (c *core) handlePreprepare(msg *istanbul.Message) error {
	logger := c.logger.New("from", msg.Address, "state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "handlePreprepare", "tag", "handleMsg")
	logger.Trace("Got pre-prepare message", "msg", msg)

	// Decode PRE-PREPARE
	var preprepare *istanbul.Preprepare
	err := msg.Decode(&preprepare)
	if err != nil {
		return errFailedDecodePreprepare
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

	// Ensure we have the same view with the PRE-PREPARE message
	// If it is old message, see if we need to broadcast COMMIT
	if err := c.checkMessage(istanbul.MsgPreprepare, preprepare.View); err != nil {
		if err == errOldMessage {
			// Get validator set for the given proposal
			valSet := c.backend.ParentValidators(preprepare.Proposal).Copy()
			previousProposer := c.backend.GetProposer(preprepare.Proposal.Number().Uint64() - 1)
			valSet.CalcProposer(previousProposer, preprepare.View.Round.Uint64())
			// Broadcast COMMIT if it is an existing block
			// 1. The proposer needs to be a proposer matches the given (Sequence + Round)
			// 2. The given block must exist
			if valSet.IsProposer(msg.Address) && c.backend.HasProposal(preprepare.Proposal.Hash(), preprepare.Proposal.Number()) {
				logger.Trace("Sending a commit message for an old block", "view", preprepare.View, "block hash", preprepare.Proposal.Hash())
				c.sendCommitForOldBlock(preprepare.View, preprepare.Proposal.Hash())
				return nil
			}
		}
		// Probably shouldn't errFutureMessage as we should have moved to that round in handleRoundChangeCertificate
		logger.Trace("Check pre-prepare failed", "cur_round", c.current.Round(), "err", err)
		return err
	}

	// Check if the message comes from current proposer
	if !c.valSet.IsProposer(msg.Address) {
		logger.Warn("Ignore preprepare messages from non-proposer")
		return errNotFromProposer
	}

	// Verify the proposal we received
	if duration, err := c.backend.Verify(preprepare.Proposal); err != nil {
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

	if c.state == StateAcceptRequest {
		logger.Trace("Accepted preprepare", "tag", "stateTransition")
		c.acceptPreprepare(preprepare)
		c.setState(StatePreprepared)
		c.sendPrepare()
	}

	return nil
}

func (c *core) acceptPreprepare(preprepare *istanbul.Preprepare) {
	c.consensusTimestamp = time.Now()
	c.current.SetPreprepare(preprepare)
}
