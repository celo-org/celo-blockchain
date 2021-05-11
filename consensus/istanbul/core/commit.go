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
	"fmt"
	"math/big"
	"reflect"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
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

func (c *core) generateCommittedSeal(sub *istanbul.Subject) (blscrypto.SerializedSignature, error) {
	seal := PrepareCommittedSeal(sub.Digest, sub.View.Round)
	committedSeal, err := c.backend.SignBLS(seal, []byte{}, false, false)
	if err != nil {
		return blscrypto.SerializedSignature{}, err
	}
	return committedSeal, nil
}

// Generates serialized epoch data for use in the Plumo SNARK circuit.
// Block number and hash may be information for a pending block.
func (c *core) generateEpochValidatorSetData(blockNumber uint64, round uint8, blockHash common.Hash, newValSet istanbul.ValidatorSet) ([]byte, []byte, bool, error) {
	if !istanbul.IsLastBlockOfEpoch(blockNumber, c.config.Epoch) {
		return nil, nil, false, errNotLastBlockInEpoch
	}

	// Serialize the public keys for the validators in the validator set.
	blsPubKeys := []blscrypto.SerializedPublicKey{}
	for _, v := range newValSet.List() {
		blsPubKeys = append(blsPubKeys, v.BLSPublicKey())
	}

	maxNonSigners := uint32(newValSet.Size() - newValSet.MinQuorumSize())

	// Before the Donut fork, use the snark data encoding with epoch entropy.
	if !c.backend.ChainConfig().IsDonut(big.NewInt(int64(blockNumber))) {
		message, extraData, err := blscrypto.EncodeEpochSnarkData(
			blsPubKeys, maxNonSigners,
			uint16(istanbul.GetEpochNumber(blockNumber, c.config.Epoch)),
		)
		// This is before the Donut hardfork, so signify this doesn't use CIP22.
		return message, extraData, false, err
	}

	// Retrieve the block hash for the last block of the previous epoch.
	parentEpochBlockHash := c.backend.HashForBlock(blockNumber - c.config.Epoch)
	if blockNumber > 0 && parentEpochBlockHash == (common.Hash{}) {
		return nil, nil, false, errors.New("unknown block")
	}

	maxNonSigners = maxValidators - uint32(newValSet.MinQuorumSize())
	message, extraData, err := blscrypto.EncodeEpochSnarkDataCIP22(
		blsPubKeys, maxNonSigners, maxValidators,
		uint16(istanbul.GetEpochNumber(blockNumber, c.config.Epoch)),
		round,
		blscrypto.EpochEntropyFromHash(blockHash),
		blscrypto.EpochEntropyFromHash(parentEpochBlockHash),
	)
	// This is after the Donut hardfork, so signify this uses CIP22.
	return message, extraData, true, err
}

func (c *core) broadcastCommit(sub *istanbul.Subject) {
	logger := c.newLogger("func", "broadcastCommit")

	committedSeal, err := c.generateCommittedSeal(sub)
	if err != nil {
		logger.Error("Failed to commit seal", "err", err)
		return
	}

	currentBlockNumber := c.current.Proposal().Number().Uint64()
	newValSet, err := c.backend.NextBlockValidators(c.current.Proposal())
	if err != nil {
		logger.Error("Failed to get next block's validators", "err", err)
		return
	}
	epochValidatorSetData, epochValidatorSetExtraData, cip22, err := c.generateEpochValidatorSetData(currentBlockNumber, uint8(sub.View.Round.Uint64()), sub.Digest, newValSet)
	if err != nil && err != errNotLastBlockInEpoch {
		logger.Error("Failed to create epoch validator set data", "err", err)
		return
	}
	var epochValidatorSetSeal blscrypto.SerializedSignature
	if err == nil {
		epochValidatorSetSeal, err = c.backend.SignBLS(epochValidatorSetData, epochValidatorSetExtraData, true, cip22)
		if err != nil {
			logger.Error("Failed to sign epoch validator set seal", "err", err)
			return
		}
	}
	istMsg := istanbul.NewMessage(&istanbul.CommittedSubject{
		Subject:               sub,
		CommittedSeal:         committedSeal[:],
		EpochValidatorSetSeal: epochValidatorSetSeal[:],
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
// sequence to the parent commit set.
// If the subject digest of msg does not match that of the previous block or it
// was not sent by one of the previous block's validators, an error is returned.
// If this is the last block of the epoch then the epoch seal will also be validated.
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
	if headBlock.Number().Uint64() > 0 {
		// Verifies the individual seal for this commit message for the epoch
		// block, no-op unless this is the last block of an epoch.
		if err := c.verifyEpochValidatorSetSeal(commit, headBlock.Number().Uint64(), c.current.ValidatorSet(), validator); err != nil {
			return errInvalidEpochValidatorSetSeal
		}
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

	newValSet, err := c.backend.NextBlockValidators(c.current.Proposal())
	if err != nil {
		return err
	}

	// Verifies the individual seal for this commit message for the epoch
	// block, no-op unless this is the last block of an epoch.
	if err := c.verifyEpochValidatorSetSeal(commit, c.current.Proposal().Number().Uint64(), newValSet, validator); err != nil {
		return errInvalidEpochValidatorSetSeal
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
		proposal := c.current.Proposal()
		// TODO understand how proposal can be nil.
		if proposal == nil {
			return nil
		}

		// Generate aggregate seal
		aggregatedSeal, err := c.generateAggregateCommittedSeal()
		if err != nil {
			logger.Warn("Initial verificaction of aggregate commit signature failed", "err", err)
			// Remove any bad committed seals and try again if sufficient commits remain
			c.removeInvalidCommittedSeals()
			if c.current.Commits().Size() < c.current.ValidatorSet().MinQuorumSize() {
				// Wait for more commits
				return nil
			}
			aggregatedSeal, err = c.generateAggregateCommittedSeal()
		}
		// If there is still an error then sit this round out, we can't continue.
		if err != nil {
			nextRound := new(big.Int).Add(c.current.Round(), common.Big1)
			logger.Warn("Error on commit, waiting for desired round", "reason", "failed to aggregate commit seals", "err", err, "desired_round", nextRound)
			c.waitForDesiredRound(nextRound)
			return nil
		}

		// Set the epoch aggregate seal if this is the last block of the epoch
		aggregatedEpochValidatorSetSeal := types.IstanbulEpochValidatorSetSeal{}
		if istanbul.IsLastBlockOfEpoch(proposal.Number().Uint64(), c.config.Epoch) {
			epochBitmap, epochAggregate, err := AggregateSeals(
				c.current.Commits(),
				func(c *istanbul.CommittedSubject) []byte { return c.EpochValidatorSetSeal },
			)
			if err != nil {
				nextRound := new(big.Int).Add(c.current.Round(), common.Big1)
				logger.Warn("Error on commit, waiting for desired round", "reason", "getAggregatedSeal", "err", err, "desired_round", nextRound)
				c.waitForDesiredRound(nextRound)
				return nil
			}

			aggregatedEpochValidatorSetSeal.Bitmap = epochBitmap
			aggregatedEpochValidatorSetSeal.Signature = epochAggregate
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

// generateAggregateCommittedSeal will generate the aggregate committed seal
// verify it and return it.
func (c *core) generateAggregateCommittedSeal() (types.IstanbulAggregatedSeal, error) {
	commits := c.current.Commits()
	if commits.Size() < c.current.ValidatorSet().MinQuorumSize() {
		return types.IstanbulAggregatedSeal{}, fmt.Errorf("insufficient commits to construct aggregate seal")
	}
	bitmap, aggregate, err := AggregateSeals(
		commits,
		func(c *istanbul.CommittedSubject) []byte { return c.CommittedSeal },
	)
	if err != nil {
		return types.IstanbulAggregatedSeal{}, fmt.Errorf("failed to aggregate seals: %v", err)
	}

	aggregatedSeal := types.IstanbulAggregatedSeal{Bitmap: bitmap, Signature: aggregate, Round: c.current.Round()}
	err = c.backend.VerifyAggregatedSeal(c.current.Proposal().Hash(), c.current.ValidatorSet(), aggregatedSeal)
	if err != nil {
		return types.IstanbulAggregatedSeal{}, fmt.Errorf("failed verify aggregate seal: %v", err)
	}
	return aggregatedSeal, nil
}

// removeInvalidCommittedSeals individually verifies the committed seal on each
// commit message and discards commits with invalid committed seals.
//
// Note that commit messages are discarded from memory but the removal is not
// persisted to disk, this should not pose a problem however because if this
// step is reached again they will again be discarded.
func (c *core) removeInvalidCommittedSeals() {
	commits := c.current.Commits()
	for _, msg := range commits.Values() {
		// Continue if this commit has already been validated.
		if msg.Commit().CommittedSealValid() {
			continue
		}
		err := c.verifyCommittedSeal(msg.Commit(), c.current.GetValidatorByAddress(msg.Address))
		if err != nil {
			commits.Remove(msg.Address)
		} else {
			// Mark this committed seal as valid
			msg.Commit().SetCommittedSealValid()
		}
	}
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
	return blscrypto.VerifySignature(src.BLSPublicKey(), seal, []byte{}, comSub.CommittedSeal, false, false)
}

// verifyEpochValidatorSetSeal verifies the epoch validator set seal in the received COMMIT message
func (c *core) verifyEpochValidatorSetSeal(comSub *istanbul.CommittedSubject, blockNumber uint64, newValSet istanbul.ValidatorSet, src istanbul.Validator) error {
	if blockNumber == 0 {
		return nil
	}
	epochData, epochExtraData, cip22, err := c.generateEpochValidatorSetData(blockNumber, uint8(comSub.Subject.View.Round.Uint64()), comSub.Subject.Digest, newValSet)
	if err != nil {
		if err == errNotLastBlockInEpoch {
			return nil
		}
		return err
	}
	return blscrypto.VerifySignature(src.BLSPublicKey(), epochData, epochExtraData, comSub.EpochValidatorSetSeal, true, cip22)
}
