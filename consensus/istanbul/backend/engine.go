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

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	istanbulCore "github.com/celo-org/celo-blockchain/consensus/istanbul/core"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/uptime"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	gpm "github.com/celo-org/celo-blockchain/contracts/gasprice_minimum"
	"github.com/celo-org/celo-blockchain/contracts/gold_token"
	"github.com/celo-org/celo-blockchain/core"
	ethCore "github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-blockchain/rpc"
	"github.com/celo-org/celo-blockchain/trie"
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
	// errEmptyAggregatedSeal is returned if the aggregated seal is missing.
	errEmptyAggregatedSeal = errors.New("empty aggregated seal")
	// errNonEmptyAggregatedSeal is returned if the aggregated seal is not empty during preprepase proposal phase.
	errNonEmptyAggregatedSeal = errors.New("Non empty aggregated seal during preprepare")
	// errMismatchTxhashes is returned if the TxHash in header is mismatch.
	errMismatchTxhashes = errors.New("mismatch transactions hashes")
	// errInvalidValidatorSetDiff is returned if the header contains invalid validator set diff
	errInvalidValidatorSetDiff = errors.New("invalid validator set diff")
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
	return sb.verifyHeader(chain, header, false, nil)
}

// verifyHeaderFromProposal checks whether a header conforms to the consensus rules from the
// preprepare istanbul phase.
func (sb *Backend) verifyHeaderFromProposal(chain consensus.ChainHeaderReader, header *types.Header) error {
	return sb.verifyHeader(chain, header, true, nil)
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
// If emptyAggregatedSeal is set, the aggregatedSeal will be checked to be completely empty. Otherwise
// it will be checked as a normal aggregated seal.
func (sb *Backend) verifyHeader(chain consensus.ChainHeaderReader, header *types.Header, emptyAggregatedSeal bool, parents []*types.Header) error {
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
	if _, err := header.IstanbulExtra(); err != nil {
		return errInvalidExtraDataFormat
	}

	return sb.verifyCascadingFields(chain, header, emptyAggregatedSeal, parents)
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
	if parent == nil || parent.Number.Uint64() != epochBlockNumber {
		return consensus.ErrUnknownAncestor
	}
	return nil
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
// If emptyAggregatedSeal is set, the aggregatedSeal will be checked to be completely empty. Otherwise
// it will be checked as a normal aggregated seal.
func (sb *Backend) verifyCascadingFields(chain consensus.ChainHeaderReader, header *types.Header, emptyAggregatedSeal bool, parents []*types.Header) error {
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

	return sb.verifyAggregatedSeals(chain, header, emptyAggregatedSeal, parents)
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
				err = sb.verifyHeader(chain, header, false, headers[:i])
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
// If emptyAggregatedSeal is set, the aggregatedSeal will be checked to be completely empty. Otherwise
// it will be checked as a normal aggregated seal.
func (sb *Backend) verifyAggregatedSeals(chain consensus.ChainHeaderReader, header *types.Header, emptyAggregatedseal bool, parents []*types.Header) error {
	number := header.Number.Uint64()
	// We don't need to verify committed seals in the genesis block
	if number == 0 {
		return nil
	}

	extra, err := header.IstanbulExtra()
	if err != nil {
		return err
	}

	// Check the signatures on the current header
	snap, err := sb.snapshot(chain, number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}
	validators := snap.ValSet.Copy()

	if emptyAggregatedseal {
		// The length of Committed seals should be exactly 0 (preprepare proposal check)
		if len(extra.AggregatedSeal.Signature) != 0 {
			return errNonEmptyAggregatedSeal
		}
		// Should we also verify that the bitmap and round are nil?
	} else {
		// The length of Committed seals should be larger than 0
		if len(extra.AggregatedSeal.Signature) == 0 {
			return errEmptyAggregatedSeal
		}

		err = sb.verifyAggregatedSeal(header.Hash(), validators, extra.AggregatedSeal)
		if err != nil {
			return err
		}
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

	extra, err := header.IstanbulExtra()
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

	// Record what the delay should be and sleep if greater than 0.
	// TODO(victor): Sleep here was previously removed and added to the miner instead, that change
	// has been temporarily reverted until it can be reimplemented without causing fewer signatures
	// to be included by the block producer.
	delay := time.Until(time.Unix(int64(header.Time), 0))
	if delay < 0 {
		sb.sleepGauge.Update(0)
	} else {
		sb.sleepGauge.Update(delay.Nanoseconds())
		time.Sleep(delay)
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
	state.Prepare(common.Hash{}, len(txs))

	snapshot := state.Snapshot()
	vmRunner := sb.chain.NewEVMRunner(header, state)
	err := gold_token.SetInitialTotalSupplyIfUnset(sb.db, vmRunner)
	if err != nil {
		state.RevertToSnapshot(snapshot)
	}

	if !sb.ChainConfig().IsGingerbread(header.Number) {
		// Trigger an update to the gas price minimum in the GasPriceMinimum contract based on block congestion
		snapshot = state.Snapshot()
		_, err = gpm.UpdateGasPriceMinimum(vmRunner, header.GasUsed)
		if err != nil {
			state.RevertToSnapshot(snapshot)
		}
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
	block := types.NewBlock(header, txs, receipts, randomness, new(trie.Trie))
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
		// Batched. For stats & announce
		chainHeadCh := make(chan ethCore.ChainHeadEvent, 10)
		chainHeadSub := bc.SubscribeChainHeadEvent(chainHeadCh)

		go func() {
			defer chainHeadSub.Unsubscribe()
			// Loop to run on new chain head events. Chain head events may be batched.
			for {
				select {
				case chainHeadEvent := <-chainHeadCh:
					sb.newChainHead(chainHeadEvent.Block)
				case err := <-chainHeadSub.Err():
					log.Error("Error in istanbul's subscription to the blockchain's chainhead event", "err", err)
					return
				}
			}
		}()

		// Unbatched event listener
		chainEventCh := make(chan ethCore.ChainEvent, 10)
		chainEventSub := bc.SubscribeChainEvent(chainEventCh)

		go func() {
			defer chainEventSub.Unsubscribe()
			// Loop to update replica state. Listens to chain events to avoid batching.
			for {
				select {
				case chainEvent := <-chainEventCh:
					if !sb.isCoreStarted() && sb.replicaState != nil {
						consensusBlock := new(big.Int).Add(chainEvent.Block.Number(), common.Big1)
						sb.replicaState.NewChainHead(consensusBlock)
					}
				case err := <-chainEventSub.Err():
					log.Error("Error in istanbul's subscription to the blockchain's chain event", "err", err)
					return
				}
			}
		}()
	}
}

// SetCallBacks implements consensus.Istanbul.SetCallBacks
func (sb *Backend) SetCallBacks(hasBadBlock func(common.Hash) bool,
	processBlock func(*types.Block, *state.StateDB) (types.Receipts, []*types.Log, uint64, error),
	validateState func(*types.Block, *state.StateDB, types.Receipts, uint64) error,
	onNewConsensusBlock func(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB)) error {
	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if sb.isCoreStarted() {
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
	if sb.isCoreStarted() {
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

	sb.coreStarted.Store(true)

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
	if !sb.isCoreStarted() {
		return istanbul.ErrStoppedEngine
	}
	sb.logger.Info("Stopping istanbul.Engine validating")
	if err := sb.core.Stop(); err != nil {
		return err
	}
	sb.coreStarted.Store(false)

	return nil
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

		istanbulExtra, err := genesis.IstanbulExtra()
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

func (sb *Backend) addParentSeal(chain consensus.ChainHeaderReader, header *types.Header) error {
	number := header.Number.Uint64()
	logger := sb.logger.New("func", "addParentSeal", "number", number)

	// only do this for blocks which start with block 1 as a parent
	if number <= 1 {
		return nil
	}

	// Get parent's extra to fetch it's AggregatedSeal
	parent := chain.GetHeader(header.ParentHash, number-1)
	parentExtra, err := parent.IstanbulExtra()
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
	istanbulExtra, err := header.IstanbulExtra()
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

	extra, err := header.IstanbulExtra()
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

	istanbulExtra, err := h.IstanbulExtra()
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

	istanbulExtra, err := h.IstanbulExtra()
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
