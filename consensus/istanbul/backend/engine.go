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
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	istanbulCore "github.com/ethereum/go-ethereum/consensus/istanbul/core"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	gpm "github.com/ethereum/go-ethereum/contract_comm/gasprice_minimum"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
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
	// errInvalidDifficulty is returned if the difficulty of a block is not 1
	errInvalidDifficulty = errors.New("invalid difficulty")
	// errInvalidExtraDataFormat is returned when the extra data format is incorrect
	errInvalidExtraDataFormat = errors.New("invalid extra data format")
	// errInvalidMixDigest is returned if a block's mix digest is not Istanbul digest.
	errInvalidMixDigest = errors.New("invalid Istanbul mix digest")
	// errInvalidNonce is returned if a block's nonce is invalid
	errInvalidNonce = errors.New("invalid nonce")
	// errCoinbase is returned if a block's coinbase is invalid
	errInvalidCoinbase = errors.New("invalid coinbase")
	// errInvalidUncleHash is returned if a block contains an non-empty uncle list.
	errInvalidUncleHash = errors.New("non empty uncle hash")
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
	// errUnauthorizedValEnodesShareMessage is returned when the received valEnodeshare message is from
	// an unauthorized sender
	errUnauthorizedValEnodesShareMessage = errors.New("unauthorized valenodesshare message")
)

var (
	defaultDifficulty = big.NewInt(1)
	nilUncleHash      = types.CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.
	emptyNonce        = types.BlockNonce{}
	now               = time.Now

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
func (sb *Backend) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	return sb.verifyHeader(chain, header, nil)
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (sb *Backend) verifyHeader(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
	if header.Number == nil {
		return errUnknownBlock
	}

	// If the full chain isn't available (as on mobile devices), don't reject future blocks
	// This is due to potential clock skew
	var allowedFutureBlockTime = big.NewInt(now().Unix())
	if !chain.Config().FullHeaderChainAvailable {
		allowedFutureBlockTime = new(big.Int).Add(allowedFutureBlockTime, new(big.Int).SetUint64(mobileAllowedClockSkew))
	}

	// Don't waste time checking blocks from the future
	if header.Time.Cmp(allowedFutureBlockTime) > 0 {
		return consensus.ErrFutureBlock
	}

	// Ensure that the extra data format is satisfied
	if _, err := types.ExtractIstanbulExtra(header); err != nil {
		return errInvalidExtraDataFormat
	}

	// Ensure that the nonce is empty (Istanbul was originally using it for a candidate validator vote)
	if header.Nonce != (emptyNonce) {
		return errInvalidNonce
	}

	// Ensure that the mix digest is zero as we don't have fork protection currently
	if header.MixDigest != types.IstanbulDigest {
		return errInvalidMixDigest
	}
	// Ensure that the block doesn't contain any uncles which are meaningless in Istanbul
	if header.UncleHash != nilUncleHash {
		return errInvalidUncleHash
	}
	// Ensure that the block's difficulty is meaningful (may not be correct at this point)
	if header.Difficulty == nil || header.Difficulty.Cmp(defaultDifficulty) != 0 {
		return errInvalidDifficulty
	}

	return sb.verifyCascadingFields(chain, header, parents)
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (sb *Backend) verifyCascadingFields(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
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
	}
	if chain.Config().FullHeaderChainAvailable {

		if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
			return consensus.ErrUnknownAncestor
		}
		if parent.Time.Uint64()+sb.config.BlockPeriod > header.Time.Uint64() {
			return errInvalidTimestamp
		}
		// Verify validators in extraData. Validators in snapshot and extraData should be the same.
		if err := sb.verifySigner(chain, header, parents); err != nil {
			return err
		}
	}

	return sb.verifyAggregatedSeals(chain, header, parents)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (sb *Backend) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for i, header := range headers {
			err := sb.verifyHeader(chain, header, headers[:i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of a given engine.
func (sb *Backend) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return errInvalidUncleHash
	}
	return nil
}

// verifySigner checks whether the signer is in parent's validator set
func (sb *Backend) verifySigner(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
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
func (sb *Backend) verifyAggregatedSeals(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {
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
	err := blscrypto.VerifyAggregatedSignature(publicKeys, proposalSeal, []byte{}, aggregatedSeal.Signature, false)
	if err != nil {
		logger.Error("Unable to verify aggregated signature", "err", err)
		return errInvalidSignature
	}

	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (sb *Backend) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
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

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (sb *Backend) Prepare(chain consensus.ChainReader, header *types.Header) error {
	// unused fields, force to set to empty
	header.Coinbase = sb.address
	header.Nonce = emptyNonce
	header.MixDigest = types.IstanbulDigest

	// copy the parent extra data as the header extra data
	number := header.Number.Uint64()
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	// use the same difficulty for all blocks
	header.Difficulty = defaultDifficulty

	// set header's timestamp
	header.Time = new(big.Int).Add(parent.Time, new(big.Int).SetUint64(sb.config.BlockPeriod))
	if header.Time.Int64() < time.Now().Unix() {
		header.Time = big.NewInt(time.Now().Unix())
	}

	if err := writeEmptyIstanbulExtra(header); err != nil {
		return err
	}

	// wait for the timestamp of header, use this to adjust the block period
	delay := time.Unix(header.Time.Int64(), 0).Sub(now())
	time.Sleep(delay)

	return sb.addParentSeal(chain, header)
}

// UpdateValSetDiff will update the validator set diff in the header, if the mined header is the last block of the epoch
func (sb *Backend) UpdateValSetDiff(chain consensus.ChainReader, header *types.Header, state *state.StateDB) error {
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

// Returns whether or not a particular header represents the last block in the epoch.
func (sb *Backend) IsLastBlockOfEpoch(header *types.Header) bool {
	return istanbul.IsLastBlockOfEpoch(header.Number.Uint64(), sb.config.Epoch)
}

// Returns the size of epochs in blocks.
func (sb *Backend) EpochSize() uint64 {
	return sb.config.Epoch
}

// Returns the size of the lookback window for calculating uptime (in blocks)
func (sb *Backend) LookbackWindow() uint64 {
	return sb.config.LookbackWindow
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
//
// Note, the block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (sb *Backend) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt, randomness *types.Randomness) (*types.Block, error) {
	start := time.Now()
	defer sb.finalizationTimer.UpdateSince(start)

	snapshot := state.Snapshot()
	err := sb.setInitialGoldTokenTotalSupplyIfUnset(header, state)
	if err != nil {
		state.RevertToSnapshot(snapshot)
	}

	// Trigger an update to the gas price minimum in the GasPriceMinimum contract based on block congestion
	snapshot = state.Snapshot()
	_, err = gpm.UpdateGasPriceMinimum(header, state)
	if err != nil {
		state.RevertToSnapshot(snapshot)
	}

	sb.logger.Trace("Finalizing", "block", header.Number.Uint64(), "epochSize", sb.config.Epoch)
	if istanbul.IsLastBlockOfEpoch(header.Number.Uint64(), sb.config.Epoch) {
		snapshot = state.Snapshot()
		err = sb.distributeEpochPaymentsAndRewards(header, state)
		if err != nil {
			state.RevertToSnapshot(snapshot)
		}
	}

	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = nilUncleHash

	if len(state.GetLogs(common.Hash{})) > 0 {
		receipt := types.NewReceipt(nil, false, 0)
		receipt.Logs = state.GetLogs(common.Hash{})
		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		receipts = append(receipts, receipt)
	}

	// Assemble and return the final block for sealing
	return types.NewBlock(header, txs, nil, receipts, randomness), nil
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top.
func (sb *Backend) Seal(chain consensus.ChainReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	// update the block header timestamp and signature and propose the block to core engine
	header := block.Header()
	number := header.Number.Uint64()

	// Bail out if we're unauthorized to sign a block
	snap, err := sb.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return err
	}
	if _, v := snap.ValSet.GetByAddress(sb.address); v == nil {
		return errUnauthorized
	}

	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	block, err = sb.updateBlock(parent, block)
	if err != nil {
		return err
	}

	// get the proposed block hash and clear it if the seal() is completed.
	sb.sealMu.Lock()
	sb.proposedBlockHash = block.Hash()
	clear := func() {
		sb.proposedBlockHash = common.Hash{}
		sb.sealMu.Unlock()
	}

	// post block into Istanbul engine
	go sb.EventMux().Post(istanbul.RequestEvent{
		Proposal: block,
	})

	go func() {
		defer clear()
		for {
			select {
			case result := <-sb.commitCh:
				// Somehow, the block `result` coming from commitCh can be null
				// if the block hash and the hash from channel are the same,
				// return the result. Otherwise, keep waiting the next hash.
				if result != nil && block.Hash() == result.Hash() {
					results <- result
					return
				}
			case <-stop:
				return
			}
		}
	}()
	return nil
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func (sb *Backend) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	return defaultDifficulty
}

// SealHash returns the hash of a block prior to it being sealed.
func (sb *Backend) SealHash(header *types.Header) common.Hash {
	return sigHash(header)
}

// update timestamp and signature of the block based on its number of transactions
func (sb *Backend) updateBlock(parent *types.Header, block *types.Block) (*types.Block, error) {
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

	return block.WithSeal(header), nil
}

// APIs returns the RPC APIs this consensus engine provides.
func (sb *Backend) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{{
		Namespace: "istanbul",
		Version:   "1.0",
		Service:   &API{chain: chain, istanbul: sb},
		Public:    true,
	}}
}

func (sb *Backend) SetChain(chain consensus.ChainReader, currentBlock func() *types.Block) {
	sb.chain = chain
	sb.currentBlock = currentBlock
}

// Start implements consensus.Istanbul.Start
func (sb *Backend) Start(hasBadBlock func(common.Hash) bool,
	stateAt func(common.Hash) (*state.StateDB, error), processBlock func(*types.Block, *state.StateDB) (types.Receipts, []*types.Log, uint64, error),
	validateState func(*types.Block, *state.StateDB, types.Receipts, uint64) error) error {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if sb.coreStarted {
		return istanbul.ErrStartedEngine
	}

	// clear previous data
	sb.proposedBlockHash = common.Hash{}
	if sb.commitCh != nil {
		close(sb.commitCh)
	}
	sb.commitCh = make(chan *types.Block, 1)

	if sb.newEpochCh != nil {
		close(sb.newEpochCh)
	}
	sb.newEpochCh = make(chan struct{})

	sb.hasBadBlock = hasBadBlock
	sb.stateAt = stateAt
	sb.processBlock = processBlock
	sb.validateState = validateState

	sb.logger.Info("Starting istanbul.Engine")
	if err := sb.core.Start(); err != nil {
		return err
	}

	sb.coreStarted = true

	go sb.sendAnnounceMsgs()

	if sb.config.Proxied {
		if sb.config.ProxyInternalFacingNode != nil && sb.config.ProxyExternalFacingNode != nil {
			if err := sb.addProxy(sb.config.ProxyInternalFacingNode, sb.config.ProxyExternalFacingNode); err != nil {
				sb.logger.Error("Issue in adding proxy on istanbul start", "err", err)
			}
		}

		go sb.sendValEnodesShareMsgs()
	} else {
		headBlock := sb.GetCurrentHeadBlock()
		valset := sb.getValidators(headBlock.Number().Uint64(), headBlock.Hash())
		sb.RefreshValPeers(valset)
	}

	return nil
}

// Stop implements consensus.Istanbul.Stop
func (sb *Backend) Stop() error {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if !sb.coreStarted {
		return istanbul.ErrStoppedEngine
	}
	sb.logger.Info("Stopping istanbul.Engine")
	if err := sb.core.Stop(); err != nil {
		return err
	}
	sb.coreStarted = false

	sb.announceQuit <- struct{}{}
	sb.announceWg.Wait()

	if sb.config.Proxied {
		sb.valEnodesShareQuit <- struct{}{}
		sb.valEnodesShareWg.Wait()

		if sb.proxyNode != nil {
			sb.removeProxy(sb.proxyNode.node)
		}
	}
	return nil
}

// snapshot retrieves the validator set needed to sign off on the block immediately after 'number'.  E.g. if you need to find the validator set that needs to sign off on block 6,
// this method should be called with number set to 5.
//
// hash - The requested snapshot's block's hash. Only used for snapshot cache storage.
// number - The requested snapshot's block number
// parents - (Optional argument) An array of headers from directly previous blocks.
func (sb *Backend) snapshot(chain consensus.ChainReader, number uint64, hash common.Hash, parents []*types.Header) (*Snapshot, error) {
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
				log.Error("The header retrieved from the chain is nil", "block num", numberIter)
				return nil, errUnknownBlock
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

func (sb *Backend) addParentSeal(chain consensus.ChainReader, header *types.Header) error {
	number := header.Number.Uint64()
	logger := sb.logger.New("func", "Backend.addParentSeal()", "number", number)

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
			// TODO(asa): Why is this logged by full nodes?
			log.Trace("Timed out while waiting for core to sequence change, unable to combine commit messages with ParentAggregatedSeal", "cur_view", core.CurrentView())
			return nil
		}
	}
}
