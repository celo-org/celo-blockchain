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

type BLSSeal struct {
	Seal            []byte
	ExtraData       []byte
	CompositeHasher bool
	Cip22           bool
}

type signFn func(seal []byte, extraData []byte, compositeHasher, cip22 bool) ([]byte, error)

func (b *BLSSeal) Sign(sign signFn) ([]byte, error) {
	return sign(b.Seal, b.ExtraData, b.CompositeHasher, b.Cip22)
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

// verifyEpochValidatorSetSeal verifies the epoch validator set seal in the received COMMIT message
func (c *core) verifyEpochValidatorSetSeal(comSub *istanbul.CommittedSubject, blockNumber uint64, newValSet istanbul.ValidatorSet, src istanbul.Validator) error {
	seal, err := c.generateEpochValidatorSetData(blockNumber, uint8(comSub.Subject.View.Round.Uint64()), comSub.Subject.Digest, newValSet)
	if err != nil {
		return err
	}
	return seal.Verify(src.BLSPublicKey(), comSub.EpochValidatorSetSeal)
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

func NewEpochSeal(keys []blscrypto.SerializedPublicKey, maxNonSigners uint32, epochNum uint16) (*BLSSeal, error) {
	message, extraData, err := blscrypto.EncodeEpochSnarkData(keys, maxNonSigners, epochNum)

	// This is before the Donut hardfork, so signify this doesn't use CIP22.
	return &BLSSeal{
		Seal:            message,
		ExtraData:       extraData,
		CompositeHasher: true,
		Cip22:           false,
	}, err
}

func NewEpochSealDonut(keys []blscrypto.SerializedPublicKey, quorumSize uint32, epochNum uint16, round uint8, blockHash, parentEpochBlockHash common.Hash) (*BLSSeal, error) {
	message, extraData, err := blscrypto.EncodeEpochSnarkDataCIP22(
		keys, maxValidators-quorumSize, maxValidators,
		epochNum,
		round,
		blscrypto.EpochEntropyFromHash(blockHash),
		blscrypto.EpochEntropyFromHash(parentEpochBlockHash),
	)
	return &BLSSeal{
		Seal:            message,
		ExtraData:       extraData,
		CompositeHasher: true,
		Cip22:           true,
	}, err
}

// Generates serialized epoch data for use in the Plumo SNARK circuit.
// Block number and hash may be information for a pending block.
func (c *core) generateEpochValidatorSetData(blockNumber uint64, round uint8, blockHash common.Hash, newValSet istanbul.ValidatorSet) (*BLSSeal, error) {
	// Serialize the public keys for the validators in the validator set.
	blsPubKeys := []blscrypto.SerializedPublicKey{}
	for _, v := range newValSet.List() {
		blsPubKeys = append(blsPubKeys, v.BLSPublicKey())
	}

	maxNonSigners := uint32(newValSet.Size() - newValSet.MinQuorumSize())

	epochNum := uint16(istanbul.GetEpochNumber(blockNumber, c.config.Epoch))
	// Before the Donut fork, use the snark data encoding with epoch entropy.
	if !c.backend.ChainConfig().IsDonut(big.NewInt(int64(blockNumber))) {
		return NewEpochSeal(blsPubKeys, maxNonSigners, epochNum)
	}

	// Retrieve the block hash for the last block of the previous epoch.
	parentEpochBlockHash := c.backend.HashForBlock(blockNumber - c.config.Epoch)
	if blockNumber > 0 && parentEpochBlockHash == (common.Hash{}) {
		return nil, errors.New("unknown block")
	}

	return NewEpochSealDonut(blsPubKeys, uint32(newValSet.MinQuorumSize()), epochNum, round, blockHash, parentEpochBlockHash)
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
