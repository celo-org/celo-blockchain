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

package backend

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	istanbulCore "github.com/celo-org/celo-blockchain/consensus/istanbul/core"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/uptime"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	gpm "github.com/celo-org/celo-blockchain/contracts/gasprice_minimum"
	"github.com/celo-org/celo-blockchain/core"
	ethCore "github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-blockchain/rpc"
	"github.com/celo-org/celo-bls-go/snark"
	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/crypto/sha3"
)

const (
	inmemorySnapshots             = 128 // Number of recent vote snapshots to keep in memory
	inmemoryPeers                 = 40
	inmemoryMessages              = 1024
	mobileAllowedClockSkew uint64 = 5
)

var (
	// errInvalidProposal is returned when a prposal is malformed.
	errInvalidProposal = errors.New("invalid proposal")
	// errInvalidSignature is returned when given signature is not signed by given
	// address.
	errInvalidSignature = errors.New("invalid signature")
	// errInsufficientSeals is returned when there is not enough signatures to
	// pass the quorum check.
	errInsufficientSeals = errors.New("not enough seals to reach quorum")
	// errUnknownBlock is returned when the list of validators or header is requested for a block
	// that is not part of the local blockchain.
	errUnknownBlock = errors.New("unknown block")
	// errUnauthorized is returned if a header is signed by a non authorized entity.
	errUnauthorized = errors.New("not an elected validator")
	// errInvalidExtraDataFormat is returned when the extra data format is incorrect
	errInvalidExtraDataFormat = errors.New("invalid extra data format")
	// errCoinbase is returned if a block's coinbase is invalid
	errInvalidCoinbase = errors.New("invalid coinbase")
	// errInvalidTimestamp is returned if the timestamp of a block is lower than the previous block's timestamp + the minimum block period.
	errInvalidTimestamp = errors.New("invalid timestamp")
	// errInvalidVotingChain is returned if an authorization list is attempted to
	// be modified via out-of-range or non-contiguous headers.
	errInvalidVotingChain = errors.New("invalid voting chain")
	// errInvalidAggregatedSeal is returned if the aggregated seal is invalid.
	errInvalidAggregatedSeal = errors.New("invalid aggregated seal")
	// errInvalidAggregatedSeal is returned if the aggregated seal is missing.
	errEmptyAggregatedSeal = errors.New("empty aggregated seal")
	// errMismatchTxhashes is returned if the TxHash in header is mismatch.
	errMismatchTxhashes = errors.New("mismatch transactions hashes")
	// errInvalidValidatorSetDiff is returned if the header contains invalid validator set diff
	errInvalidValidatorSetDiff = errors.New("invalid validator set diff")
	// errUnauthorizedAnnounceMessage is returned when the received announce message is from
	// an unregistered validator
	errUnauthorizedAnnounceMessage = errors.New("unauthorized announce message")
	// errNotAValidator is returned when the node is not configured as a validator
	errNotAValidator = errors.New("Not configured as a validator")
)

var (
	now = time.Now

	inmemoryAddresses  = 20 // Number of recent addresses from ecrecover
	recentAddresses, _ = lru.NewARC(inmemoryAddresses)
)

// Author retrieves the Ethereum address of the account that minted the given
// block, which may be different from the header's coinbase if a consensus
// engine is based on signatures.
func (sb *Backend) Author(header *types.Header) (common.Address, error) {
	return ecrecover(header)
}

// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifies the seal regardless of given "seal" argument.
func (sb *Backend) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, seal bool) error {
	return sb.verifyHeader(chain, header, nil)
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (sb *Backend) verifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	if header.Number == nil {
		return errUnknownBlock
	}

	// If the full chain isn't available (as on mobile devices), don't reject future blocks
	// This is due to potential clock skew
	allowedFutureBlockTime := uint64(now().Unix())
	if !chain.Config().FullHeaderChainAvailable {
		allowedFutureBlockTime = allowedFutureBlockTime + mobileAllowedClockSkew
	}

	// Don't waste time checking blocks from the future
	if header.Time > allowedFutureBlockTime {
		return consensus.ErrFutureBlock
	}

	// Ensure that the extra data format is satisfied
	if _, err := types.ExtractIstanbulExtra(header); err != nil {
		return errInvalidExtraDataFormat
	}

	return sb.verifyCascadingFields(chain, header, parents)
}

// A sanity check for lightest mode. Checks that the correct epoch block exists for this header
func (sb *Backend) checkEpochBlockExists(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	number := header.Number.Uint64()
	// Check that latest epoch block is available
	epoch := istanbul.GetEpochNumber(number, sb.config.Epoch)
	epochBlockNumber := istanbul.GetEpochLastBlockNumber(epoch-1, sb.config.Epoch)
	if number == epochBlockNumber {
		epochBlockNumber = istanbul.GetEpochLastBlockNumber(epoch-2, sb.config.Epoch)
	}
	for _, hdr := range parents {
		if hdr.Number.Uint64() == epochBlockNumber {
			return nil
		}
	}
	parent := chain.GetHeaderByNumber(epochBlockNumber)
	log.Error("Correct parent for light chain", "epochblockNumber", epochBlockNumber, "parent", parent)
	if parent == nil || parent.Number.Uint64() != epochBlockNumber {
		return consensus.ErrUnknownAncestor
	}
	return nil
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (sb *Backend) verifyCascadingFields(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	// The genesis block is the always valid dead-end
	number := header.Number.Uint64()
	if number == 0 {
		return nil
	}
	// Ensure that the block's timestamp isn't too close to it's parent
	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
		log.Error("Parent", "parent", parent)
	}
	if chain.Config().FullHeaderChainAvailable {

		if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
			return consensus.ErrUnknownAncestor
		}
		if parent.Time+sb.config.BlockPeriod > header.Time {
			return errInvalidTimestamp
		}
		// Verify validators in extraData. Validators in snapshot and extraData should be the same.
		if err := sb.verifySigner(chain, header, parents); err != nil {
			return err
		}
	} else if err := sb.checkEpochBlockExists(chain, header, parents); err != nil {
		return err
	}

	return sb.verifyAggregatedSeals(chain, header, parents)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (sb *Backend) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		errored := false
		for i, header := range headers {
			var err error
			if errored {
				err = consensus.ErrUnknownAncestor
			} else {
				err = sb.verifyHeader(chain, header, headers[:i])
			}

			if err != nil {
				errored = true
			}

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// verifySigner checks whether the signer is in parent's validator set
func (sb *Backend) verifySigner(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	// Verifying the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}

	// Retrieve the snapshot needed to verify this header and cache it
	snap, err := sb.snapshot(chain, number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}

	// resolve the authorization key and check against signers
	signer, err := ecrecover(header)
	if err != nil {
		return err
	}

	// Signer should be in the validator set of previous block's extraData.
	if _, v := snap.ValSet.GetByAddress(signer); v == nil {
		return errUnauthorized
	}
	return nil
}

// verifyAggregatedSeals checks whether the aggregated seal and parent seal in the header is
// signed on by the block's validators and the parent block's validators respectively
func (sb *Backend) verifyAggregatedSeals(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	number := header.Number.Uint64()
	// We don't need to verify committed seals in the genesis block
	if number == 0 {
		return nil
	}

	extra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		return err
	}

	// The length of Committed seals should be larger than 0
	if len(extra.AggregatedSeal.Signature) == 0 {
		return errEmptyAggregatedSeal
	}

	// Check the signatures on the current header
	snap, err := sb.snapshot(chain, number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}
	validators := snap.ValSet.Copy()
	err = sb.verifyAggregatedSeal(header.Hash(), validators, extra.AggregatedSeal)
	if err != nil {
		return err
	}

	// The genesis block is skipped since it has no parents.
	// The first block is also skipped, since its parent
	// is the genesis block which contains no parent signatures.
	// The parent commit messages are only used for the uptime calculation,
	// so ultralight clients don't need to verify them
	if number > 1 && chain.Config().FullHeaderChainAvailable {
		sb.logger.Trace("verifyAggregatedSeals: verifying parent seals for block", "num", number)
		var parentValidators istanbul.ValidatorSet
		// The first block in an epoch will have a different validator set than the block
		// before it. If the current block is the first block in an epoch, we need to fetch the previous
		// validator set to validate the parent signatures.
		if number%sb.config.Epoch == 1 {
			snap, err := sb.snapshot(chain, number-2, common.Hash{}, nil)
			if err != nil {
				return err
			}
			parentValidators = snap.ValSet.Copy()
		} else {
			parentValidators = validators.Copy()
		}

		// Check the signatures made by the validator set corresponding to the
		// parent block's hash. We use header.ParentHash to handle both
		// ultralight and non-ultralight cases.
		// parent.Hash() would correspond to the previous epoch
		// block in ultralight, while the extra.ParentCommit is made on the block which was
		// immediately before the current block.
		return sb.verifyAggregatedSeal(header.ParentHash, parentValidators, extra.ParentAggregatedSeal)
	}

	return nil
}

func (sb *Backend) verifyAggregatedSeal(headerHash common.Hash, validators istanbul.ValidatorSet, aggregatedSeal types.IstanbulAggregatedSeal) error {
	logger := sb.logger.New("func", "Backend.verifyAggregatedSeal()")
	if len(aggregatedSeal.Signature) != types.IstanbulExtraBlsSignature {
		return errInvalidAggregatedSeal
	}

	proposalSeal := istanbulCore.PrepareCommittedSeal(headerHash, aggregatedSeal.Round)
	// Find which public keys signed from the provided validator set
	publicKeys := []blscrypto.SerializedPublicKey{}
	for i := 0; i < validators.Size(); i++ {
		if aggregatedSeal.Bitmap.Bit(i) == 1 {
			pubKey := validators.GetByIndex(uint64(i)).BLSPublicKey()
			publicKeys = append(publicKeys, pubKey)
		}
	}
	// The length of a valid seal should be greater than the minimum quorum size
	if len(publicKeys) < validators.MinQuorumSize() {
		logger.Error("Aggregated seal does not aggregate enough seals", "numSeals", len(publicKeys), "minimum quorum size", validators.MinQuorumSize())
		return errInsufficientSeals
	}
	err := blscrypto.VerifyAggregatedSignature(publicKeys, proposalSeal, []byte{}, aggregatedSeal.Signature, false, false)
	if err != nil {
		logger.Error("Unable to verify aggregated signature", "err", err)
		return errInvalidSignature
	}

	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (sb *Backend) VerifySeal(header *types.Header) error {
	// Ensure the block number is greater than zero, but less or equal to than max uint64.
	if header.Number.Cmp(common.Big0) <= 0 || !header.Number.IsUint64() {
		return errUnknownBlock
	}

	extra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		return errInvalidExtraDataFormat
	}

	// Acquire the validator set whose signatures will be verified.
	// FIXME: Based on the current implemenation of validator set construction, only validator sets
	// from the canonical chain will be used. This means that if the provided header is a valid
	// member of a non-canonical chain, seal verification will only succeed if the validator set
	// happens to be the same as the canonical chain at the same block number (as would be the case
	// for a fork from the canonical chain which does not cross an epoch boundary)
	valSet := sb.getValidators(header.Number.Uint64()-1, header.ParentHash)
	return sb.verifyAggregatedSeal(header.Hash(), valSet, extra.AggregatedSeal)
}

func (sb *Backend) VerifyLightPlumoProofs(lightProofs []istanbul.LightPlumoProof) error {
	// TODO batching, version numbers
	verificationKey, _ := hex.DecodeString("944a99f744e12f5cdf28b0e7a19e00fe60600a6680a6423171128c4b7f75f6bfd6b49c9197eb03e7e0a57f2dad2b5898d9f61d580adc26bf9b616bb5d67ca32c224ec849b9fa49247b31a4c25940477d440de37ffe8d9bda48eaf56fbe884e80233abb1e1c4b090832625460f14f00c6175da2493079c5ebc92d52a09f538442e0a7472e028a9947b9baa44b7ec09c43e50880ae51df7f797cd3d683c6ee466496837b8687596c43d907a87eb0bcc5734ba26ddd1191373ceada8df887171200074d198e25c9fae6544a6812366c0fb9d0a0865a7b4f6c90ae300c049bd5ec2278890e4d8f494ddd2b0c559c2a60612d981074bd84c29f60fb224172adc60e6113c09a144578c456b9d8e37a87504b5565315906595ccc9330858fcbf59820811a2f45661bb05c6ffffdfffa153cd202ab5dbf19057cefba9198a8eabeeb347217b3d882f8eb90efad01a837f35e3f49c1fd571c15d85dbd6072ffce3b2836784f36c596aed9511b8b3d5e2754668a5206d131642fbdae55cc685ce91ffb5b800300000000000000211c999ecd73a9ebf0feab7f15e1b51462dcb111d9c9c3e67938095623f1e5a35644115d964aec176d74800fb0d80ac4fdd461bb841dd136ba5859889a2f07e0bf055434004bd0c73e5e87d3a1755a5c3ceaac89f0058f76c32e4acca8ffc10026ad7185527b077096d160630d8c418317a78c75a582cdf9ebe9b72365fea3135fce6bf929de11c8f2e38f02baa88fa69509ef1f91bcecba13b2b8eb70ca4bef3339a2e7dbe8d13ea58a822888be18871fb79947c7d0d970326721ac3089668039db236f4433bd957fc2c9b65d0566a316982f1fa046bdc29090b84b2fb62dd9d07b2576428b04f797a0b4cc1b22e09c9826b13d82e9dfd5f9b280a9a1f5034c365d6911bada9dfb16e5e6791cefd484ae20883be201a06315f08445fdf06600")
	// verificationKey, _ := hex.DecodeString("a548b97fa96ffc5da4af019dc65c57016002b8d388fca6f68b9274f5344705fb5f3ecda9ebdd4e3dfad1227e5df2f79b910ddabb264a060b8aa8340d37f44572b5a995a1b3365e70452f7e6f3f1b56b5a18c55b39b9a57edbef7f0f0666cda0000041f30136b2f8b3f52dd5329ad2e02d324e3d5e0fa79a8714c12039b7bfbfeba0eda2a36a16885f567b3d82243bfe5d14d409275b57d9d42ae24fa25f4e0b9345fa64a5184849f827b22baa7c0288d26e33f0bfcd518804645f99181dcea80cb336cb3318c065ef3b754eee8dfac8cf9c6604f42c13589a7169881f591f71c658b47b58c02b44b4abef3e333db172bd2c6512b28928d1bd4b0813ab96923d7fc670dce3fe19d64c3a3dca872f2f287cd07ce6b52b672b9f8a5732925090101a9872d4d7d730e225da9b525a4c3d250e97c33aa5957a7cb4728739051b5d9e52cf5f0a1a0f86fc1892ce270095a0ce003dcf33ba0e9c280e99139deeedfc60db720c6087ce7d7184bfb2edfa95b3d59cdf257fdb8f676d711becc1fbc91b4800300000000000000124eac58d55d41fce4211aba1a24665a611dc36c57a338cd05e7df722570c048b7135fc5c9800656219b03d75adb77b4422593ae46108ba32df4c58644588ba05ada2cdb75028214246fe57bf2e1d9b3f1ff32cdf15b2f96ad41bc5fad8a50808997cc4a4d2e43029d3a1b945bc87b110da4eac406f1e20f48e8db7d374747deb25bfa45d1b67c079aae91949037ed83b276af1e9e9b6395459d62122effeaf5b3f26a0c1b1d7e9964d6b28c8c9f45f8ee440e1af66ae5ddde1c5149dcb26c807abc36bc2e497f439c81b1e396294003ec225838ca925883ba3f74874d548bffb8b47ee8739af0b8bb12c0b5d57c8b86dd9d6f33fb5067994a6a8f57f7a2a4b77ef6be3657be3205545362b5ac2eb299127844195f797937444e2633eb95ca00")
	// verificationKey, _ := hex.DecodeString("27de9b5798fd5c832aaabe2b1bb480473df29203a3a1971aed07a21441a6f21573031b01b821546c16f8470dda69131439854051daf5fb6ab7b6815cd9acc0c583ca1376d37343d69fc397323be1d601c7cd43fc42d4361aa27cdf8bc565d3731a93514b0fd38659ebc40f3c44ba9c6882a2e820828135d292d9bb7734afb8094e1e8a9c755420b93155ac4caefe44dd6a64e732daf6e0e141b26c88a30353d627e3a4afec307e285607d18261c04cf8b568eb4fdb3f48da889e8d46c4ff2712fd5664326a9642161463b234dd7b89f1fccb1a6c9acc26b1994c58b68106276345ecf79e0137a941be4bca06489616810b7db4d93fd8b3f6615e84b9683112cbf8e696867656616b5dbd60553df7f92fc1300c0aaf6217b2561b7f10671ae540c209f22a8e2cb98fb60e118aa2c372a44db6a404eb19d7e68ec9d0f0b4521a7f405ca2f421cf4b4128176a34cdf49a9a471dc197e4ee5c8ccb9342fb967173eb3137659930767af5228d1c36038885512286189c358cfc053c1319d4a2f8e50ca32ab13468d86aa059ed201de13f0eae7f4a687baa5d212df991cc5e656200fe705cb8e293614eebc196acc1a5d493d189162e0eeef2c68a37d58cc6717e9b649a595293cd18cc27f8aed9863b0a727b5e12cfa4e90513a282bb65658bd786968f89993ebbc7006f810f0cb6a7002599739fa40f337eaab7e639f79ebdf85ffecf106d5fa8464a197803f8a742a1ef2f05d03c592c97abdd0847c45f5b8878443319618ff04b7a3de13ce778c58c79b1c7f8d6e900d02fa1279dde856c7a62e1b13aff7a42b63bed3c2100318d515c0175c0c067b1ae5b4dc17c3423290c0d5c5e4c6da641073562f5ba99de560671bce41e2aa0abbeb2176b13802e58ba424ce8d790c7d5b610a409e26c718ef1a939198d3a32aef7f5b83347b15fc2b4adfebc468e7bb4f5667a70ea45c3b7135e0efc2e1738ab5b033176f75780fa17415fb5037c748d6be0a6ae68235cd307bf08f890261378aad62ef89395e33004c1e5f50a9fc261fd6a6f175e64110f15af68d3e888f132348039e3fb8db3ac974b8b07867b452bb73584d4c6e7ba5f790c119500b7b538cddcfc81ee754db65aca15f5853c806a40b38e02c0ed2a797fcc11afe2e26f972b4a7f8fe70e626e5d3b4fa600ee730f6a27b9fb448daa9b364ede0feefba1fc74a2d546eb74ac2c61f7d8033e0df3e214d3f12b8bf229692148305e2d82e3ad4c8266b7da641a42c21689e1e738b2689d1bc31afcb6d2b53a87afabd970de2da2e26431a1d4178a6808a23ca803a479e15ea5b7875adc1f4456ce88a3a94a7aecc9763899a353bcc1363a746ab55bd3322d44919eb0a3350c1a8eb574f7220300000000000000a21df711258265911c858a49dc5e866c47f4da8c6f24c4d5002156f719631fcf22af144657300ce576ca5523c69a2cef3e1771a054f51365321a78e2877a8505a02a25300f414035e4f84517141ada5e702376358634fe0b2d5477bfcda04f6ced04908edbd11f470d8e060600031662f9bb513919ab3b21846461a9c70c486adb9a200f16eeb66bf4d550bcfc44b2b29af9338b9c054f04c00eda28301a5d78ad5352fb88ee7623a5554a08a8f886e325a9a5cc4071fbe168860614a0a4e6ffd3aa7c2837fea5669ccd78c6c4c6573d1512ac83ecca8bb2b31d2a7a8870e84763eee2e6f47dc83ab5a78c6a45254c003853a563d59a00bc9cfec4586ef5b5b6e37564304f1ea1ad6c002b0f016863e0af2229a473258e9f1d91926ad75a310c5adead0bbda1")
	for _, lightProof := range lightProofs {
		firstValSet := sb.getValidators(uint64(lightProof.FirstEpoch), common.Hash{})
		firstSerializedValSet := istanbul.MapValidatorsToPublicKeys(firstValSet.List())
		var firstSerializedValSetBytes [][]byte
		for _, pubKey := range firstSerializedValSet {
			firstSerializedValSetBytes = append(firstSerializedValSetBytes, pubKey[:])
		}
		firstEpochBlockNumber := istanbul.GetEpochLastBlockNumber(uint64(lightProof.FirstEpoch), sb.config.Epoch)
		firstBlockHeader := sb.chain.GetHeaderByNumber(firstEpochBlockNumber)
		epochEntropy := blscrypto.EpochEntropyFromHash(firstBlockHeader.Hash())
		parentEpochBlockNumber := istanbul.GetEpochLastBlockNumber(uint64(lightProof.FirstEpoch)-1, sb.config.Epoch)
		parentBlockHeader := sb.chain.GetHeaderByNumber(parentEpochBlockNumber)
		parentEpochEntropy := blscrypto.EpochEntropyFromHash(parentBlockHeader.Hash())
		firstEpochBlock := snark.EpochBlock{
			Index:         uint16(lightProof.FirstEpoch),
			MaxNonSigners: uint32(firstValSet.Size() - firstValSet.MinQuorumSize()),
			PublicKeys:    firstSerializedValSetBytes,
			EpochEntropy:  epochEntropy[:],
			ParentEntropy: parentEpochEntropy[:],
		}
		log.Error("Verify light plumo proofs", "first epopch block", firstEpochBlock)
		firstValData := istanbul.MapValidatorsToValidatorData(firstValSet.List())
		lastValData := istanbul.SnarkUpdateValSet(firstValData, lightProof.ValidatorPositions, lightProof.NewValidators)

		// lastSerializedValSet := istanbul.SnarkUpdatePublicKeySet(firstSerializedValSet, lightProof.ValidatorPositions, lightProof.NewValidators)

		var lastSerializedValSetBytes [][]byte
		for _, valData := range lastValData {
			lastSerializedValSetBytes = append(lastSerializedValSetBytes, valData.BLSPublicKey[:])
		}
		lastEpochBlockNumber := istanbul.GetEpochLastBlockNumber(uint64(lightProof.LastEpoch.Index), sb.config.Epoch)
		lastEpochEntropy := blscrypto.EpochEntropyFromHash(lightProof.LastEpoch.Header.Hash())
		lastParentEpochEntropy := blscrypto.EpochEntropyFromHash(lightProof.LastEpoch.ParentHeader.Hash())
		lastEpochBlock := snark.EpochBlock{
			Index: uint16(lightProof.LastEpoch.Index),
			// TODO get last?
			MaxNonSigners: uint32(firstValSet.Size() - firstValSet.MinQuorumSize()),
			PublicKeys:    lastSerializedValSetBytes,
			EpochEntropy:  lastEpochEntropy[:],
			ParentEntropy: lastParentEpochEntropy[:],
		}
		err := snark.VerifyEpochs(
			verificationKey,
			lightProof.Proof,
			firstEpochBlock,
			lastEpochBlock,
		)
		if err != nil {
			log.Error("Proof verification error")
			return err
		}
		log.Error("Proof verification succeeded!")
		// TODO Verify this updates snapshot correctly and that this index is not off by one
		// firstEpochBlockNumber := istanbul.GetEpochLastBlockNumber(uint64(firstEpochBlock.Index), sb.config.Epoch)
		// lastEpochBlockNumber := istanbul.GetEpochLastBlockNumber(uint64(lastEpochBlock.Index), sb.config.Epoch)
		// Ok so `snapshot` expects that you have epoch headers.. So maybe we can just avoid that and call a subroutine of snapshot, giving the verified proof results..
		log.Error("First epoch block number", "x", firstEpochBlockNumber)
		snap, err := sb.snapshot(sb.chain, 0, common.Hash{}, nil)
		if err != nil {
			log.Error("Error receiving old snapshot", "err", err)
		}
		log.Error("Old snapshot", "snap", snap)
		newSnap := snap.copy()
		newSnap.ValSet = validator.NewSet(lastValData)
		newSnap.Epoch = snap.Epoch
		newSnap.Number = lastEpochBlockNumber
		newSnap.Hash = common.Hash{}
		newSnap.store(sb.db)
		log.Error("Stored snapshot to disk", "number", newSnap.Number, "newSnap", newSnap)
	}
	return nil
}

func (sb *Backend) VerifyPlumoProofs(proofs []types.PlumoProof) error {
	for _, proof := range proofs {
		if err := sb.verifyPlumoProof(proof); err != nil {
			return err
		}
	}
	return nil
	// split := func(buf []byte, lim int) [][]byte {
	// 	var chunk []byte
	// 	chunks := make([][]byte, 0, len(buf)/lim+1)
	// 	for len(buf) >= lim {
	// 		chunk, buf = buf[:lim], buf[lim:]
	// 		chunks = append(chunks, chunk)
	// 	}
	// 	if len(buf) > 0 {
	// 		chunks = append(chunks, buf[:len(buf)])
	// 	}
	// 	return chunks
	// }

	// verificationKey, _ := hex.DecodeString("27de9b5798fd5c832aaabe2b1bb480473df29203a3a1971aed07a21441a6f21573031b01b821546c16f8470dda69131439854051daf5fb6ab7b6815cd9acc0c583ca1376d37343d69fc397323be1d601c7cd43fc42d4361aa27cdf8bc565d3731a93514b0fd38659ebc40f3c44ba9c6882a2e820828135d292d9bb7734afb8094e1e8a9c755420b93155ac4caefe44dd6a64e732daf6e0e141b26c88a30353d627e3a4afec307e285607d18261c04cf8b568eb4fdb3f48da889e8d46c4ff2712fd5664326a9642161463b234dd7b89f1fccb1a6c9acc26b1994c58b68106276345ecf79e0137a941be4bca06489616810b7db4d93fd8b3f6615e84b9683112cbf8e696867656616b5dbd60553df7f92fc1300c0aaf6217b2561b7f10671ae540c209f22a8e2cb98fb60e118aa2c372a44db6a404eb19d7e68ec9d0f0b4521a7f405ca2f421cf4b4128176a34cdf49a9a471dc197e4ee5c8ccb9342fb967173eb3137659930767af5228d1c36038885512286189c358cfc053c1319d4a2f8e50ca32ab13468d86aa059ed201de13f0eae7f4a687baa5d212df991cc5e656200fe705cb8e293614eebc196acc1a5d493d189162e0eeef2c68a37d58cc6717e9b649a595293cd18cc27f8aed9863b0a727b5e12cfa4e90513a282bb65658bd786968f89993ebbc7006f810f0cb6a7002599739fa40f337eaab7e639f79ebdf85ffecf106d5fa8464a197803f8a742a1ef2f05d03c592c97abdd0847c45f5b8878443319618ff04b7a3de13ce778c58c79b1c7f8d6e900d02fa1279dde856c7a62e1b13aff7a42b63bed3c2100318d515c0175c0c067b1ae5b4dc17c3423290c0d5c5e4c6da641073562f5ba99de560671bce41e2aa0abbeb2176b13802e58ba424ce8d790c7d5b610a409e26c718ef1a939198d3a32aef7f5b83347b15fc2b4adfebc468e7bb4f5667a70ea45c3b7135e0efc2e1738ab5b033176f75780fa17415fb5037c748d6be0a6ae68235cd307bf08f890261378aad62ef89395e33004c1e5f50a9fc261fd6a6f175e64110f15af68d3e888f132348039e3fb8db3ac974b8b07867b452bb73584d4c6e7ba5f790c119500b7b538cddcfc81ee754db65aca15f5853c806a40b38e02c0ed2a797fcc11afe2e26f972b4a7f8fe70e626e5d3b4fa600ee730f6a27b9fb448daa9b364ede0feefba1fc74a2d546eb74ac2c61f7d8033e0df3e214d3f12b8bf229692148305e2d82e3ad4c8266b7da641a42c21689e1e738b2689d1bc31afcb6d2b53a87afabd970de2da2e26431a1d4178a6808a23ca803a479e15ea5b7875adc1f4456ce88a3a94a7aecc9763899a353bcc1363a746ab55bd3322d44919eb0a3350c1a8eb574f7220300000000000000a21df711258265911c858a49dc5e866c47f4da8c6f24c4d5002156f719631fcf22af144657300ce576ca5523c69a2cef3e1771a054f51365321a78e2877a8505a02a25300f414035e4f84517141ada5e702376358634fe0b2d5477bfcda04f6ced04908edbd11f470d8e060600031662f9bb513919ab3b21846461a9c70c486adb9a200f16eeb66bf4d550bcfc44b2b29af9338b9c054f04c00eda28301a5d78ad5352fb88ee7623a5554a08a8f886e325a9a5cc4071fbe168860614a0a4e6ffd3aa7c2837fea5669ccd78c6c4c6573d1512ac83ecca8bb2b31d2a7a8870e84763eee2e6f47dc83ab5a78c6a45254c003853a563d59a00bc9cfec4586ef5b5b6e37564304f1ea1ad6c002b0f016863e0af2229a473258e9f1d91926ad75a310c5adead0bbda1")

	// first_epoch_pubkeys := "3dfaa2527c6e3870170898aca8b7285c3dd2f748427d9b0d368e3764917831c82d0e5571819162d721f4b1aeb02f4601b7709ef14786ca43c16fd0416265426734af20e705ad97a255d20176759cb0392d29cfffc21733e4e25e86767324a08153e8b0c43b72bfdac10fcdbcd8438a180d11ae7741b8bf34261652eb61d67022beba80d99a1611536188bf3d1966ac000a6c31c78230a841a97191ccd40aa30a5163bcbf655c3d4984750a31753145e5592b3795087996d80869b2ee1708a60026ce5dc1b1519987439e0791eb34ee50c9cc67919d0010232a2b106d60262189e237f228b731d30adfec4152947b5401b92cdf69ad0a3fd355465b29358aefb8ba99b2742fad5d4a032bac6e378ca4a84dfce4bfdad6a5858f0954876687b3803414bb082c1e925500d85971007e42432a5d34b865fd4fc3f91d514617401a8ec47b9b4e72c4c831886f48a71738c400583b8796cf9cf098dcb40548202946fdea0be1c6806d4ecd7784e23eab19b5dcbfec78de74c994717609f39e3d9c9001"
	// pk1, _ := hex.DecodeString(first_epoch_pubkeys)

	// firstEpochBlock := snark.EpochBlock{
	// 	Index:         0, //uint16(proof.Metadata.FirstEpoch),
	// 	MaxNonSigners: 1,
	// 	PublicKeys:    split(pk1, 96),
	// }

	// last_epoch_pubkeys := "a3fa866f5b51ff226f70a6872aedb89020372b88847a75f1c20dee118a679aa077dd16d1c1d06d079dad2e91808c11002199f32b651a010807264c6a00d034c3d4a31fad24935b2395d2fa80e02b6ac3e1a25e0f202ba99aa6647efe28d68081134bc536f6a172b50ad3a75e633272e5bc53f0a1c9520c584e922dda377eb2b2858ca443769753ba182012e609cc9f01c0df40d4c108fa01e9e4805ccc4947e5ff9dd90ca30bcf4ee258a957118a3b1b1ed8493c595d7598ae4e639147ef4901a84ba4f0cf1beda01fa86c8b3a71f97f657456991a20210ef73f1379156967c822d984e459209961d892c376a67c650056ca5d645a01131836a9382afb936c8ee2a430f9af098df29d776b9b9c2000c3326630a98ba48f4eb610685bb9d66b00ff530875d838c4b68ced77572837a5b335f5056973a41249c6049bab694e1e54bffc3a3c6a940ef69d5029300b7d98009534be42de14dce9d0a74a6ff52ffd36efa050cae9efe47a409bf73f748f795e81a914a9863edad412cf2cfa3b2c9e80"
	// pk2, _ := hex.DecodeString(last_epoch_pubkeys)

	// lastEpochBlock := snark.EpochBlock{
	// 	Index:         2, //uint16(proof.Metadata.LastEpoch),
	// 	MaxNonSigners: 1,
	// 	PublicKeys:    split(pk2, 96),
	// }

	// TODO handle err case?
	// TODO separate into (sb.getValidators) vs. light clients
}

func (sb *Backend) verifyPlumoProof(proof types.PlumoProof) error {
	// verificationKey, _ := hex.DecodeString("27de9b5798fd5c832aaabe2b1bb480473df29203a3a1971aed07a21441a6f21573031b01b821546c16f8470dda69131439854051daf5fb6ab7b6815cd9acc0c583ca1376d37343d69fc397323be1d601c7cd43fc42d4361aa27cdf8bc565d3731a93514b0fd38659ebc40f3c44ba9c6882a2e820828135d292d9bb7734afb8094e1e8a9c755420b93155ac4caefe44dd6a64e732daf6e0e141b26c88a30353d627e3a4afec307e285607d18261c04cf8b568eb4fdb3f48da889e8d46c4ff2712fd5664326a9642161463b234dd7b89f1fccb1a6c9acc26b1994c58b68106276345ecf79e0137a941be4bca06489616810b7db4d93fd8b3f6615e84b9683112cbf8e696867656616b5dbd60553df7f92fc1300c0aaf6217b2561b7f10671ae540c209f22a8e2cb98fb60e118aa2c372a44db6a404eb19d7e68ec9d0f0b4521a7f405ca2f421cf4b4128176a34cdf49a9a471dc197e4ee5c8ccb9342fb967173eb3137659930767af5228d1c36038885512286189c358cfc053c1319d4a2f8e50ca32ab13468d86aa059ed201de13f0eae7f4a687baa5d212df991cc5e656200fe705cb8e293614eebc196acc1a5d493d189162e0eeef2c68a37d58cc6717e9b649a595293cd18cc27f8aed9863b0a727b5e12cfa4e90513a282bb65658bd786968f89993ebbc7006f810f0cb6a7002599739fa40f337eaab7e639f79ebdf85ffecf106d5fa8464a197803f8a742a1ef2f05d03c592c97abdd0847c45f5b8878443319618ff04b7a3de13ce778c58c79b1c7f8d6e900d02fa1279dde856c7a62e1b13aff7a42b63bed3c2100318d515c0175c0c067b1ae5b4dc17c3423290c0d5c5e4c6da641073562f5ba99de560671bce41e2aa0abbeb2176b13802e58ba424ce8d790c7d5b610a409e26c718ef1a939198d3a32aef7f5b83347b15fc2b4adfebc468e7bb4f5667a70ea45c3b7135e0efc2e1738ab5b033176f75780fa17415fb5037c748d6be0a6ae68235cd307bf08f890261378aad62ef89395e33004c1e5f50a9fc261fd6a6f175e64110f15af68d3e888f132348039e3fb8db3ac974b8b07867b452bb73584d4c6e7ba5f790c119500b7b538cddcfc81ee754db65aca15f5853c806a40b38e02c0ed2a797fcc11afe2e26f972b4a7f8fe70e626e5d3b4fa600ee730f6a27b9fb448daa9b364ede0feefba1fc74a2d546eb74ac2c61f7d8033e0df3e214d3f12b8bf229692148305e2d82e3ad4c8266b7da641a42c21689e1e738b2689d1bc31afcb6d2b53a87afabd970de2da2e26431a1d4178a6808a23ca803a479e15ea5b7875adc1f4456ce88a3a94a7aecc9763899a353bcc1363a746ab55bd3322d44919eb0a3350c1a8eb574f7220300000000000000a21df711258265911c858a49dc5e866c47f4da8c6f24c4d5002156f719631fcf22af144657300ce576ca5523c69a2cef3e1771a054f51365321a78e2877a8505a02a25300f414035e4f84517141ada5e702376358634fe0b2d5477bfcda04f6ced04908edbd11f470d8e060600031662f9bb513919ab3b21846461a9c70c486adb9a200f16eeb66bf4d550bcfc44b2b29af9338b9c054f04c00eda28301a5d78ad5352fb88ee7623a5554a08a8f886e325a9a5cc4071fbe168860614a0a4e6ffd3aa7c2837fea5669ccd78c6c4c6573d1512ac83ecca8bb2b31d2a7a8870e84763eee2e6f47dc83ab5a78c6a45254c003853a563d59a00bc9cfec4586ef5b5b6e37564304f1ea1ad6c002b0f016863e0af2229a473258e9f1d91926ad75a310c5adead0bbda1")
	// verificationKey, _ := hex.DecodeString("701d85323653276aec48c9eafe688dec2515eb92c088566bcdc4832bd1739b6690de38826279fe26460139e12b41bfd98c731955fabcfce6d41ada5033c0ace5cf9e99bc948627f0a09650e1af88e2568450dbd11ff07f557094b8a06d522400fa3abdfaf700fd2800f94a68a8c22aed9d034fe1c63a09e711c343f07c08e64a2eee7c4d99519ea987cddc097aea58477a6afa5f07deb32a9af2cf8ac886ace98407ab8dea4d1731d586fda37528437e0cd58b0e9c0395d11180663f195475802a26949dcf9e4b5e9f5ced3dad5b527ab11d357ccec4424379990b36545816713bc3002533a831a9d54a55c06be737d3509a86661d1e4cf736e59c4675da9548e9de0d05dd651a3f56a63d265551e16730453fd0f9ab523993521eb969eacc000b1a0530e94ac0f284dbceaa8be9985f47bdee849ed21433da7a358106af34e9629fdaa80f2a454f88857aba9278e904d105ca04ea633bf36453b30b92814e4a3ddb8ed1f3d1fe894358311e6b07aaa7bac542cb6de9c31c680849e060f32c000300000000000000705784c0f7a78fbef563817aa455cbc41cd1605c06f71e458e560672c266664efc2176f91afc4e5aaa971d2e87271ce7677ab9e43455a34bfb191fbc2fd12b92a6be55ca68b3b32dc024990c9da435d186350539e9b54e4aaf2ae3980ae31b8059f0e1aefa5b1f8f12d9645ad60f1262f8bfa4d299b1b99bde20d5251e920c68aa54f75ec7dc9694f4171c1cd27f3912c563db1a5413012e05bb3e055a46245a2f5b93b9352d2264a2572a33376dd9ec4a0e59c829ac09eacb66cc14b5596b00c4f772c9a97fed5ce51d858d06cc394c1fdb57b325fe383650689bc74b6eb804ee330f87cc021314911d0274d1c49604e84c7b8d1c584c46cf85e3fa0d1a36a2d035b53a8636f4971717531b5fde18a3dc1a5a0bce140e464a38f22fa7972880")
	// verificationKey, _ := hex.DecodeString("a548b97fa96ffc5da4af019dc65c57016002b8d388fca6f68b9274f5344705fb5f3ecda9ebdd4e3dfad1227e5df2f79b910ddabb264a060b8aa8340d37f44572b5a995a1b3365e70452f7e6f3f1b56b5a18c55b39b9a57edbef7f0f0666cda0000041f30136b2f8b3f52dd5329ad2e02d324e3d5e0fa79a8714c12039b7bfbfeba0eda2a36a16885f567b3d82243bfe5d14d409275b57d9d42ae24fa25f4e0b9345fa64a5184849f827b22baa7c0288d26e33f0bfcd518804645f99181dcea80cb336cb3318c065ef3b754eee8dfac8cf9c6604f42c13589a7169881f591f71c658b47b58c02b44b4abef3e333db172bd2c6512b28928d1bd4b0813ab96923d7fc670dce3fe19d64c3a3dca872f2f287cd07ce6b52b672b9f8a5732925090101a9872d4d7d730e225da9b525a4c3d250e97c33aa5957a7cb4728739051b5d9e52cf5f0a1a0f86fc1892ce270095a0ce003dcf33ba0e9c280e99139deeedfc60db720c6087ce7d7184bfb2edfa95b3d59cdf257fdb8f676d711becc1fbc91b4800300000000000000124eac58d55d41fce4211aba1a24665a611dc36c57a338cd05e7df722570c048b7135fc5c9800656219b03d75adb77b4422593ae46108ba32df4c58644588ba05ada2cdb75028214246fe57bf2e1d9b3f1ff32cdf15b2f96ad41bc5fad8a50808997cc4a4d2e43029d3a1b945bc87b110da4eac406f1e20f48e8db7d374747deb25bfa45d1b67c079aae91949037ed83b276af1e9e9b6395459d62122effeaf5b3f26a0c1b1d7e9964d6b28c8c9f45f8ee440e1af66ae5ddde1c5149dcb26c807abc36bc2e497f439c81b1e396294003ec225838ca925883ba3f74874d548bffb8b47ee8739af0b8bb12c0b5d57c8b86dd9d6f33fb5067994a6a8f57f7a2a4b77ef6be3657be3205545362b5ac2eb299127844195f797937444e2633eb95ca00")
	verificationKey, _ := hex.DecodeString("944a99f744e12f5cdf28b0e7a19e00fe60600a6680a6423171128c4b7f75f6bfd6b49c9197eb03e7e0a57f2dad2b5898d9f61d580adc26bf9b616bb5d67ca32c224ec849b9fa49247b31a4c25940477d440de37ffe8d9bda48eaf56fbe884e80233abb1e1c4b090832625460f14f00c6175da2493079c5ebc92d52a09f538442e0a7472e028a9947b9baa44b7ec09c43e50880ae51df7f797cd3d683c6ee466496837b8687596c43d907a87eb0bcc5734ba26ddd1191373ceada8df887171200074d198e25c9fae6544a6812366c0fb9d0a0865a7b4f6c90ae300c049bd5ec2278890e4d8f494ddd2b0c559c2a60612d981074bd84c29f60fb224172adc60e6113c09a144578c456b9d8e37a87504b5565315906595ccc9330858fcbf59820811a2f45661bb05c6ffffdfffa153cd202ab5dbf19057cefba9198a8eabeeb347217b3d882f8eb90efad01a837f35e3f49c1fd571c15d85dbd6072ffce3b2836784f36c596aed9511b8b3d5e2754668a5206d131642fbdae55cc685ce91ffb5b800300000000000000211c999ecd73a9ebf0feab7f15e1b51462dcb111d9c9c3e67938095623f1e5a35644115d964aec176d74800fb0d80ac4fdd461bb841dd136ba5859889a2f07e0bf055434004bd0c73e5e87d3a1755a5c3ceaac89f0058f76c32e4acca8ffc10026ad7185527b077096d160630d8c418317a78c75a582cdf9ebe9b72365fea3135fce6bf929de11c8f2e38f02baa88fa69509ef1f91bcecba13b2b8eb70ca4bef3339a2e7dbe8d13ea58a822888be18871fb79947c7d0d970326721ac3089668039db236f4433bd957fc2c9b65d0566a316982f1fa046bdc29090b84b2fb62dd9d07b2576428b04f797a0b4cc1b22e09c9826b13d82e9dfd5f9b280a9a1f5034c365d6911bada9dfb16e5e6791cefd484ae20883be201a06315f08445fdf06600")
	firstValSet := sb.getValidators(uint64(proof.Metadata.FirstEpoch), common.Hash{})
	firstSerializedValSet := istanbul.MapValidatorsToPublicKeys(firstValSet.List())
	var firstSerializedValSetBytes [][]byte
	for _, pubKey := range firstSerializedValSet {
		firstSerializedValSetBytes = append(firstSerializedValSetBytes, pubKey[:])
	}
	firstEpochBlockNumber := istanbul.GetEpochLastBlockNumber(uint64(proof.Metadata.FirstEpoch), sb.config.Epoch)
	firstBlockHeader := sb.chain.GetHeaderByNumber(firstEpochBlockNumber)
	epochEntropy := blscrypto.EpochEntropyFromHash(firstBlockHeader.Hash())
	parentEpochBlockNumber := istanbul.GetEpochLastBlockNumber(uint64(proof.Metadata.FirstEpoch)-1, sb.config.Epoch)
	parentBlockHeader := sb.chain.GetHeaderByNumber(parentEpochBlockNumber)
	parentEpochEntropy := blscrypto.EpochEntropyFromHash(parentBlockHeader.Hash())
	firstEpochBlock := snark.EpochBlock{
		Index:         uint16(proof.Metadata.FirstEpoch),
		MaxNonSigners: uint32(firstValSet.Size() - firstValSet.MinQuorumSize()),
		MaxValidators: 1,
		Round:         0,
		PublicKeys:    firstSerializedValSetBytes,
		EpochEntropy:  epochEntropy[:],
		ParentEntropy: parentEpochEntropy[:],
	}
	log.Error("First epoch block", "first", firstEpochBlock)

	lastValSet := sb.getValidators(uint64(proof.Metadata.LastEpoch), common.Hash{})
	lastSerializedValSet := istanbul.MapValidatorsToPublicKeys(lastValSet.List())
	var lastSerializedValSetBytes [][]byte
	for _, pubKey := range lastSerializedValSet {
		lastSerializedValSetBytes = append(lastSerializedValSetBytes, pubKey[:])
	}
	lastEpochBlockNumber := istanbul.GetEpochLastBlockNumber(uint64(proof.Metadata.LastEpoch), sb.config.Epoch)
	lastBlockHeader := sb.chain.GetHeaderByNumber(lastEpochBlockNumber)
	lastEpochEntropy := blscrypto.EpochEntropyFromHash(lastBlockHeader.Hash())
	lastEpochParentBlockNumber := istanbul.GetEpochLastBlockNumber(uint64(proof.Metadata.LastEpoch)-1, sb.config.Epoch)
	lastParentBlockHeader := sb.chain.GetHeaderByNumber(lastEpochParentBlockNumber)
	lastParentEpochEntropy := blscrypto.EpochEntropyFromHash(lastParentBlockHeader.Hash())
	lastEpochBlock := snark.EpochBlock{
		Index:         uint16(proof.Metadata.LastEpoch),
		MaxNonSigners: uint32(lastValSet.Size() - lastValSet.MinQuorumSize()),
		MaxValidators: 1,
		Round:         0,
		PublicKeys:    lastSerializedValSetBytes,
		EpochEntropy:  lastEpochEntropy[:],
		ParentEntropy: lastParentEpochEntropy[:],
	}
	err := snark.VerifyEpochs(
		verificationKey,
		proof.Proof,
		firstEpochBlock,
		lastEpochBlock,
	)
	if err != nil {
		log.Error("Proof verification error")
		return err
	}
	log.Error("Proof verification SUCCEEDED")
	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
// The parent seal is not included when the node is not validating.
func (sb *Backend) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	// copy the parent extra data as the header extra data
	number := header.Number.Uint64()
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	// set header's timestamp
	header.Time = parent.Time + sb.config.BlockPeriod
	nowTime := uint64(now().Unix())
	if header.Time < nowTime {
		header.Time = nowTime
	}

	// Record what the delay should be, but sleep in the miner, not the consensus engine.
	delay := time.Until(time.Unix(int64(header.Time), 0))
	if delay < 0 {
		sb.sleepGauge.Update(0)
	} else {
		sb.sleepGauge.Update(delay.Nanoseconds())
	}

	if err := writeEmptyIstanbulExtra(header); err != nil {
		return err
	}

	// addParentSeal blocks for up to 500ms waiting for the core to reach the target sequence.
	// Prepare is called from non-validators, so don't bother with the parent seal unless this
	// block is to be proposed instead of for the local state.
	if sb.IsValidating() {
		return sb.addParentSeal(chain, header)
	} else {
		return nil
	}
}

// UpdateValSetDiff will update the validator set diff in the header, if the mined header is the last block of the epoch
func (sb *Backend) UpdateValSetDiff(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB) error {
	// If this is the last block of the epoch, then get the validator set diff, to save into the header
	log.Trace("Called UpdateValSetDiff", "number", header.Number.Uint64(), "epoch", sb.config.Epoch)
	if istanbul.IsLastBlockOfEpoch(header.Number.Uint64(), sb.config.Epoch) {
		newValSet, err := sb.getNewValidatorSet(header, state)
		if err == nil {
			// Get the last epoch's validator set
			snap, err := sb.snapshot(chain, header.Number.Uint64()-1, header.ParentHash, nil)
			if err != nil {
				return err
			}

			// add validators in snapshot to extraData's validators section
			return writeValidatorSetDiff(header, snap.validators(), newValSet)
		}
	}
	// If it's not the last block or we were unable to pull the new validator set, then the validator set diff should be empty
	return writeValidatorSetDiff(header, []istanbul.ValidatorData{}, []istanbul.ValidatorData{})
}

// IsLastBlockOfEpoch returns whether or not a particular header represents the last block in the epoch.
func (sb *Backend) IsLastBlockOfEpoch(header *types.Header) bool {
	return istanbul.IsLastBlockOfEpoch(header.Number.Uint64(), sb.config.Epoch)
}

// EpochSize returns the size of epochs in blocks.
func (sb *Backend) EpochSize() uint64 {
	return sb.config.Epoch
}

// LookbackWindow returns the size of the lookback window for calculating uptime (in blocks)
// Value is constant during an epoch
func (sb *Backend) LookbackWindow(header *types.Header, state *state.StateDB) uint64 {
	// Check if donut was already active at the beginning of the epoch
	// as we want to activate the change at epoch change
	firstBlockOfEpoch := istanbul.MustGetEpochFirstBlockGivenBlockNumber(header.Number.Uint64(), sb.config.Epoch)
	cip21Activated := sb.chain.Config().IsDonut(new(big.Int).SetUint64(firstBlockOfEpoch))

	vmRunner := sb.chain.NewEVMRunner(header, state)
	return uptime.ComputeLookbackWindow(
		sb.config.Epoch,
		sb.config.DefaultLookbackWindow,
		cip21Activated,
		func() (uint64, error) { return blockchain_parameters.GetLookbackWindow(vmRunner) },
	)
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// but does not assemble the block.
//
// Note: The block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (sb *Backend) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction) {
	start := time.Now()
	defer sb.finalizationTimer.UpdateSince(start)

	logger := sb.logger.New("func", "Finalize", "block", header.Number.Uint64(), "epochSize", sb.config.Epoch)
	logger.Trace("Finalizing")

	// The contract calls in Finalize() may emit logs, which we later add to an extra "block" receipt
	// (in FinalizeAndAssemble() during construction or in `StateProcessor.process()` during verification).
	// They are looked up using the zero hash instead of a transaction hash, and so we need to first call
	// `state.Prepare()` so that they get filed under the zero hash. Otherwise, they would get filed under
	// the hash of the last transaction in the block (if there were any).
	state.Prepare(common.Hash{}, header.Hash(), len(txs))

	snapshot := state.Snapshot()
	vmRunner := sb.chain.NewEVMRunner(header, state)
	err := sb.setInitialGoldTokenTotalSupplyIfUnset(vmRunner)
	if err != nil {
		state.RevertToSnapshot(snapshot)
	}

	// Trigger an update to the gas price minimum in the GasPriceMinimum contract based on block congestion
	snapshot = state.Snapshot()
	_, err = gpm.UpdateGasPriceMinimum(vmRunner, header.GasUsed)
	if err != nil {
		state.RevertToSnapshot(snapshot)
	}

	lastBlockOfEpoch := istanbul.IsLastBlockOfEpoch(header.Number.Uint64(), sb.config.Epoch)
	if lastBlockOfEpoch {
		snapshot = state.Snapshot()
		err = sb.distributeEpochRewards(header, state)
		if err != nil {
			sb.logger.Error("Failed to distribute epoch rewards", "blockNumber", header.Number, "err", err)
			state.RevertToSnapshot(snapshot)
		}
	}

	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	logger.Debug("Finalized", "duration", now().Sub(start), "lastInEpoch", lastBlockOfEpoch)
}

// FinalizeAndAssemble runs any post-transaction state modifications (e.g. block
// rewards) and assembles the final block.
//
// Note: The block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (sb *Backend) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, receipts []*types.Receipt, randomness *types.Randomness) (*types.Block, error) {

	sb.Finalize(chain, header, state, txs)
	// Add the block receipt with logs from the non-transaction core contract calls (if there were any)
	receipts = core.AddBlockReceipt(receipts, state, header.Hash())

	// Assemble and return the final block for sealing
	block := types.NewBlock(header, txs, receipts, randomness)
	return block, nil
}

// checkIsValidSigner checks if validator is a valid signer for the block
// returns an error if not
func (sb *Backend) checkIsValidSigner(chain consensus.ChainHeaderReader, header *types.Header) error {
	snap, err := sb.snapshot(chain, header.Number.Uint64()-1, header.ParentHash, nil)
	if err != nil {
		return err
	}

	_, v := snap.ValSet.GetByAddress(sb.wallets().Ecdsa.Address)
	if v == nil {
		return errUnauthorized
	}
	return nil
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top and submits it the the consensus engine.
func (sb *Backend) Seal(chain consensus.ChainHeaderReader, block *types.Block) error {

	header := block.Header()

	// Bail out if we're unauthorized to sign a block
	if err := sb.checkIsValidSigner(chain, header); err != nil {
		return err
	}

	if parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1); parent == nil {
		return consensus.ErrUnknownAncestor
	}

	// update the block header timestamp and signature and propose the block to core engine
	block, err := sb.signBlock(block)
	if err != nil {
		return err
	}

	// post block into Istanbul engine
	if err := sb.EventMux().Post(istanbul.RequestEvent{Proposal: block}); err != nil {
		return err
	}

	return nil
}

// signBlock signs block with a seal
func (sb *Backend) signBlock(block *types.Block) (*types.Block, error) {
	header := block.Header()
	// sign the hash
	seal, err := sb.Sign(sigHash(header).Bytes())
	if err != nil {
		return nil, err
	}

	err = writeSeal(header, seal)
	if err != nil {
		return nil, err
	}

	return block.WithHeader(header), nil
}

// APIs returns the RPC APIs this consensus engine provides.
func (sb *Backend) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	return []rpc.API{{
		Namespace: "istanbul",
		Version:   "1.0",
		Service:   &API{chain: chain, istanbul: sb},
		Public:    true,
	}}
}

func (sb *Backend) SetChain(chain consensus.ChainContext, currentBlock func() *types.Block, stateAt func(common.Hash) (*state.StateDB, error)) {
	sb.chain = chain
	sb.currentBlock = currentBlock
	sb.stateAt = stateAt

	if bc, ok := chain.(*ethCore.BlockChain); ok {
		go sb.newChainHeadLoop(bc)
		go sb.updateReplicaStateLoop(bc)
	}

}

// Loop to run on new chain head events. Chain head events may be batched.
func (sb *Backend) newChainHeadLoop(bc *ethCore.BlockChain) {
	// Batched. For stats & announce
	chainHeadCh := make(chan ethCore.ChainHeadEvent, 10)
	chainHeadSub := bc.SubscribeChainHeadEvent(chainHeadCh)
	defer chainHeadSub.Unsubscribe()

	for {
		select {
		case chainHeadEvent := <-chainHeadCh:
			sb.newChainHead(chainHeadEvent.Block)
		case err := <-chainHeadSub.Err():
			log.Error("Error in istanbul's subscription to the blockchain's chainhead event", "err", err)
			return
		}
	}
}

// Loop to update replica state. Listens to chain events to avoid batching.
func (sb *Backend) updateReplicaStateLoop(bc *ethCore.BlockChain) {
	// Unbatched event listener
	chainEventCh := make(chan ethCore.ChainEvent, 10)
	chainEventSub := bc.SubscribeChainEvent(chainEventCh)
	defer chainEventSub.Unsubscribe()

	for {
		select {
		case chainEvent := <-chainEventCh:
			sb.coreMu.RLock()
			if !sb.coreStarted && sb.replicaState != nil {
				consensusBlock := new(big.Int).Add(chainEvent.Block.Number(), common.Big1)
				sb.replicaState.NewChainHead(consensusBlock)
			}
			sb.coreMu.RUnlock()
		case err := <-chainEventSub.Err():
			log.Error("Error in istanbul's subscription to the blockchain's chain event", "err", err)
			return
		}
	}
}

// SetCallBacks implements consensus.Istanbul.SetCallBacks
func (sb *Backend) SetCallBacks(hasBadBlock func(common.Hash) bool,
	processBlock func(*types.Block, *state.StateDB) (types.Receipts, []*types.Log, uint64, error),
	validateState func(*types.Block, *state.StateDB, types.Receipts, uint64) error,
	onNewConsensusBlock func(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB)) error {
	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if sb.coreStarted {
		return istanbul.ErrStartedEngine
	}

	sb.hasBadBlock = hasBadBlock
	sb.processBlock = processBlock
	sb.validateState = validateState
	sb.onNewConsensusBlock = onNewConsensusBlock
	return nil
}

// StartValidating implements consensus.Istanbul.StartValidating
func (sb *Backend) StartValidating() error {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if sb.coreStarted {
		return istanbul.ErrStartedEngine
	}

	if sb.hasBadBlock == nil || sb.processBlock == nil || sb.validateState == nil {
		return errors.New("Must SetCallBacks prior to StartValidating")
	}

	sb.logger.Info("Starting istanbul.Engine validating")
	if err := sb.core.Start(); err != nil {
		return err
	}

	// Having coreStarted as false at this point guarantees that announce versions
	// will be updated by the time announce messages in the announceThread begin
	// being generated
	if !sb.IsProxiedValidator() {
		sb.UpdateAnnounceVersion()
	}

	sb.coreStarted = true

	// coreStarted must be true by this point for validator peers to be successfully added
	if !sb.config.Proxied {
		if err := sb.RefreshValPeers(); err != nil {
			sb.logger.Warn("Error refreshing validator peers", "err", err)
		}
	}

	return nil
}

// StopValidating implements consensus.Istanbul.StopValidating
func (sb *Backend) StopValidating() error {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if !sb.coreStarted {
		return istanbul.ErrStoppedEngine
	}
	sb.logger.Info("Stopping istanbul.Engine validating")
	if err := sb.core.Stop(); err != nil {
		return err
	}
	sb.coreStarted = false

	return nil
}

// StartAnnouncing implements consensus.Istanbul.StartAnnouncing
func (sb *Backend) StartAnnouncing() error {
	sb.announceMu.Lock()
	defer sb.announceMu.Unlock()
	if sb.announceRunning {
		return istanbul.ErrStartedAnnounce
	}

	sb.announceThreadQuit = make(chan struct{})
	sb.announceRunning = true

	sb.announceThreadWg.Add(1)
	go sb.announceThread()

	if err := sb.vph.startThread(); err != nil {
		sb.StopAnnouncing()
		return err
	}

	return nil
}

// StopAnnouncing implements consensus.Istanbul.StopAnnouncing
func (sb *Backend) StopAnnouncing() error {
	sb.announceMu.Lock()
	defer sb.announceMu.Unlock()

	if !sb.announceRunning {
		return istanbul.ErrStoppedAnnounce
	}

	close(sb.announceThreadQuit)
	sb.announceThreadWg.Wait()

	sb.announceRunning = false

	return sb.vph.stopThread()
}

// StartProxiedValidatorEngine implements consensus.Istanbul.StartProxiedValidatorEngine
func (sb *Backend) StartProxiedValidatorEngine() error {
	sb.proxiedValidatorEngineMu.Lock()
	defer sb.proxiedValidatorEngineMu.Unlock()

	if sb.proxiedValidatorEngineRunning {
		return istanbul.ErrStartedProxiedValidatorEngine
	}

	if !sb.config.Proxied {
		return istanbul.ErrValidatorNotProxied
	}

	sb.proxiedValidatorEngine.Start()
	sb.proxiedValidatorEngineRunning = true

	return nil
}

// StopProxiedValidatorEngine implements consensus.Istanbul.StopProxiedValidatorEngine
func (sb *Backend) StopProxiedValidatorEngine() error {
	sb.proxiedValidatorEngineMu.Lock()
	defer sb.proxiedValidatorEngineMu.Unlock()

	if !sb.proxiedValidatorEngineRunning {
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	sb.proxiedValidatorEngine.Stop()
	sb.proxiedValidatorEngineRunning = false

	return nil
}

// MakeReplica clears the start/stop state & stops this node from participating in consensus
func (sb *Backend) MakeReplica() error {
	if sb.replicaState != nil {
		return sb.replicaState.MakeReplica()
	}
	return istanbul.ErrUnauthorizedAddress
}

// MakePrimary clears the start/stop state & makes this node participate in consensus
func (sb *Backend) MakePrimary() error {
	if sb.replicaState != nil {
		return sb.replicaState.MakePrimary()
	}
	return istanbul.ErrUnauthorizedAddress
}

// snapshot retrieves the validator set needed to sign off on the block immediately after 'number'.  E.g. if you need to find the validator set that needs to sign off on block 6,
// this method should be called with number set to 5.
//
// hash - The requested snapshot's block's hash. Only used for snapshot cache storage.
// number - The requested snapshot's block number
// parents - (Optional argument) An array of headers from directly previous blocks.
func (sb *Backend) snapshot(chain consensus.ChainHeaderReader, number uint64, hash common.Hash, parents []*types.Header) (*Snapshot, error) {
	// Search for a snapshot in memory or on disk
	var (
		headers []*types.Header
		header  *types.Header
		snap    *Snapshot
	)

	numberIter := number

	// If numberIter is not the last block of an epoch, then adjust it to be the last block of the previous epoch
	if !istanbul.IsLastBlockOfEpoch(numberIter, sb.config.Epoch) {
		epochNum := istanbul.GetEpochNumber(numberIter, sb.config.Epoch)
		numberIter = istanbul.GetEpochLastBlockNumber(epochNum-1, sb.config.Epoch)
	}

	// At this point, numberIter will always be the last block number of an epoch.  Namely, it will be
	// block numbers where the header contains the validator set diff.
	// Note that block 0 (the genesis block) is one of those headers.  It contains the initial set of validators in the
	// 'addedValidators' field in the header.

	// Retrieve the most recent cached or on disk snapshot.
	for ; ; numberIter = numberIter - sb.config.Epoch {
		// If an in-memory snapshot was found, use that
		if s, ok := sb.recentSnapshots.Get(numberIter); ok {
			snap = s.(*Snapshot)
			break
		}

		var blockHash common.Hash
		if numberIter == number && hash != (common.Hash{}) {
			blockHash = hash
		} else {
			header = chain.GetHeaderByNumber(numberIter)
			if header == nil {
				log.Trace("Unable to find header in chain", "number", number)
			} else {
				blockHash = chain.GetHeaderByNumber(numberIter).Hash()
			}
		}

		if (blockHash != common.Hash{}) {
			if s, err := loadSnapshot(sb.config.Epoch, sb.db, blockHash); err == nil {
				log.Trace("Loaded validator set snapshot from disk", "number", numberIter, "hash", blockHash)
				snap = s
				sb.recentSnapshots.Add(numberIter, snap)
				break
			}
		}

		if numberIter == 0 {
			break
		}

		// Panic if numberIter underflows (becomes greater than number).
		if numberIter > number {
			panic(fmt.Sprintf("There is a bug in the code.  NumberIter underflowed, and should of stopped at 0.  NumberIter: %v, number: %v", numberIter, number))
		}
	}

	// If snapshot is still nil, then create a snapshot from genesis block
	if snap == nil {
		log.Debug("Snapshot is nil, creating from genesis")
		// Panic if the numberIter does not equal 0
		if numberIter != 0 {
			panic(fmt.Sprintf("There is a bug in the code.  NumberIter should be 0.  NumberIter: %v", numberIter))
		}

		genesis := chain.GetHeaderByNumber(0)
		if genesis == nil {
			log.Error("Cannot load genesis")
			return nil, errors.New("Cannot load genesis")
		}

		istanbulExtra, err := types.ExtractIstanbulExtra(genesis)
		if err != nil {
			log.Error("Unable to extract istanbul extra", "err", err)
			return nil, err
		}

		// The genesis block should have an empty RemovedValidators set.  If not, throw an error
		if istanbulExtra.RemovedValidators.BitLen() != 0 {
			log.Error("Genesis block has a non empty RemovedValidators set")
			return nil, errInvalidValidatorSetDiff
		}

		validators, err := istanbul.CombineIstanbulExtraToValidatorData(istanbulExtra.AddedValidators, istanbulExtra.AddedValidatorsPublicKeys)
		if err != nil {
			log.Error("Cannot construct validators data from istanbul extra")
			return nil, errInvalidValidatorSetDiff
		}
		snap = newSnapshot(sb.config.Epoch, 0, genesis.Hash(), validator.NewSet(validators))

		if err := snap.store(sb.db); err != nil {
			log.Error("Unable to store snapshot", "err", err)
			return nil, err
		}
	}

	log.Trace("Most recent snapshot found", "number", numberIter)
	// Calculate the returned snapshot by applying epoch headers' val set diffs to the intermediate snapshot (the one that is retrieved/created from above).
	// This will involve retrieving all of those headers into an array, and then call snapshot.apply on that array and the intermediate snapshot.
	// Note that the callee of this method may have passed in a set of previous headers, so we may be able to use some of them.
	for numberIter+sb.config.Epoch <= number {
		numberIter += sb.config.Epoch

		log.Trace("Retrieving ancestor header", "number", number, "numberIter", numberIter, "parents size", len(parents))
		inParents := -1
		for i := len(parents) - 1; i >= 0; i-- {
			if parents[i].Number.Uint64() == numberIter {
				inParents = i
				break
			}
		}
		if inParents >= 0 {
			header = parents[inParents]
			log.Trace("Retrieved header from parents param", "header num", header.Number.Uint64())
		} else {
			header = chain.GetHeaderByNumber(numberIter)
			if header == nil {
				// TODO resolve this better
				// return nil, errUnknownBlock
				break
			}
		}

		headers = append(headers, header)
	}

	if len(headers) > 0 {
		var err error
		log.Trace("Snapshot headers len greater than 0", "headers", headers)
		snap, err = snap.apply(headers, sb.db)
		if err != nil {
			log.Error("Unable to apply headers to snapshots", "headers", headers)
			return nil, err
		}

		sb.recentSnapshots.Add(numberIter, snap)
	}
	// Make a copy of the snapshot to return, since a few fields will be modified.
	// The original snap is probably stored within the LRU cache, so we don't want to
	// modify that one.
	returnSnap := snap.copy()

	returnSnap.Number = number
	returnSnap.Hash = hash

	return returnSnap, nil
}

func (sb *Backend) addParentSeal(chain consensus.ChainHeaderReader, header *types.Header) error {
	number := header.Number.Uint64()
	logger := sb.logger.New("func", "addParentSeal", "number", number)

	// only do this for blocks which start with block 1 as a parent
	if number <= 1 {
		return nil
	}

	// Get parent's extra to fetch it's AggregatedSeal
	parent := chain.GetHeader(header.ParentHash, number-1)
	parentExtra, err := types.ExtractIstanbulExtra(parent)
	if err != nil {
		return err
	}

	createParentSeal := func() types.IstanbulAggregatedSeal {
		// In some cases, "addParentSeal" may be called before sb.core has moved to the next sequence,
		// preventing signature aggregation.
		// This typically happens in round > 0, since round 0 typically hits the "time.Sleep()"
		// above.
		// When this happens, loop until sb.core moves to the next sequence, with a limit of 500ms.
		seq := waitCoreToReachSequence(sb.core, header.Number)
		if seq == nil {
			return parentExtra.AggregatedSeal
		}

		logger = logger.New("parentAggregatedSeal", parentExtra.AggregatedSeal.String(), "cur_seq", seq)

		parentCommits := sb.core.ParentCommits()
		if parentCommits == nil || parentCommits.Size() == 0 {
			logger.Debug("No additional seals to combine with ParentAggregatedSeal")
			return parentExtra.AggregatedSeal
		}

		logger = logger.New("numParentCommits", parentCommits.Size())
		logger.Trace("Found commit messages from previous sequence to combine with ParentAggregatedSeal")

		// if we had any seals gossiped to us, proceed to add them to the
		// already aggregated signature
		unionAggregatedSeal, err := istanbulCore.UnionOfSeals(parentExtra.AggregatedSeal, parentCommits)
		if err != nil {
			logger.Error("Failed to combine commit messages with ParentAggregatedSeal", "err", err)
			return parentExtra.AggregatedSeal
		}

		// need to pass the previous block from the parent to get the parent's validators
		// (otherwise we'd be getting the validators for the current block)
		parentValidators := sb.getValidators(parent.Number.Uint64()-1, parent.ParentHash)
		// only update to use the union if we indeed provided a valid aggregate signature for this block
		if err := sb.verifyAggregatedSeal(parent.Hash(), parentValidators, unionAggregatedSeal); err != nil {
			logger.Error("Failed to verify combined ParentAggregatedSeal", "err", err)
			return parentExtra.AggregatedSeal
		}

		logger.Debug("Succeeded in verifying combined ParentAggregatedSeal", "combinedParentAggregatedSeal", unionAggregatedSeal.String())
		return unionAggregatedSeal
	}

	return writeAggregatedSeal(header, createParentSeal(), true)
}

// SetStartValidatingBlock sets block that the validator will start validating on (inclusive)
func (sb *Backend) SetStartValidatingBlock(blockNumber *big.Int) error {
	if sb.replicaState == nil {
		return errNotAValidator
	}
	if blockNumber.Cmp(sb.currentBlock().Number()) < 0 {
		return errors.New("blockNumber should be greater than the current block number")
	}
	return sb.replicaState.SetStartValidatingBlock(blockNumber)
}

// SetStopValidatingBlock sets the block that the validator will stop just before (exclusive range)
func (sb *Backend) SetStopValidatingBlock(blockNumber *big.Int) error {
	if sb.replicaState == nil {
		return errNotAValidator
	}
	if blockNumber.Cmp(sb.currentBlock().Number()) < 0 {
		return errors.New("blockNumber should be greater than the current block number")
	}
	return sb.replicaState.SetStopValidatingBlock(blockNumber)
}

// FIXME: Need to update this for Istanbul
// sigHash returns the hash which is used as input for the Istanbul
// signing. It is the hash of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
//
// Note, the method requires the extra data to be at least 65 bytes, otherwise it
// panics. This is done to avoid accidentally using both forms (signature present
// or not), which could be abused to produce different hashes for the same header.
func sigHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()

	// Clean seal is required for calculating proposer seal.
	rlp.Encode(hasher, types.IstanbulFilteredHeader(header, false))
	hasher.Sum(hash[:0])
	return hash
}

// ecrecover extracts the Ethereum account address from a signed header.
func ecrecover(header *types.Header) (common.Address, error) {
	hash := header.Hash()
	if addr, ok := recentAddresses.Get(hash); ok {
		return addr.(common.Address), nil
	}

	// Retrieve the signature from the header extra-data
	istanbulExtra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		return common.Address{}, err
	}

	addr, err := istanbul.GetSignatureAddress(sigHash(header).Bytes(), istanbulExtra.Seal)
	if err != nil {
		return addr, err
	}
	recentAddresses.Add(hash, addr)
	return addr, nil
}

func writeEmptyIstanbulExtra(header *types.Header) error {
	extra := types.IstanbulExtra{
		AddedValidators:           []common.Address{},
		AddedValidatorsPublicKeys: []blscrypto.SerializedPublicKey{},
		RemovedValidators:         big.NewInt(0),
		Seal:                      []byte{},
		AggregatedSeal:            types.IstanbulAggregatedSeal{},
		ParentAggregatedSeal:      types.IstanbulAggregatedSeal{},
	}
	payload, err := rlp.EncodeToBytes(&extra)
	if err != nil {
		return err
	}

	if len(header.Extra) < types.IstanbulExtraVanity {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity-len(header.Extra))...)
	}
	header.Extra = append(header.Extra[:types.IstanbulExtraVanity], payload...)

	return nil
}

// writeValidatorSetDiff initializes the header's Extra field with any changes in the
// validator set that occurred since the last block
func writeValidatorSetDiff(header *types.Header, oldValSet []istanbul.ValidatorData, newValSet []istanbul.ValidatorData) error {
	// compensate the lack bytes if header.Extra is not enough IstanbulExtraVanity bytes.
	if len(header.Extra) < types.IstanbulExtraVanity {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity-len(header.Extra))...)
	}

	addedValidators, removedValidators := istanbul.ValidatorSetDiff(oldValSet, newValSet)
	addedValidatorsAddresses, addedValidatorsPublicKeys := istanbul.SeparateValidatorDataIntoIstanbulExtra(addedValidators)

	if len(addedValidators) > 0 || removedValidators.BitLen() > 0 {
		oldValidatorsAddresses, _ := istanbul.SeparateValidatorDataIntoIstanbulExtra(oldValSet)
		newValidatorsAddresses, _ := istanbul.SeparateValidatorDataIntoIstanbulExtra(newValSet)
		log.Debug("Setting istanbul header validator fields", "oldValSet", common.ConvertToStringSlice(oldValidatorsAddresses), "newValSet", common.ConvertToStringSlice(newValidatorsAddresses),
			"addedValidators", common.ConvertToStringSlice(addedValidatorsAddresses), "removedValidators", removedValidators.Text(16))
	}

	extra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		return nil
	}

	extra.AddedValidators = addedValidatorsAddresses
	extra.AddedValidatorsPublicKeys = addedValidatorsPublicKeys
	extra.RemovedValidators = removedValidators

	// update the header's extra with the new diff
	payload, err := rlp.EncodeToBytes(extra)
	if err != nil {
		return err
	}
	header.Extra = append(header.Extra[:types.IstanbulExtraVanity], payload...)

	return nil
}

// writeSeal writes the extra-data field of the given header with the given seal.
func writeSeal(h *types.Header, seal []byte) error {
	if len(seal) != types.IstanbulExtraSeal {
		return errInvalidSignature
	}

	istanbulExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		return err
	}

	istanbulExtra.Seal = seal
	payload, err := rlp.EncodeToBytes(&istanbulExtra)
	if err != nil {
		return err
	}

	h.Extra = append(h.Extra[:types.IstanbulExtraVanity], payload...)
	return nil
}

// writeAggregatedSeal writes the extra-data field of a block header with given committed
// seals. If isParent is set to true, then it will write to the fields related
// to the parent commits of the block
func writeAggregatedSeal(h *types.Header, aggregatedSeal types.IstanbulAggregatedSeal, isParent bool) error {
	if len(aggregatedSeal.Signature) != types.IstanbulExtraBlsSignature {
		return errInvalidAggregatedSeal
	}

	istanbulExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		return err
	}

	if isParent {
		istanbulExtra.ParentAggregatedSeal = aggregatedSeal
	} else {
		istanbulExtra.AggregatedSeal = aggregatedSeal
	}

	payload, err := rlp.EncodeToBytes(&istanbulExtra)
	if err != nil {
		return err
	}

	// compensate the lack bytes if header.Extra is not enough IstanbulExtraVanity bytes.
	if len(h.Extra) < types.IstanbulExtraVanity {
		h.Extra = append(h.Extra, bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity-len(h.Extra))...)
	}

	h.Extra = append(h.Extra[:types.IstanbulExtraVanity], payload...)
	return nil
}

func waitCoreToReachSequence(core istanbulCore.Engine, expectedSequence *big.Int) *big.Int {
	logger := log.New("func", "waitCoreToReachSequence")
	timeout := time.After(500 * time.Millisecond)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			view := core.CurrentView()
			if view != nil && view.Sequence != nil && view.Sequence.Cmp(expectedSequence) == 0 {
				logger.Trace("Current sequence matches header", "cur_seq", view.Sequence)
				return view.Sequence
			}
		case <-timeout:
			log.Trace("Timed out while waiting for core to sequence change, unable to combine commit messages with ParentAggregatedSeal", "cur_view", core.CurrentView())
			return nil
		}
	}
}
