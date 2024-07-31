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
	"errors"
	"reflect"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	celoCore "github.com/celo-org/celo-blockchain/core"
)

func (c *core) sendPrepare() {
	logger := c.newLogger("func", "sendPrepare")
	logger.Debug("Sending prepare")
	c.broadcast(istanbul.NewPrepareMessage(c.current.Subject(), c.address))
}

func (c *core) verifySignedPrepareOrCommitMessage(message istanbul.Message, seen map[common.Address]bool) (*common.Address, error) {
	// Verify message signed by a validator
	if err := istanbul.CheckSignedBy(&message, message.Signature,
		message.Address, errInvalidPreparedCertificateMsgSignature, c.validateFn); err != nil {
		return nil, err
	}
	// Check for duplicate messages
	if seen[message.Address] {
		return nil, errInvalidPreparedCertificateDuplicate
	}
	seen[message.Address] = true

	// Check that the message is a PREPARE or COMMIT message
	if message.Code != istanbul.MsgPrepare && message.Code != istanbul.MsgCommit {
		return nil, errInvalidPreparedCertificateMsgCode
	}
	return &message.Address, nil
}

func (c *core) verifyProposalPrepareOrCommitMessage(proposal istanbul.Proposal, message istanbul.Message, seen map[common.Address]bool) (*istanbul.View, error) {
	logger := c.newLogger("func", "verifyProposalPrepareOrCommitMessage", "proposal_number", proposal.Number(), "proposal_hash", proposal.Hash().String())
	signer, err := c.verifySignedPrepareOrCommitMessage(message, seen)
	if err != nil {
		return nil, err
	}
	// Assume prepare but overwrite if commit
	subject := message.Prepare()
	if message.Code == istanbul.MsgCommit {
		commit := message.Commit()
		// Verify the committedSeal
		src := c.current.GetValidatorByAddress(*signer)
		err = c.verifyCommittedSeal(commit, src)
		if err != nil {
			logger.Error("Commit seal did not contain signature from message signer.", "err", err)
			return nil, err
		}

		newValSet, err := c.backend.NextBlockValidators(proposal)
		if err != nil {
			return nil, err
		}
		err = c.verifyEpochValidatorSetSeal(commit, proposal.Number().Uint64(), newValSet, src)
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
	if subject.Digest != proposal.Hash() {
		return nil, errInvalidPreparedCertificateDigestMismatch
	}

	return subject.View, nil
}

func (c *core) verifyPCV2WithProposal(pcV2 istanbul.PreparedCertificateV2, proposal istanbul.Proposal) (*istanbul.View, error) {
	return c.verifyProposalAndPCMessages(proposal, pcV2.PrepareOrCommitMessages)
}

// Verify a prepared certificate and return the view that all of its messages pertain to.
func (c *core) verifyPreparedCertificate(preparedCertificate istanbul.PreparedCertificate) (*istanbul.View, error) {
	return c.verifyProposalAndPCMessages(preparedCertificate.Proposal, preparedCertificate.PrepareOrCommitMessages)
}

func (c *core) verifyProposalAndPCMessages(proposal istanbul.Proposal, pCMessages []istanbul.Message) (*istanbul.View, error) {
	// Validate the attached proposal
	if _, err := c.verifyProposal(proposal); err != nil {
		if err == celoCore.ErrPostL2BlockNumber {
			return nil, err
		}
		return nil, errInvalidPreparedCertificateProposal
	}

	if len(pCMessages) > c.current.ValidatorSet().Size() || len(pCMessages) < c.current.ValidatorSet().MinQuorumSize() {
		return nil, errInvalidPreparedCertificateNumMsgs
	}

	seen := make(map[common.Address]bool)

	var view *istanbul.View
	for _, message := range pCMessages {
		messageView, err := c.verifyProposalPrepareOrCommitMessage(proposal, message, seen)
		if err != nil {
			return nil, err
		}
		// Verify that the view is the same for all of the messages
		if view == nil {
			view = messageView
		} else {
			if view.Cmp(messageView) != 0 {
				return nil, errInvalidPreparedCertificateInconsistentViews
			}
		}
	}
	return view, nil
}

// Extract the view from a PreparedCertificate that has already been verified.
func (c *core) getViewFromVerifiedPreparedCertificate(preparedCertificate istanbul.PreparedCertificate) (*istanbul.View, error) {
	return c.getViewFromVerifiedPreparedCertificateMessages(preparedCertificate.PrepareOrCommitMessages)
}

func (c *core) getViewFromVerifiedPreparedCertificateV2(preparedCertificate istanbul.PreparedCertificateV2) (*istanbul.View, error) {
	return c.getViewFromVerifiedPreparedCertificateMessages(preparedCertificate.PrepareOrCommitMessages)
}

func (c *core) getViewFromVerifiedPreparedCertificateMessages(messages []istanbul.Message) (*istanbul.View, error) {
	if len(messages) < c.current.ValidatorSet().MinQuorumSize() {
		return nil, errInvalidPreparedCertificateNumMsgs
	}

	message := messages[0]

	// Assume prepare but overwrite if commit
	subject := message.Prepare()
	if message.Code == istanbul.MsgCommit {
		subject = message.Commit().Subject
	}
	return subject.View, nil
}

func (c *core) handlePrepare(msg *istanbul.Message) error {
	defer c.handlePrepareTimer.UpdateSince(time.Now())
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

	preparesAndCommits := c.current.GetPrepareOrCommitSize()
	minQuorumSize := c.current.ValidatorSet().MinQuorumSize()
	logger = logger.New("prepares_and_commits", preparesAndCommits, "commits", c.current.Commits().Size(), "prepares", c.current.Prepares().Size())
	logger.Trace("Accepted prepare")

	// Change to Prepared state if we've received enough PREPARE messages and we are in earlier state
	// before Prepared state.
	// TODO(joshua): Remove state comparisons (or change the cmp function)
	if (preparesAndCommits >= minQuorumSize) && c.current.State().Cmp(StatePrepared) < 0 {

		err := c.current.TransitionToPrepared(minQuorumSize)
		if err != nil {
			logger.Error("Failed to create and set preprared certificate", "err", err)
			return err
		}
		logger.Trace("Got quorum prepares or commits", "tag", "stateTransition")
		// Update metrics.
		if !c.consensusTimestamp.IsZero() {
			c.consensusPrepareTimeGauge.Update(time.Since(c.consensusTimestamp).Nanoseconds())
		}

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

// GossipPrepares gossips to other validators all the prepares received in the current round.
func (c *core) GossipPrepares() error {
	logger := c.newLogger("func", "gossipPrepares")
	st := c.current.State()
	if st != StatePreprepared && st != StatePrepared && st != StateCommitted {
		return errors.New("Cant gossip prepares if not in preprepared, prepared, or committed state")
	}
	prepares := c.current.Prepares().Values()
	logger.Debug("Gossipping prepares", "len", len(prepares))
	for _, prepare := range prepares {
		c.gossip(prepare)
		// let the bandwidth breathe a little
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}
