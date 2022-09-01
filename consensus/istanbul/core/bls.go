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
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-bls-go/bls"
)

// Make a local copy of types.IstanbulAggregatedSeal so that we can add functionality to it.
// Unfortunately it is not easy to move types.IstanbulAggregatedSeal here.
type IstanbulAggregatedSeal types.IstanbulAggregatedSeal

func (s IstanbulAggregatedSeal) Verify(digest common.Hash, validators istanbul.ValidatorSet) error {
	return NewCommitSeal(digest, s.Round).VerifyAggregate(validators, s.Signature, s.Bitmap)
}

// SignatureExtractorFunction extracts a bls signature and indication of
// whether that signature has already been successfully verified from a commit
// message. Currently we just have commit signatures and epoch signatures.
type SignatureExtractorFunction func(*istanbul.CommittedSubject) (alreadyVerified bool, signature []byte)

// SignatureMarkerFunction marks a commit's bls signature as valid. Currently we
// only have commit signatures and epoch signatures.
type SignatureMarkerFunction func(*istanbul.CommittedSubject)

var (
	extractCommitSignature = func(c *istanbul.CommittedSubject) (alreadyVerified bool, signature []byte) {
		return c.CommittedSealVerified(), c.CommittedSeal
	}

	markCommitSignatureVerified = func(c *istanbul.CommittedSubject) {
		c.SetCommittedSealVerified()
	}

	extractEpochSignature = func(c *istanbul.CommittedSubject) (alreadyVerified bool, signature []byte) {
		return false, c.EpochValidatorSetSeal
	}

	markEpochSignatureVerified = func(c *istanbul.CommittedSubject) {
		c.SetEpochSealVerified()
	}
)

// BLSSeal encapsulates functionality to sign, verify, verify in aggregate,
// extract and mark verified different seals. Depending on the parameterisation
// an instance of BLSSeal can represent the commit seal (found on each block)
// and both types of epoch block seal (found on the last block of an epoch).
type BLSSeal struct {
	Seal             []byte
	ExtraData        []byte
	CompositeHasher  bool
	Cip22            bool
	ExtractSignature SignatureExtractorFunction
	MarkVerified     SignatureMarkerFunction
}

func NewCommitSeal(hash common.Hash, round *big.Int) *BLSSeal {
	var buf bytes.Buffer
	buf.Write(hash.Bytes())
	buf.Write(round.Bytes())
	buf.Write([]byte{byte(istanbul.MsgCommit)})
	return &BLSSeal{
		Seal:             buf.Bytes(),
		ExtraData:        []byte{},
		CompositeHasher:  false,
		Cip22:            false,
		ExtractSignature: extractCommitSignature,
		MarkVerified:     markCommitSignatureVerified,
	}
}

func NewEpochSeal(keys []blscrypto.SerializedPublicKey, maxNonSigners uint32, epochNum uint16) (*BLSSeal, error) {
	message, extraData, err := blscrypto.EncodeEpochSnarkData(keys, maxNonSigners, epochNum)

	// This is before the Donut hardfork, so signify this doesn't use CIP22.
	return &BLSSeal{
		Seal:             message,
		ExtraData:        extraData,
		CompositeHasher:  true,
		Cip22:            false,
		ExtractSignature: extractEpochSignature,
		MarkVerified:     markEpochSignatureVerified,
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
		Seal:             message,
		ExtraData:        extraData,
		CompositeHasher:  true,
		Cip22:            true,
		ExtractSignature: extractEpochSignature,
		MarkVerified:     markEpochSignatureVerified,
	}, err
}

type signFn func(seal []byte, extraData []byte, compositeHasher, cip22 bool) ([]byte, error)

func (b *BLSSeal) Sign(sign signFn) ([]byte, error) {
	return sign(b.Seal, b.ExtraData, b.CompositeHasher, b.Cip22)
}

func (b *BLSSeal) Verify(key blscrypto.SerializedPublicKey, signature []byte) error {
	return blscrypto.VerifySignature(key, b.Seal, b.ExtraData, signature, b.CompositeHasher, b.Cip22)
}

func (b *BLSSeal) VerifyAggregate(validators istanbul.ValidatorSet, signature []byte, bitmap *big.Int) error {

	if len(signature) != types.IstanbulExtraBlsSignature {
		return fmt.Errorf("invalid aggregate seal, expecting length %d, but got length %d", types.IstanbulExtraBlsSignature, len(signature))
	}

	// Find which public keys signed from the provided validator set
	publicKeys := []*bls.PublicKey{}
	for i := 0; i < validators.Size(); i++ {
		if bitmap.Bit(i) == 1 {
			serializedKey := validators.GetByIndex(uint64(i)).BLSPublicKey()
			publicKey, err := bls.DeserializePublicKeyCached(serializedKey[:])
			if err != nil {
				return err
			}
			defer publicKey.Destroy()
			publicKeys = append(publicKeys, publicKey)
		}
	}

	// The length of a valid seal should be greater than the minimum quorum size
	if len(publicKeys) < validators.MinQuorumSize() {
		return fmt.Errorf(
			"aggregated seal does not aggregate enough seals, numSeals %d minimum quorum size %d",
			len(publicKeys),
			validators.MinQuorumSize(),
		)
	}

	apk, err := bls.AggregatePublicKeys(publicKeys)
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

// GenerateValidAggregateSignature will generate an aggregate signature of the
// provided seal. It is assumed that there will be exactly a quorum of messages
// in commits, because core handles messages in a single threaded manner so
// this will be called whenever we reach a quorum of messages. If the aggregate
// signature turns out to be invalid then messages will be individually
// verified and messges with invalid signatures removed from the message set.
// If no messages are removed during this step then an error will be returned,
// because this implies that there is a problem with the code. Otherwise if
// individual messages with invalid signatures are found and removed then a nil
// signature is returned since there are not enough messages left to form a
// quorum.
func GenerateValidAggregateSignature(
	logger log.Logger,
	seal *BLSSeal,
	commits MessageSet,
	validators istanbul.ValidatorSet,
	extractSignature SignatureExtractorFunction,
) ([]byte, *big.Int, error) {

	l := logger.New("func", "GenerateValidAggregateSignature")
	bitmap, aggregate, err := AggregateSeals(
		commits,
		extractSignature,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to aggregate seals: %v", err)
	}

	err = seal.VerifyAggregate(validators, aggregate, bitmap)
	if err == nil {
		// No error return the aggregate seal
		return aggregate, bitmap, nil
	}

	// Attempt to remove bad seals, if we do not remove any bad seals then
	// there must be some other problem in the code.
	prevSize := commits.Size()
	RemoveInvalidSignatures(l, seal, commits, validators)
	if commits.Size() == prevSize {
		return nil, nil, fmt.Errorf("failed to verify aggregate seal: %v", err)
	}
	// If commits were removed we now need to wait for a quorum again.
	return nil, nil, nil
}

// RemoveInvalidSignatures individually verifies the signature for the given
// seal on each commit message and discards commits with invalid signatures.
//
// Note that commit messages are discarded from memory but the removal is not
// persisted to disk, this should not pose a problem however because if this
// step is reached again they will again be discarded.
func RemoveInvalidSignatures(logger log.Logger, seal *BLSSeal, commits MessageSet, validators istanbul.ValidatorSet) {
	l := logger.New("func", "RemoveInvalidSignatures")
	for _, msg := range commits.Values() {
		commit := msg.Commit()
		// Continue if this commit has already been validated.
		alreadyVerified, sig := seal.ExtractSignature(commit)
		if alreadyVerified {
			continue
		}
		_, validator := validators.GetByAddress(msg.Address)
		err := seal.Verify(validator.BLSPublicKey(), sig)
		if err != nil {
			commits.Remove(msg.Address)
			l.Warn("Invalid committed seal received", "from", msg.Address.String(), "err", err)
		} else {
			// Mark this committed seal as valid
			seal.MarkVerified(commit)
		}
	}
}

// AggregateSeals returns the bls aggregation of the committed seals for the
// messgages in mset. It returns a big.Int that represents a bitmap where each
// set bit corresponds to the position of a validator in the list of validators
// for this epoch that contributed a seal to the returned aggregate. It is
// assumed that mset contains only commit messages.
func AggregateSeals(mset MessageSet, signatureExtractor SignatureExtractorFunction) (bitmap *big.Int, aggregateSeal []byte, err error) {
	bitmap = big.NewInt(0)
	committedSeals := make([][]byte, mset.Size())
	for i, v := range mset.Values() {
		committedSeals[i] = make([]byte, types.IstanbulExtraBlsSignature)

		commit := v.Commit()
		_, sig := signatureExtractor(commit)
		copy(committedSeals[i][:], sig)

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
func UnionOfSeals(aggregatedSignature types.IstanbulAggregatedSeal, commits MessageSet) (types.IstanbulAggregatedSeal, error) {
	// TODO(asa): Check for round equality...
	// Check who already has signed the message
	newBitmap := new(big.Int).Set(aggregatedSignature.Bitmap)

	committedSeals := make([][]byte, 0, commits.Size()+1)
	committedSeals = append(committedSeals, aggregatedSignature.Signature[:])
	for _, msg := range commits.Values() {
		valIndex, err := commits.GetAddressIndex(msg.Address)
		if err != nil {
			return types.IstanbulAggregatedSeal{}, err
		}
		// Check that round matches
		if msg.Commit().Subject.View.Round.Cmp(aggregatedSignature.Round) != 0 {
			return types.IstanbulAggregatedSeal{}, fmt.Errorf(
				"commit round %s does not match that of aggregate to be unioned with %s",
				msg.Commit().Subject.View.Round.String(),
				aggregatedSignature.Round.String(),
			)
		}

		// if the bit was not set, this means we should add this signature to
		// the batch
		if newBitmap.Bit(int(valIndex)) == 0 {
			newBitmap.SetBit(newBitmap, (int(valIndex)), 1)
			committedSeals = append(committedSeals, msg.Commit().CommittedSeal)
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
