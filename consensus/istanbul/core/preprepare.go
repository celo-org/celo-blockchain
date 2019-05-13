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

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

// What happens with multiple round changes?
func (c *core) sendPreprepare(request *istanbul.Request, roundChangeCertificate []message) {
	logger := c.logger.New("state", c.state)

	// If I'm the proposer and I have the same sequence with the proposal
	if c.current.Sequence().Cmp(request.Proposal.Number()) == 0 && c.isProposer() {
		curView := c.currentView()
		preprepare, err := Encode(&istanbul.Preprepare{
			View:                   curView,
			Proposal:               request.Proposal,
			RoundChangeCertificate: roundChangeCertificate,
		})
		if err != nil {
			logger.Error("Failed to encode", "view", curView)
			return
		}

		c.broadcast(&message{
			Code: msgPreprepare,
			Msg:  preprepare,
		})
	}
}

func (c *core) ValidateRoundChangeCertificate(roundChangeCertificate []message) error {
	if (len(roundChangeCertificate) > c.valSet.Size() || len(roundChangeCertificate) < 2 * c.valSet.F() + 1) {
		return errInvalidRoundChangeCertificate
	}

	seen := make(map[common.Address]bool)
	for k, message := range rc.RoundChangeCertificate {
		// Verify message signed by a validator
		if signer, err := CheckValidatorSignature(c.valSet, message.Msg, message.Signature); err != nil {
			return errInvalidRoundChangeCertificate
		}

		if signer != message.Address {
			return errInvalidRoundChangeCertificate
		}

		// Check for duplicate messages
		if seen[signer] {
			return errInvalidRoundChangeCertificate
		}
		seen[signer] = true

		// Check that the message is a Prepare message
		if msgRoundChange != message.Code {
			return errInvalidRoundChangeCertificate
		}

		var roundChange *istanbul.RoundChange
		if err := msg.Decode(&roundChange); err != nil {
			logger.Error("Failed to decode ROUND CHANGE in certificate", "err", err)
			return errInvalidRoundChangeCertificate
		}

		// Verify prepare certificate for the proper view
		if err := c.checkMessage(msgRoundChange, roundChange.View; err != nil {
			return errInvalidRoundChangeCertificate
		}

		if roundChange.PreparedCertificate != nil {
			if err := c.ValidatePreparedCertificate(roundChange.PreparedCertificate); err != nil {
				return errInvalidRoundChangeCertificate
			}
		}
	}
	return nil
}

func (c *core) handlePreprepare(msg *message, src istanbul.Validator) error {
	logger := c.logger.New("from", src, "state", c.state)

	// Decode PRE-PREPARE
	var preprepare *istanbul.Preprepare
	err := msg.Decode(&preprepare)
	if err != nil {
		return errFailedDecodePreprepare
	}

	// Ensure we have the same view with the PRE-PREPARE message
	// If it is old message, see if we need to broadcast COMMIT
	if err := c.checkMessage(msgPreprepare, preprepare.View); err != nil {
		if err == errOldMessage {
			// Get validator set for the given proposal
			valSet := c.backend.ParentValidators(preprepare.Proposal).Copy()
			previousProposer := c.backend.GetProposer(preprepare.Proposal.Number().Uint64() - 1)
			valSet.CalcProposer(previousProposer, preprepare.View.Round.Uint64())
			// Broadcast COMMIT if it is an existing block
			// 1. The proposer needs to be a proposer matches the given (Sequence + Round)
			// 2. The given block must exist
			if valSet.IsProposer(src.Address()) && c.backend.HasProposal(preprepare.Proposal.Hash(), preprepare.Proposal.Number()) {
				c.sendCommitForOldBlock(preprepare.View, preprepare.Proposal.Hash())
				return nil
			}
		}
		return err
	}

	// If round > 0, validate the existence of a valid ROUND CHANGE certificate.
	if preprepare.View.Round.Cmp(common.Big0) > 0 {
		if preprepare.RoundChangeCertificate == nil {
			return errMissingRoundChangeCertificate
		}
		if err := c.ValidateRoundChangeCertificate(preprepare.RoundChangeCertificate); err != nil {
			return err
		}

		// If there is a PREPARED certificate in at least one of the ROUND CHANGE messages, verify that
		// a PREPARED certificate for the proposal exists.
		// TODO(asa)
	}

	// Check if the message comes from current proposer
	if !c.valSet.IsProposer(src.Address()) {
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
					src: src,
					msg: msg,
				})
			})
		} else {
			c.sendNextRoundChange()
		}
		return err
	}

	// TODO(asa): Not sure what to do here
	// Here is about to accept the PRE-PREPARE
	if c.state == StateAcceptRequest {
		// Send ROUND CHANGE if the locked proposal and the received proposal are different
		if c.current.IsHashLocked() {
			if preprepare.Proposal.Hash() == c.current.GetLockedHash() {
				// Broadcast COMMIT and enters Prepared state directly
				c.acceptPreprepare(preprepare)
				c.setState(StatePrepared)
				c.sendCommit()
			} else {
				// Send round change
				c.sendNextRoundChange()
			}
		} else {
			// Either
			//   1. the locked proposal and the received proposal match
			//   2. we have no locked proposal
			c.acceptPreprepare(preprepare)
			c.setState(StatePreprepared)
			c.sendPrepare()
		}
	}

	return nil
}

func (c *core) acceptPreprepare(preprepare *istanbul.Preprepare) {
	c.consensusTimestamp = time.Now()
	c.current.SetPreprepare(preprepare)
}
