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

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

func (c *core) sendPrepare() {
	logger := c.newLogger("func", "sendPrepare")

	sub := c.current.Subject()
	encodedSubject, err := Encode(sub)
	if err != nil {
		logger.Error("Failed to encode", "subject", sub)
		return
	}
	logger.Debug("Sending prepare")
	c.broadcast(&istanbul.Message{
		Code: istanbul.MsgPrepare,
		Msg:  encodedSubject,
	})
}

// Verify a prepared certificate and return the view that all of its messages pertain to.
func (c *core) verifyPreparedCertificate(preparedCertificate istanbul.PreparedCertificate) (*istanbul.View, error) {
	logger := c.newLogger("func", "verifyPreparedCertificate", "proposal_number", preparedCertificate.Proposal.Number(), "proposal_hash", preparedCertificate.Proposal.Hash().String())

	// Validate the attached proposal
	if _, err := c.verifyProposal(preparedCertificate.Proposal); err != nil {
		return nil, errInvalidPreparedCertificateProposal
	}

	if len(preparedCertificate.PrepareOrCommitMessages) > c.current.ValidatorSet().Size() || len(preparedCertificate.PrepareOrCommitMessages) < c.current.ValidatorSet().MinQuorumSize() {
		return nil, errInvalidPreparedCertificateNumMsgs
	}

	seen := make(map[common.Address]bool)

	var view *istanbul.View
	for _, message := range preparedCertificate.PrepareOrCommitMessages {
		data, err := message.PayloadNoSig()
		if err != nil {
			return nil, err
		}
		// Verify message signed by a validator
		signer, err := c.validateFn(data, message.Signature)
		if err != nil {
			return nil, err
		}

		if signer != message.Address {
			return nil, errInvalidPreparedCertificateMsgSignature
		}

		// Check for duplicate messages
		if seen[signer] {
			return nil, errInvalidPreparedCertificateDuplicate
		}
		seen[signer] = true

		// Check that the message is a PREPARE or COMMIT message
		if message.Code != istanbul.MsgPrepare && message.Code != istanbul.MsgCommit {
			return nil, errInvalidPreparedCertificateMsgCode
		}

		// Assume prepare but overwrite if commit
		subject := message.Prepare()
		if message.Code == istanbul.MsgCommit {
			commit := message.Commit()
			// Verify the committedSeal
			src := c.current.GetValidatorByAddress(signer)
			err = c.verifyCommittedSeal(commit, src)
			if err != nil {
				logger.Error("Commit seal did not contain signature from message signer.", "err", err)
				return nil, err
			}

			newValSet, err := c.backend.NextBlockValidators(preparedCertificate.Proposal)
			if err != nil {
				return nil, err
			}
			err = c.verifyEpochValidatorSetSeal(commit, preparedCertificate.Proposal.Number().Uint64(), newValSet, src)
			if err != nil {
				logger.Error("Epoch validator set seal seal did not contain signature from message signer.", "err", err)
				return nil, err
			}

			subject = commit.Subject
		}

		msgLogger := logger.New("msg_round", subject.View.Round, "msg_seq", subject.View.Sequence, "msg_digest", subject.Digest.String())
		msgLogger.Trace("Decoded message in prepared certificate", "code", message.Code)

		// Verify message for the proper sequence.
		if subject.View.Sequence.Cmp(c.current.Sequence()) != 0 {
			return nil, errInvalidPreparedCertificateMsgView
		}

		// Verify message for the proper proposal.
		if subject.Digest != preparedCertificate.Proposal.Hash() {
			return nil, errInvalidPreparedCertificateDigestMismatch
		}

		// Verify that the view is the same for all of the messages
		if view == nil {
			view = subject.View
		} else {
			if view.Cmp(subject.View) != 0 {
				return nil, errInvalidPreparedCertificateInconsistentViews
			}
		}
	}
	return view, nil
}

// Extract the view from a PreparedCertificate that has already been verified.
func (c *core) getViewFromVerifiedPreparedCertificate(preparedCertificate istanbul.PreparedCertificate) (*istanbul.View, error) {
	if len(preparedCertificate.PrepareOrCommitMessages) < c.current.ValidatorSet().MinQuorumSize() {
		return nil, errInvalidPreparedCertificateNumMsgs
	}

	message := preparedCertificate.PrepareOrCommitMessages[0]

	// Assume prepare but overwrite if commit
	subject := message.Prepare()
	if message.Code == istanbul.MsgCommit {
		subject = message.Commit().Subject
	}
	return subject.View, nil
}

func (c *core) handlePrepare(msg *istanbul.Message) error {
	// Decode PREPARE message
	prepare := msg.Prepare()
	logger := c.newLogger("func", "handlePrepare", "tag", "handleMsg", "msg_round", prepare.View.Round, "msg_seq", prepare.View.Sequence, "msg_digest", prepare.Digest.String())

	if err := c.checkMessage(istanbul.MsgPrepare, prepare.View); err != nil {
		return err
	}

	if err := c.verifyPrepare(prepare); err != nil {
		return err
	}

	// Add the PREPARE message to current round state
	if err := c.current.AddPrepare(msg); err != nil {
		logger.Error("Failed to add PREPARE message to round state", "err", err)
		return err
	}

	minQuorumSize := c.current.ValidatorSet().MinQuorumSize()
	psize := c.current.Prepares().Size()
	logger = logger.New("prepares", psize)
	logger.Trace("Accepted prepare")

	// Change to Prepared state if we've received enough PREPARE messages and we are in earlier state
	// before Prepared state.
	// TODO(joshua): Remove state comparisons (or change the cmp function)
	if (psize >= minQuorumSize) && c.current.State().Cmp(StatePrepared) < 0 {

		err := c.current.TransitionToPrepared(minQuorumSize)
		if err != nil {
			logger.Error("Failed to create and set preprared certificate", "err", err)
			return err
		}
		logger.Trace("Got quorum prepares or commits", "tag", "stateTransition")

		// Process Backlog Messages
		c.backlog.updateState(c.current.View(), c.current.State())

		c.sendCommit()
	}

	return nil
}

// verifyPrepare verifies if the received PREPARE message is equivalent to our subject
func (c *core) verifyPrepare(prepare *istanbul.Subject) error {
	logger := c.newLogger("func", "verifyPrepare", "prepare_round", prepare.View.Round, "prepare_seq", prepare.View.Sequence, "prepare_digest", prepare.Digest.String())

	sub := c.current.Subject()
	if !reflect.DeepEqual(prepare, sub) {
		logger.Warn("Inconsistent subjects between PREPARE and proposal", "expected", sub, "got", prepare)
		return errInconsistentSubject
	}

	return nil
}
