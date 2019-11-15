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
	"reflect"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

func (c *core) sendPrepare() {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "sendPrepare")

	sub := c.current.Subject()
	encodedSubject, err := Encode(sub)
	if err != nil {
		logger.Error("Failed to encode", "subject", sub)
		return
	}
	logger.Trace("Sending prepare")
	c.broadcast(&istanbul.Message{
		Code: istanbul.MsgPrepare,
		Msg:  encodedSubject,
	})
}

func (c *core) verifyPreparedCertificate(preparedCertificate istanbul.PreparedCertificate) error {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "verifyPreparedCertificate")

	// Validate the attached proposal
	if _, err := c.backend.Verify(preparedCertificate.Proposal); err != nil {
		return errInvalidPreparedCertificateProposal
	}

	if len(preparedCertificate.PrepareOrCommitMessages) > c.valSet.Size() || len(preparedCertificate.PrepareOrCommitMessages) < c.valSet.MinQuorumSize() {
		return errInvalidPreparedCertificateNumMsgs
	}

	seen := make(map[common.Address]bool)

	var view *istanbul.View
	for _, message := range preparedCertificate.PrepareOrCommitMessages {
		data, err := message.PayloadNoSig()
		if err != nil {
			return err
		}

		// Verify message signed by a validator
		signer, err := c.validateFn(data, message.Signature)
		if err != nil {
			return err
		}

		if signer != message.Address {
			return errInvalidPreparedCertificateMsgSignature
		}

		// Check for duplicate messages
		if seen[signer] {
			return errInvalidPreparedCertificateDuplicate
		}
		seen[signer] = true

		// Check that the message is a PREPARE or COMMIT message
		if message.Code != istanbul.MsgPrepare && message.Code != istanbul.MsgCommit {
			return errInvalidPreparedCertificateMsgCode
		}

		var subject *istanbul.Subject
		if err := message.Decode(&subject); err != nil {
			logger.Error("Failed to decode message in PREPARED certificate", "err", err)
			return err
		}

		// Verify message for the proper sequence.
		if subject.View.Sequence.Cmp(c.currentView().Sequence) != 0 {
			return errInvalidPreparedCertificateMsgView
		}

		// Verify message for the proper proposal.
		if subject.Digest != preparedCertificate.Proposal.Hash() {
			return errInvalidPreparedCertificateDigestMismatch
		}

		// Verify that the view is the same for all of the messages
		if view == nil {
			view = subject.View
		} else {
			if view.Cmp(subject.View) != 0 {
				return errInvalidPreparedCertificateInconsistentViews
			}
		}

		// If COMMIT message, verify valid committed seal.
		if message.Code == istanbul.MsgCommit {
			_, src := c.valSet.GetByAddress(signer)
			err := c.verifyCommittedSeal(subject.Digest, message.CommittedSeal, src)
			if err != nil {
				logger.Error("Commit seal did not contain signature from message signer.", "err", err)
				return err
			}
		}
	}
	return nil
}

func (c *core) handlePrepare(msg *istanbul.Message) error {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "handlePrepare", "tag", "handleMsg")
	// Decode PREPARE message
	var prepare *istanbul.Subject
	err := msg.Decode(&prepare)
	if err != nil {
		return errFailedDecodePrepare
	}

	if err := c.checkMessage(istanbul.MsgPrepare, prepare.View); err != nil {
		return err
	}

	if err := c.verifyPrepare(prepare); err != nil {
		return err
	}

	c.acceptPrepare(msg)
	preparesAndCommits := c.current.GetPrepareOrCommitSize()
	minQuorumSize := c.valSet.MinQuorumSize()
	logger.Trace("Accepted prepare", "Number of prepares or commits", preparesAndCommits)

	// Change to Prepared state if we've received enough PREPARE messages and we are in earlier state
	// before Prepared state.
	// TODO(joshua): Remove state comparisons (or change the cmp function)
	if (preparesAndCommits >= minQuorumSize) && c.state.Cmp(StatePrepared) < 0 {
		if err := c.current.CreateAndSetPreparedCertificate(minQuorumSize); err != nil {
			logger.Error("Failed to create and set preprared certificate", "err", err)
			return err
		}
		logger.Trace("Got quorum prepares or commits", "tag", "stateTransition", "commits", c.current.Commits, "prepares", c.current.Prepares)
		c.setState(StatePrepared)
		c.sendCommit()
	}

	return nil
}

// verifyPrepare verifies if the received PREPARE message is equivalent to our subject
func (c *core) verifyPrepare(prepare *istanbul.Subject) error {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "verifyPrepare")

	sub := c.current.Subject()
	if !reflect.DeepEqual(prepare, sub) {
		logger.Warn("Inconsistent subjects between PREPARE and proposal", "expected", sub, "got", prepare)
		return errInconsistentSubject
	}

	return nil
}

func (c *core) acceptPrepare(msg *istanbul.Message) error {
	logger := c.logger.New("from", msg.Address, "state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "acceptPrepare")

	// Add the PREPARE message to current round state
	if err := c.current.Prepares.Add(msg); err != nil {
		logger.Error("Failed to add PREPARE message to round state", "msg", msg, "err", err)
		return err
	}

	return nil
}
