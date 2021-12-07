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
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
)

// maxValidators represents the maximum number of validators the SNARK circuit supports
// The prover code will then pad any proofs to this maximum to ensure consistent proof structure
// TODO: Eventually make this governable
const maxValidators = uint32(150)

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
	if headBlock.Number().Uint64() > 0 {
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
