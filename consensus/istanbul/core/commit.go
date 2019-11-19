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
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
)

func (c *core) sendCommit() {
	logger := c.newLogger("func", "sendCommit")
	logger.Trace("Sending commit")
	sub := c.current.Subject()
	c.broadcastCommit(sub)
}

func (c *core) sendCommitForOldBlock(view *istanbul.View, digest common.Hash) {
	sub := &istanbul.Subject{
		View:   view,
		Digest: digest,
	}
	c.broadcastCommit(sub)
}

func (c *core) generateCommittedSeal(sub *istanbul.Subject) ([]byte, error) {
	seal := PrepareCommittedSeal(sub.Digest, sub.View.Round)
	committedSeal, err := c.backend.SignBlockHeader(seal)
	if err != nil {
		return nil, err
	}
	return committedSeal, nil
}

func (c *core) broadcastCommit(sub *istanbul.Subject) {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence())

	encodedSubject, err := Encode(sub)
	if err != nil {
		logger.Error("Failed to encode", "subject", sub)
		return
	}

	committedSeal, err := c.generateCommittedSeal(sub)
	if err != nil {
		logger.Error("Failed to commit seal", "err", err)
		return
	}

	istMsg := istanbul.Message{
		Code:          istanbul.MsgCommit,
		Msg:           encodedSubject,
		CommittedSeal: committedSeal,
	}
	c.broadcast(&istMsg)
}

func (c *core) handleCommit(msg *istanbul.Message) error {
	// Decode COMMIT message
	var commit *istanbul.Subject
	err := msg.Decode(&commit)
	if err != nil {
		return errFailedDecodeCommit
	}

	if err := c.checkMessage(istanbul.MsgCommit, commit.View); err != nil {
		return err
	}

	// Valid commit messages may be for the current, or previous sequence. We compare against our
	// current view to find out which.
	if commit.View.Cmp(c.current.View()) == 0 {
		return c.handleCheckedCommitForCurrentSequence(msg, commit)
	} else {
		return c.handleCheckedCommitForPreviousSequence(msg, commit)
	}
}

func (c *core) handleCheckedCommitForPreviousSequence(msg *istanbul.Message, commit *istanbul.Subject) error {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "handleCheckedCommitForPreviousSequence", "tag", "handleMsg")
	lastProposal, _ := c.backend.LastProposal()
	// Retrieve the validator set for the previous proposal (which should
	// match the one broadcast)
	parentValset := c.backend.ParentValidators(lastProposal)
	_, validator := parentValset.GetByAddress(msg.Address)
	if validator == nil {
		return errInvalidValidatorAddress
	}
	if err := c.verifyCommittedSeal(commit, msg.CommittedSeal, validator); err != nil {
		return errInvalidCommittedSeal
	}
	// Ensure that the commit's digest (ie the received proposal's hash) matches the saved last proposal's hash
	if lastProposal.Number().Uint64() > 0 && commit.Digest != lastProposal.Hash() {
		logger.Debug("Received a commit message for the previous sequence with an unexpected hash", "expected", lastProposal.Hash().String(), "received", commit.Digest.String())
		return errInconsistentSubject
	}
	return c.acceptParentCommit(msg, commit.View)
}

func (c *core) handleCheckedCommitForCurrentSequence(msg *istanbul.Message, commit *istanbul.Subject) error {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "handleCheckedCommitForCurrentSequence", "tag", "handleMsg")
	_, validator := c.valSet.GetByAddress(msg.Address)
	if validator == nil {
		return errInvalidValidatorAddress
	}

	if err := c.verifyCommittedSeal(commit, msg.CommittedSeal, validator); err != nil {
		return errInvalidCommittedSeal
	}

	// ensure that the commit is in the current proposal
	if err := c.verifyCommit(commit); err != nil {
		return err
	}

	c.acceptCommit(msg)
	numberOfCommits := c.current.Commits().Size()
	minQuorumSize := c.valSet.MinQuorumSize()
	logger.Trace("Accepted commit", "Number of commits", numberOfCommits)

	// Commit the proposal once we have enough COMMIT messages and we are not in the Committed state.
	//
	// If we already have a proposal, we may have chance to speed up the consensus process
	// by committing the proposal without PREPARE messages.
	// TODO(joshua): Remove state comparisons (or change the cmp function)
	if numberOfCommits >= minQuorumSize && c.state.Cmp(StateCommitted) < 0 {
		logger.Trace("Got a quorum of commits", "tag", "stateTransition", "commits", c.current.Commits)
		c.commit()
	} else if c.current.GetPrepareOrCommitSize() >= minQuorumSize && c.state.Cmp(StatePrepared) < 0 {
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

// verifyCommit verifies if the received COMMIT message is equivalent to our subject
func (c *core) verifyCommit(commit *istanbul.Subject) error {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "verifyCommit")

	sub := c.current.Subject()
	if !reflect.DeepEqual(commit, sub) {
		logger.Warn("Inconsistent subjects between commit and proposal", "expected", sub, "got", commit)
		return errInconsistentSubject
	}

	return nil
}

// verifyCommittedSeal verifies the commit seal in the received COMMIT message
func (c *core) verifyCommittedSeal(sub *istanbul.Subject, committedSeal []byte, src istanbul.Validator) error {
	seal := PrepareCommittedSeal(sub.Digest, sub.View.Round)
	return blscrypto.VerifySignature(src.BLSPublicKey(), seal, []byte{}, committedSeal, false)
}

func (c *core) acceptCommit(msg *istanbul.Message) error {
	logger := c.logger.New("from", msg.Address, "state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "acceptCommit")

	// Add the COMMIT message to current round state
	if err := c.current.Commits().Add(msg); err != nil {
		logger.Error("Failed to record commit message", "msg", msg, "err", err)
		return err
	}

	return nil
}

func (c *core) acceptParentCommit(msg *istanbul.Message, view *istanbul.View) error {
	logger := c.logger.New("from", msg.Address, "state", c.state, "parent_round", view.Round, "parent_seq", view.Sequence, "func", "acceptParentCommit")

	// Add the ParentCommit to current round state
	if err := c.current.ParentCommits().Add(msg); err != nil {
		logger.Error("Failed to record parent seal", "msg", msg, "err", err)
		return err
	}

	return nil
}
