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

	"github.com/ethereum/go-ethereum/consensus/istanbul"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
)

func (c *core) sendCommit() {
	logger := c.newLogger("func", "sendCommit")
	logger.Trace("Sending commit")
	sub := c.current.Subject()
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
	logger := c.newLogger("func", "broadcastCommit")

	committedSeal, err := c.generateCommittedSeal(sub)
	if err != nil {
		logger.Error("Failed to commit seal", "err", err)
		return
	}

	committedSub := &istanbul.CommittedSubject{
		Subject:       sub,
		CommittedSeal: committedSeal,
	}
	encodedCommittedSubject, err := Encode(committedSub)
	if err != nil {
		logger.Error("Failed to encode committedSubject", committedSub)
	}

	istMsg := istanbul.Message{
		Code: istanbul.MsgCommit,
		Msg:  encodedCommittedSubject,
	}
	c.broadcast(&istMsg)
}

func (c *core) handleCommit(msg *istanbul.Message) error {
	// Decode COMMIT message
	var commit *istanbul.CommittedSubject
	err := msg.Decode(&commit)
	if err != nil {
		return errFailedDecodeCommit
	}

	err = c.checkMessage(istanbul.MsgCommit, commit.Subject.View)

	if err == errOldMessage {
		// Discard messages from previous views, unless they are commits from the previous sequence,
		// with the same round as what we wound up finalizing, as we would be able to include those
		// to create the ParentAggregatedSeal for our next proposal.
		lastSubject, err := c.backend.LastSubject()
		if err != nil {
			return err
		} else if commit.Subject.View.Cmp(lastSubject.View) != 0 {
			return errOldMessage
		}
		return c.handleCheckedCommitForPreviousSequence(msg, commit)
	} else if err != nil {
		return err
	}

	return c.handleCheckedCommitForCurrentSequence(msg, commit)
}

func (c *core) handleCheckedCommitForPreviousSequence(msg *istanbul.Message, commit *istanbul.CommittedSubject) error {
	logger := c.newLogger("func", "handleCheckedCommitForPreviousSequence", "tag", "handleMsg", "msg_view", commit.Subject.View)
	headBlock := c.backend.GetCurrentHeadBlock()
	// Retrieve the validator set for the previous proposal (which should
	// match the one broadcast)
	parentValset := c.backend.ParentBlockValidators(headBlock)
	_, validator := parentValset.GetByAddress(msg.Address)
	if validator == nil {
		return errInvalidValidatorAddress
	}
	if err := c.verifyCommittedSeal(commit, validator); err != nil {
		return errInvalidCommittedSeal
	}
	// Ensure that the commit's digest (ie the received proposal's hash) matches the head block's hash
	if headBlock.Number().Uint64() > 0 && commit.Subject.Digest != headBlock.Hash() {
		logger.Debug("Received a commit message for the previous sequence with an unexpected hash", "expected", headBlock.Hash().String(), "received", commit.Subject.Digest.String())
		return errInconsistentSubject
	}

	// Add the ParentCommit to current round state
	if err := c.current.AddParentCommit(msg); err != nil {
		logger.Error("Failed to record parent seal", "m", msg, "err", err)
		return err
	}
	return nil
}

func (c *core) handleCheckedCommitForCurrentSequence(msg *istanbul.Message, commit *istanbul.CommittedSubject) error {
	logger := c.newLogger("func", "handleCheckedCommitForCurrentSequence", "tag", "handleMsg")
	validator := c.current.GetValidatorByAddress(msg.Address)
	if validator == nil {
		return errInvalidValidatorAddress
	}

	if err := c.verifyCommittedSeal(commit, validator); err != nil {
		return errInvalidCommittedSeal
	}

	// ensure that the commit is in the current proposal
	if err := c.verifyCommit(commit); err != nil {
		return err
	}

	// Add the COMMIT message to current round state
	if err := c.current.AddCommit(msg); err != nil {
		logger.Error("Failed to record commit message", "m", msg, "err", err)
		return err
	}
	numberOfCommits := c.current.Commits().Size()
	minQuorumSize := c.current.ValidatorSet().MinQuorumSize()
	logger.Trace("Accepted commit for current sequence", "Number of commits", numberOfCommits)

	// Commit the proposal once we have enough COMMIT messages and we are not in the Committed state.
	//
	// If we already have a proposal, we may have chance to speed up the consensus process
	// by committing the proposal without PREPARE messages.
	// TODO(joshua): Remove state comparisons (or change the cmp function)
	if numberOfCommits >= minQuorumSize && c.current.State().Cmp(StateCommitted) < 0 {
		logger.Trace("Got a quorum of commits", "tag", "stateTransition", "commits", c.current.Commits)
		err := c.commit()
		if err != nil {
			logger.Error("Failed to commit()", "err", err)
			return err
		}

	} else if c.current.GetPrepareOrCommitSize() >= minQuorumSize && c.current.State().Cmp(StatePrepared) < 0 {
		err := c.current.TransitionToPrepared(minQuorumSize)
		if err != nil {
			logger.Error("Failed to create and set preprared certificate", "err", err)
			return err
		}
		// Process Backlog Messages
		c.backlog.updateState(c.current.View(), c.current.State())

		logger.Trace("Got quorum prepares or commits", "tag", "stateTransition", "commits", c.current.Commits, "prepares", c.current.Prepares)
		c.sendCommit()
	}
	return nil

}

// verifyCommit verifies if the received COMMIT message is equivalent to our subject
func (c *core) verifyCommit(commit *istanbul.CommittedSubject) error {
	logger := c.newLogger("func", "verifyCommit")

	sub := c.current.Subject()
	if !reflect.DeepEqual(commit.Subject, sub) {
		logger.Warn("Inconsistent subjects between commit and proposal", "expected", sub, "got", commit)
		return errInconsistentSubject
	}

	return nil
}

// verifyCommittedSeal verifies the commit seal in the received COMMIT message
func (c *core) verifyCommittedSeal(comSub *istanbul.CommittedSubject, src istanbul.Validator) error {
	seal := PrepareCommittedSeal(comSub.Subject.Digest, comSub.Subject.View.Round)
	return blscrypto.VerifySignature(src.BLSPublicKey(), seal, []byte{}, comSub.CommittedSeal, false)
}
