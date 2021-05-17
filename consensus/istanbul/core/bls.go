package core

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-bls-go/bls"
)

// type BLSSeal interface {
// 	Seal() []byte
// 	Verify(key blscrypto.SerializedPublicKey, signature []byte) error
// }

type BLSSeal struct {
	Seal            []byte
	ExtraData       []byte
	CompositeHasher bool
	Cip22           bool
}

func (b *BLSSeal) Verify(key blscrypto.SerializedPublicKey, signature []byte) error {
	return blscrypto.VerifySignature(key, b.Seal, b.ExtraData, signature, b.CompositeHasher, b.Cip22)
}

func (b *BLSSeal) VerifyAggregate(publicKeys []blscrypto.SerializedPublicKey, signature []byte) error {
	publicKeyObjs := []*bls.PublicKey{}
	for _, publicKey := range publicKeys {
		publicKeyObj, err := bls.DeserializePublicKeyCached(publicKey[:])
		if err != nil {
			return err
		}
		defer publicKeyObj.Destroy()
		publicKeyObjs = append(publicKeyObjs, publicKeyObj)
	}
	apk, err := bls.AggregatePublicKeys(publicKeyObjs)
	if err != nil {
		return err
	}
	defer apk.Destroy()

	signatureObj, err := bls.DeserializeSignature(signature)
	if err != nil {
		return err
	}
	defer signatureObj.Destroy()

	return apk.VerifySignature(b.Seal, b.ExtraData, signatureObj, b.CompositeHasher, b.Cip22)
}

// verifyCommittedSeal verifies the commit seal in the received COMMIT message
func (c *core) verifyCommittedSeal(comSub *istanbul.CommittedSubject, src istanbul.Validator) error {
	seal, extraData, compositeHasher, cip22, err := PrepareCommitSeal(comSub.Subject.Digest, comSub.Subject.View.Round)
	if err != nil {
		return err
	}
	return blscrypto.VerifySignature(src.BLSPublicKey(), seal, extraData, comSub.CommittedSeal, compositeHasher, cip22)
}

// verifyEpochValidatorSetSeal verifies the epoch validator set seal in the received COMMIT message
func (c *core) verifyEpochValidatorSetSeal(comSub *istanbul.CommittedSubject, blockNumber uint64, newValSet istanbul.ValidatorSet, src istanbul.Validator) error {
	epochData, epochExtraData, cip22, err := c.generateEpochValidatorSetData(blockNumber, uint8(comSub.Subject.View.Round.Uint64()), comSub.Subject.Digest, newValSet)
	if err != nil {
		return err
	}
	return blscrypto.VerifySignature(src.BLSPublicKey(), epochData, epochExtraData, comSub.EpochValidatorSetSeal, true, cip22)
}

func NewCommitSeal(hash common.Hash, round *big.Int) *BLSSeal {
	var buf bytes.Buffer
	buf.Write(hash.Bytes())
	buf.Write(round.Bytes())
	buf.Write([]byte{byte(istanbul.MsgCommit)})
	return &BLSSeal{
		Seal:            buf.Bytes(),
		ExtraData:       []byte{},
		CompositeHasher: false,
		Cip22:           false,
	}
}

// PrepareCommitSeal returns a commit seal for the given hash and round number.
func PrepareCommitSeal(hash common.Hash, round *big.Int) (message []byte, extraData []byte, compositeHasher, cip22 bool, err error) {
	var buf bytes.Buffer
	buf.Write(hash.Bytes())
	buf.Write(round.Bytes())
	buf.Write([]byte{byte(istanbul.MsgCommit)})
	return buf.Bytes(), []byte{}, false, false, nil
}

func NewEpochSeal(keys []blscrypto.SerializedPublicKey, maxNonSigners uint32, epochNum uint16) (*BLSSeal, error) {
	message, extraData, err = blscrypto.EncodeEpochSnarkData(keys, maxNonSigners, epochNum)
	// This is before the Donut hardfork, so signify this doesn't use CIP22.
	return message, extraData, true, false, err
}

func PrepareEpochDonut(keys []blscrypto.SerializedPublicKey, maxNonSigners uint32, epochNum uint16) (message []byte, extraData []byte, compositeHasher, cip22 bool, err error) {
	message, extraData, err = blscrypto.EncodeEpochSnarkData(keys, maxNonSigners, epochNum)
	// This is before the Donut hardfork, so signify this doesn't use CIP22.
	return message, extraData, true, false, err
}

// Generates serialized epoch data for use in the Plumo SNARK circuit.
// Block number and hash may be information for a pending block.
func (c *core) generateEpochValidatorSetData(blockNumber uint64, round uint8, blockHash common.Hash, newValSet istanbul.ValidatorSet) ([]byte, []byte, bool, error) {
	// Serialize the public keys for the validators in the validator set.
	blsPubKeys := []blscrypto.SerializedPublicKey{}
	for _, v := range newValSet.List() {
		blsPubKeys = append(blsPubKeys, v.BLSPublicKey())
	}

	maxNonSigners := uint32(newValSet.Size() - newValSet.MinQuorumSize())

	// Before the Donut fork, use the snark data encoding with epoch entropy.
	if !c.backend.ChainConfig().IsDonut(big.NewInt(int64(blockNumber))) {
		message, extraData, err := blscrypto.EncodeEpochSnarkData(
			blsPubKeys,
			maxNonSigners,
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

// AggregateSeals returns the bls aggregation of the committed seals for the
// messgages in mset. It returns a big.Int that represents a bitmap where each
// set bit corresponds to the position of a validator in the list of validators
// for this epoch that contributed a seal to the returned aggregate. It is
// assumed that mset contains only commit messages.
func AggregateSeals(mset MessageSet, sealExtractor func(*istanbul.CommittedSubject) []byte) (bitmap *big.Int, aggregateSeal []byte, err error) {
	bitmap = big.NewInt(0)
	committedSeals := make([][]byte, mset.Size())
	for i, v := range mset.Values() {
		committedSeals[i] = make([]byte, types.IstanbulExtraBlsSignature)

		commit := v.Commit()
		copy(committedSeals[i][:], sealExtractor(commit))

		j, err := mset.GetAddressIndex(v.Address)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"missing validator %q for committed seal at %s: %v",
				v.Address.String(),
				commit.Subject.View.String(),
				err,
			)
		}
		bitmap.SetBit(bitmap, int(j), 1)
	}

	aggregateSeal, err = blscrypto.AggregateSignatures(committedSeals)
	if err != nil {
		return nil, nil, fmt.Errorf("aggregating signatures failed: %v", err)
	}

	return bitmap, aggregateSeal, nil
}

// UnionOfSeals combines a BLS aggregated signature with an array of signatures. Accounts for
// double aggregating the same signature by only adding aggregating if the
// validator was not found in the previous bitmap.
// This function assumes that the provided seals' validator set is the same one
// which produced the provided bitmap
func UnionOfSeals(aggregatedSignature types.IstanbulAggregatedSeal, seals MessageSet) (types.IstanbulAggregatedSeal, error) {
	// TODO(asa): Check for round equality...
	// Check who already has signed the message
	newBitmap := new(big.Int).Set(aggregatedSignature.Bitmap)
	committedSeals := [][]byte{}
	committedSeals = append(committedSeals, aggregatedSignature.Signature)
	for _, v := range seals.Values() {
		valIndex, err := seals.GetAddressIndex(v.Address)
		if err != nil {
			return types.IstanbulAggregatedSeal{}, err
		}

		// if the bit was not set, this means we should add this signature to
		// the batch
		if newBitmap.Bit(int(valIndex)) == 0 {
			newBitmap.SetBit(newBitmap, (int(valIndex)), 1)
			committedSeals = append(committedSeals, v.Commit().CommittedSeal)
		}
	}

	asig, err := blscrypto.AggregateSignatures(committedSeals)
	if err != nil {
		return types.IstanbulAggregatedSeal{}, err
	}

	return types.IstanbulAggregatedSeal{
		Bitmap:    newBitmap,
		Signature: asig,
		Round:     aggregatedSignature.Round,
	}, nil
}
