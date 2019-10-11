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
	"github.com/ethereum/go-ethereum/crypto/bls"
)

func (c *core) sendCommit() {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "sendCommit")
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

func (c *core) generateCommittedSeal(digest common.Hash) ([]byte, error) {
	seal := PrepareCommittedSeal(digest)
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

	committedSeal, err := c.generateCommittedSeal(sub.Digest)
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
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "handleCommit", "tag", "handleMsg")
	// Decode COMMIT message
	var commit *istanbul.Subject
	err := msg.Decode(&commit)
	if err != nil {
		return errFailedDecodeCommit
	}

	if err := c.checkMessage(istanbul.MsgCommit, commit.View); err != nil {
		return err
	}

	if err := c.verifyCommit(commit); err != nil {
		return err
	}

	_, validator := c.valSet.GetByAddress(msg.Address)
	if validator == nil {
		return errInvalidValidatorAddress
	}

	if err := c.verifyCommittedSeal(commit.Digest, msg.CommittedSeal, validator); err != nil {
		return errInvalidCommittedSeal
	}

	c.acceptCommit(msg)
	numberOfCommits := c.current.Commits.Size()
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
		logger.Trace("Got enough prepares and commits to generate a PreparedCertificate")
		if err := c.current.CreateAndSetPreparedCertificate(minQuorumSize); err != nil {
			logger.Error("Failed to create and set preprared certificate", "err", err)
			return err
		}
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
func (c *core) verifyCommittedSeal(digest common.Hash, committedSeal []byte, src istanbul.Validator) error {
	seal := PrepareCommittedSeal(digest)
	return blscrypto.VerifySignature(src.BLSPublicKey(), seal, []byte{}, committedSeal, false)
}

func (c *core) acceptCommit(msg *istanbul.Message) error {
	logger := c.logger.New("from", msg.Address, "state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "acceptCommit")

	// Add the COMMIT message to current round state
	if err := c.current.Commits.Add(msg); err != nil {
		logger.Error("Failed to record commit message", "msg", msg, "err", err)
		return err
	}

	return nil
}
