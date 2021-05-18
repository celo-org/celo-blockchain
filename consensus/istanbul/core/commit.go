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
	"math/big"
	"reflect"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
)

// maxValidators represents the maximum number of validators the SNARK circuit supports
// The prover code will then pad any proofs to this maximum to ensure consistent proof structure
// TODO: Eventually make this governable
const maxValidators = uint32(150)

func (c *core) sendCommit() {
	logger := c.newLogger("func", "sendCommit")
	logger.Trace("Sending commit")
	sub := c.current.Subject()
	c.broadcastCommit(sub)
}

func (c *core) broadcastCommit(sub *istanbul.Subject) {
	logger := c.newLogger("func", "broadcastCommit")

	committedSeal, err := NewCommitSeal(sub.Digest, sub.View.Round).Sign(c.backend.SignBLS)
	if err != nil {
		logger.Error("Failed to commit seal", "err", err)
		return
	}

	currentBlockNumber := c.current.Proposal().Number().Uint64()
	var epochValidatorSetSeal []byte
	if istanbul.IsLastBlockOfEpoch(currentBlockNumber, c.config.Epoch) {
		newValSet, err := c.backend.NextBlockValidators(c.current.Proposal())
		if err != nil {
			logger.Error("Failed to get next block's validators", "err", err)
			return
		}

		epochSeal, err := c.generateEpochValidatorSetData(currentBlockNumber, uint8(sub.View.Round.Uint64()), sub.Digest, newValSet)
		if err != nil {
			logger.Error("Failed to create epoch validator set data", "err", err)
			return
		}
		epochValidatorSetSeal, err = epochSeal.Sign(c.backend.SignBLS)
		if err != nil {
			logger.Error("Failed to sign epoch validator set seal", "err", err)
			return
		}
	}
	istMsg := istanbul.NewMessage(&istanbul.CommittedSubject{
		Subject:               sub,
		CommittedSeal:         committedSeal,
		EpochValidatorSetSeal: epochValidatorSetSeal,
	}, c.address)
	c.broadcast(istMsg)
}

func (c *core) handleCommit(msg *istanbul.Message) error {
	commit := msg.Commit()
	err := c.checkMessage(istanbul.MsgCommit, commit.Subject.View)
	if err == errOldMessage {
		// Discard messages from previous views, unless they are commits from the previous sequence,
		// with the same round as what we wound up finalizing, as we would be able to include those
		// to create the ParentAggregatedSeal for our next proposal.
		lastSubject, err := c.backend.LastSubject()
		if err != nil {
			return err
		} else if commit.Subject.View.Cmp(lastSubject.View) != 0 {
			return errOldMessage
		} else if lastSubject.View.Sequence.Cmp(common.Big0) == 0 {
			// Don't handle commits for the genesis block, will cause underflows
			return errOldMessage
		}
		return c.handleCheckedCommitForPreviousSequence(msg, commit)
	} else if err != nil {
		return err
	}

	return c.handleCheckedCommitForCurrentSequence(msg, commit)
}

// handleCheckedCommitForPreviousSequence adds messages for the previous
// sequence to the parent commit set.  If the subject digest of msg does not
// match that of the previous block or it was not sent by one of the previous
// block's validators, an error is returned.
//
// The parent commit set is maintained for the sole purpose of tracking uptime,
// allowing commits that did not arrive in time to be part of their intended
// block to be collected during the subsequent block and to count towards the
// uptime of the sender.
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

	// ensure that the commit is for the current proposal
	if err := c.verifyCommit(commit); err != nil {
		return err
	}

	num := commit.Subject.View.Sequence.Uint64()
	if num > 0 && istanbul.IsLastBlockOfEpoch(num, c.config.Epoch) {
		newValSet, err := c.backend.NextBlockValidators(c.current.Proposal())
		if err != nil {
			return err
		}

		// Verifies the individual seal for this commit message for the epoch
		if err := c.verifyEpochValidatorSetSeal(commit, num, newValSet, validator); err != nil {
			return errInvalidEpochValidatorSetSeal
		}
	}

	// Add the COMMIT message to current round state
	if err := c.current.AddCommit(msg); err != nil {
		logger.Error("Failed to record commit message", "m", msg, "err", err)
		return err
	}

	commits := c.current.Commits()
	numberOfCommits := commits.Size()
	minQuorumSize := c.current.ValidatorSet().MinQuorumSize()
	logger.Trace("Accepted commit for current sequence", "Number of commits", numberOfCommits)

	// Commit the proposal once we have enough COMMIT messages and we are not in the Committed state.
	//
	// TODO(joshua): Remove state comparisons (or change the cmp function)
	if numberOfCommits >= minQuorumSize && c.current.State().Cmp(StateCommitted) < 0 {
		round := c.current.Round()
		proposal := c.current.Proposal()
		validators := c.current.ValidatorSet()
		// TODO understand how proposal can be nil.
		if proposal == nil {
			return nil
		}
		// Generate aggregate seal
		seal := NewCommitSeal(proposal.Hash(), round)
		sig, bitmap, err := GenerateValidAggregateSignature(logger, seal, commits, validators, extractCommitSignature)
		if err != nil {
			// If there was an error then sit this round out, we can't continue.
			nextRound := new(big.Int).Add(round, common.Big1)
			logger.Error("Error on commit, waiting for desired round", "reason", "failed to generate valid aggregate commit seal", "err", err, "desired_round", nextRound)
			c.waitForDesiredRound(nextRound)
			return nil
		}
		if sig == nil {
			// Some bad seals were encountered, wait for more commits.
			return nil
		}
		aggregatedSeal := types.IstanbulAggregatedSeal{
			Bitmap:    bitmap,
			Signature: sig,
			Round:     round,
		}

		// Set the epoch aggregate seal if this is the last block of the epoch
		aggregatedEpochValidatorSetSeal := types.IstanbulEpochValidatorSetSeal{}
		if istanbul.IsLastBlockOfEpoch(proposal.Number().Uint64(), c.config.Epoch) {

			seal, err := c.generateEpochValidatorSetData(num, uint8(round.Uint64()), proposal.Hash(), validators)
			if err != nil {
				nextRound := new(big.Int).Add(c.current.Round(), common.Big1)
				logger.Error("Error on commit, waiting for desired round", "reason", "failed to build epoch seal data", "err", err, "desired_round", nextRound)
				c.waitForDesiredRound(nextRound)
				return nil
			}

			sig, bitmap, err := GenerateValidAggregateSignature(logger, seal, commits, validators, extractEpochSignature)
			if err != nil {
				nextRound := new(big.Int).Add(c.current.Round(), common.Big1)
				logger.Error("Error on commit, waiting for desired round", "reason", "failed to generate valid aggregate epoch seal", "err", err, "desired_round", nextRound)
				c.waitForDesiredRound(nextRound)
				return nil
			}
			if sig == nil {
				// Some bad seals were encountered, wait for more commits.
				return nil
			}

			aggregatedEpochValidatorSetSeal.Bitmap = bitmap
			aggregatedEpochValidatorSetSeal.Signature = sig
		}

		logger.Trace("Got a quorum of commits", "tag", "stateTransition", "commits", commits)
		err = c.commit(aggregatedSeal, aggregatedEpochValidatorSetSeal)
		if err != nil {
			logger.Error("Failed to commit()", "err", err)
			return err
		}
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
