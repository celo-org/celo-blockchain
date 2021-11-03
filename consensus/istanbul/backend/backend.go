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
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/announce"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/internal/replica"
	istanbulCore "github.com/celo-org/celo-blockchain/consensus/istanbul/core"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/proxy"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/election"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/trie"

	"github.com/celo-org/celo-blockchain/contracts/random"
	"github.com/celo-org/celo-blockchain/contracts/validators"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/event"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/params"
	lru "github.com/hashicorp/golang-lru"
)

// New creates an Ethereum backend for Istanbul core engine.
func New(config *istanbul.Config, db ethdb.Database) consensus.Istanbul {
	// Allocate the snapshot caches and create the engine
	logger := log.New()
	recentSnapshots, err := lru.NewARC(inmemorySnapshots)
	if err != nil {
		logger.Crit("Failed to create recent snapshots cache", "err", err)
	}

	coreStarted := atomic.Value{}
	coreStarted.Store(false)
	backend := &Backend{
		config:                             config,
		istanbulEventMux:                   new(event.TypeMux),
		logger:                             logger,
		db:                                 db,
		recentSnapshots:                    recentSnapshots,
		coreStarted:                        coreStarted,
		gossipCache:                        NewLRUGossipCache(inmemoryPeers, inmemoryMessages),
		updatingCachedValidatorConnSetCond: sync.NewCond(&sync.Mutex{}),
		finalizationTimer:                  metrics.NewRegisteredTimer("consensus/istanbul/backend/finalize", nil),
		rewardDistributionTimer:            metrics.NewRegisteredTimer("consensus/istanbul/backend/rewards", nil),
		blocksElectedMeter:                 metrics.NewRegisteredMeter("consensus/istanbul/blocks/elected", nil),
		blocksElectedAndSignedMeter:        metrics.NewRegisteredMeter("consensus/istanbul/blocks/signedbyus", nil),
		blocksElectedButNotSignedMeter:     metrics.NewRegisteredMeter("consensus/istanbul/blocks/missedbyus", nil),
		blocksElectedAndProposedMeter:      metrics.NewRegisteredMeter("consensus/istanbul/blocks/proposedbyus", nil),
		blocksTotalSigsGauge:               metrics.NewRegisteredGauge("consensus/istanbul/blocks/totalsigs", nil),
		blocksValSetSizeGauge:              metrics.NewRegisteredGauge("consensus/istanbul/blocks/validators", nil),
		blocksTotalMissedRoundsMeter:       metrics.NewRegisteredMeter("consensus/istanbul/blocks/missedrounds", nil),
		blocksMissedRoundsAsProposerMeter:  metrics.NewRegisteredMeter("consensus/istanbul/blocks/missedroundsasproposer", nil),
		blocksElectedButNotSignedGauge:     metrics.NewRegisteredGauge("consensus/istanbul/blocks/missedbyusinarow", nil),
		blocksDowntimeEventMeter:           metrics.NewRegisteredMeter("consensus/istanbul/blocks/downtimeevent", nil),
		blocksFinalizedTransactionsGauge:   metrics.NewRegisteredGauge("consensus/istanbul/blocks/transactions", nil),
		blocksFinalizedGasUsedGauge:        metrics.NewRegisteredGauge("consensus/istanbul/blocks/gasused", nil),
		sleepGauge:                         metrics.NewRegisteredGauge("consensus/istanbul/backend/sleep", nil),
	}
	backend.aWallets.Store(&istanbul.Wallets{})
	if config.LoadTestCSVFile != "" {
		if f, err := os.Create(config.LoadTestCSVFile); err == nil {
			backend.csvRecorder = metrics.NewCSVRecorder(f, "blockNumber", "txCount", "gasUsed", "round",
				"cycle", "sleep", "consensus", "block_verify", "block_construct",
				"sysload", "syswait", "procload")
		}
	}

	backend.core = istanbulCore.New(backend, backend.config)

	if config.Validator {
		rs, err := replica.NewState(config.Replica, config.ReplicaStateDBPath, backend.StartValidating, backend.StopValidating)
		if err != nil {
			logger.Crit("Can't open ReplicaStateDB", "err", err, "dbpath", config.ReplicaStateDBPath)
		}
		backend.replicaState = rs
	} else {
		backend.replicaState = nil
	}

	backend.vph = newVPH(backend)
	valEnodeTable, err := announce.OpenValidatorEnodeDB(config.ValidatorEnodeDBPath, backend.vph)
	if err != nil {
		logger.Crit("Can't open ValidatorEnodeDB", "err", err, "dbpath", config.ValidatorEnodeDBPath)
	}
	backend.valEnodeTable = valEnodeTable

	// If this node is a proxy or is a proxied validator, then create the appropriate proxy engine object
	if backend.IsProxy() {
		backend.proxyEngine, err = proxy.NewProxyEngine(backend, backend.config)
		if err != nil {
			logger.Crit("Can't create a new proxy engine", "err", err)
		}
	} else if backend.IsProxiedValidator() {
		backend.proxiedValidatorEngine, err = proxy.NewProxiedValidatorEngine(backend, backend.config)
		if err != nil {
			logger.Crit("Can't create a new proxied validator engine", "err", err)
		}
	}

	backend.announceManager = createAnnounceManager(backend)

	return backend
}

func createAnnounceManager(backend *Backend) *AnnounceManager {
	versionCertificateTable, err := announce.OpenVersionCertificateDB(backend.config.VersionCertificateDBPath)
	if err != nil {
		backend.logger.Crit("Can't open VersionCertificateDB", "err", err, "dbpath", backend.config.VersionCertificateDBPath)
	}
	vcGossiper := NewVcGossiper(func(payload []byte) error {
		return backend.Gossip(payload, istanbul.VersionCertificatesMsg)
	})

	state := NewAnnounceState(backend.valEnodeTable, versionCertificateTable)
	checker := announce.NewValidatorChecker(&backend.aWallets, backend.RetrieveValidatorConnSet, backend.IsValidating)
	ovcp := NewOutboundVCProcessor(checker, backend, vcGossiper)
	ecertHolder := announce.NewLockedHolder()
	pruner := NewAnnounceStatePruner(backend.RetrieveValidatorConnSet)

	var vpap ValProxyAssigmnentProvider
	var ecertGenerator announce.EnodeCertificateMsgGenerator
	var onNewEnodeMsgs OnNewEnodeCertsMsgSentFn
	if backend.IsProxiedValidator() {
		ecertGenerator = announce.NewEnodeCertificateMsgGenerator(
			announce.NewProxiedExternalFacingEnodeGetter(backend.proxiedValidatorEngine.GetProxiesAndValAssignments),
		)
		vpap = NewProxiedValProxyAssigmentProvider(backend.proxiedValidatorEngine.GetValidatorProxyAssignments)
		onNewEnodeMsgs = backend.proxiedValidatorEngine.SendEnodeCertsToAllProxies
	} else {
		ecertGenerator = announce.NewEnodeCertificateMsgGenerator(announce.NewSelfExternalFacingEnodeGetter(backend.SelfNode))
		vpap = NewSelfValProxyAssigmentProvider(backend.SelfNode)
		onNewEnodeMsgs = nil
	}

	avs := NewAnnounceVersionSharer(&backend.aWallets, backend, state, ovcp, ecertGenerator, ecertHolder, onNewEnodeMsgs)
	worker := createAnnounceWorker(backend, state, ovcp, vcGossiper, checker, pruner, vpap, avs)
	return NewAnnounceManager(
		backend.config,
		&backend.aWallets,
		backend,
		backend,
		backend,
		state,
		backend.gossipCache,
		checker,
		ovcp,
		ecertHolder,
		vcGossiper,
		vpap,
		worker)
}

func createAnnounceWorker(backend *Backend, state *AnnounceState, ovcp OutboundVersionCertificateProcessor,
	vcGossiper VersionCertificateGossiper,
	checker announce.ValidatorChecker, pruner AnnounceStatePruner,
	vpap ValProxyAssigmnentProvider, avs AnnounceVersionSharer) AnnounceWorker {
	announceVersion := announce.NewAtomicVersion()
	peerCounter := func(purpose p2p.PurposeFlag) int {
		return len(backend.broadcaster.FindPeers(nil, p2p.AnyPurpose))
	}

	enodeGossiper := announce.NewEnodeQueryGossiper(announceVersion, func(payload []byte) error {
		return backend.Gossip(payload, istanbul.QueryEnodeMsg)
	})
	// Gossip the announce after a minute.
	// The delay allows for all receivers of the announce message to
	// have a more up-to-date cached registered/elected valset, and
	// hence more likely that they will be aware that this node is
	// within that set.
	waitPeriod := 1 * time.Minute
	if backend.config.Epoch <= 10 {
		waitPeriod = 5 * time.Second
	}
	return NewAnnounceWorker(
		waitPeriod,
		&backend.aWallets,
		announceVersion,
		state,
		checker,
		pruner,
		vcGossiper,
		enodeGossiper,
		backend.config,
		peerCounter,
		vpap,
		avs,
	)
}

// ----------------------------------------------------------------------------

type Backend struct {
	config           *istanbul.Config
	istanbulEventMux *event.TypeMux

	aWallets atomic.Value

	core         istanbulCore.Engine
	logger       log.Logger
	db           ethdb.Database
	chain        consensus.ChainContext
	currentBlock func() *types.Block
	hasBadBlock  func(hash common.Hash) bool
	stateAt      func(hash common.Hash) (*state.StateDB, error)
	replicaState replica.State

	processBlock        func(block *types.Block, statedb *state.StateDB) (types.Receipts, []*types.Log, uint64, error)
	validateState       func(block *types.Block, statedb *state.StateDB, receipts types.Receipts, usedGas uint64) error
	onNewConsensusBlock func(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB)

	// We need this to be an atomic value so that we can access it in a lock
	// free way from IsValidating. This is required because StartValidating
	// makes a call to RefreshValPeers while holding coreMu and RefreshValPeers
	// waits for all validator peers to be deleted and then reconnects to known
	// validators. If any of those peers has called IsValidating before
	// RefreshValPeers tries to delete them the system gets stuck in a
	// deadlock, the peer will never acquire coreMu because it is held by
	// StartValidating, and StartValidating will never return because it is
	// waiting for all peers to disconnect.
	coreStarted atomic.Value
	coreMu      sync.RWMutex

	// Snapshots for recent blocks to speed up reorgs
	recentSnapshots *lru.ARCCache

	// event subscription for ChainHeadEvent event
	broadcaster consensus.Broadcaster

	// interface to the p2p server
	p2pserver consensus.P2PServer

	gossipCache GossipCache

	valEnodeTable *announce.ValidatorEnodeDB

	announceManager *AnnounceManager

	delegateSignFeed  event.Feed
	delegateSignScope event.SubscriptionScope

	// Metric timer used to record block finalization times.
	finalizationTimer metrics.Timer
	// Metric timer used to record epoch reward distribution times.
	rewardDistributionTimer metrics.Timer

	// Meters for number of blocks seen for which the current validator signer has been elected,
	// for which it was elected and has signed, elected but not signed, and both elected and proposed.
	blocksElectedMeter             metrics.Meter
	blocksElectedAndSignedMeter    metrics.Meter
	blocksElectedButNotSignedMeter metrics.Meter
	blocksElectedAndProposedMeter  metrics.Meter

	// Gauge for how many blocks that we missed while elected in a row.
	blocksElectedButNotSignedGauge metrics.Gauge
	// Meter for downtime events when we did not sign 12+ blocks in a row.
	blocksDowntimeEventMeter metrics.Meter

	// Gauge for total signatures in parentSeal of last received block (how much better than quorum are we doing)
	blocksTotalSigsGauge metrics.Gauge

	// Gauge for validator set size of grandparent of last received block (maximum value for blocksTotalSigsGauge)
	blocksValSetSizeGauge metrics.Gauge

	// Meter counting cumulative number of round changes that had to happen to get blocks agreed
	// for all blocks & when are the proposer.
	blocksTotalMissedRoundsMeter      metrics.Meter
	blocksMissedRoundsAsProposerMeter metrics.Meter

	// Gauge counting the transactions in the last block
	blocksFinalizedTransactionsGauge metrics.Gauge

	// Gauge counting the gas used in the last block
	blocksFinalizedGasUsedGauge metrics.Gauge

	// Gauge reporting how many nanoseconds were spent sleeping
	sleepGauge metrics.Gauge
	// Start of the previous block cycle.
	cycleStart time.Time

	// Consensus csv recorded for load testing
	csvRecorder *metrics.CSVRecorder

	// Cache for the return values of the method RetrieveValidatorConnSet
	cachedValidatorConnSet         map[common.Address]bool
	cachedValidatorConnSetBlockNum uint64
	cachedValidatorConnSetTS       time.Time
	cachedValidatorConnSetMu       sync.RWMutex

	// Used for ensuring that only one goroutine is doing the work of updating
	// the validator conn set cache at a time.
	updatingCachedValidatorConnSet     bool
	updatingCachedValidatorConnSetErr  error
	updatingCachedValidatorConnSetCond *sync.Cond

	// Handler to manage and maintain validator peer connections
	vph *validatorPeerHandler

	// Handler for proxy related functionality
	proxyEngine proxy.ProxyEngine

	// Handler for proxied validator related functionality
	proxiedValidatorEngine        proxy.ProxiedValidatorEngine
	proxiedValidatorEngineRunning bool
	proxiedValidatorEngineMu      sync.RWMutex

	// RandomSeed (and it's mutex) used to generate the random beacon randomness
	randomSeed   []byte
	randomSeedMu sync.Mutex

	// Test hooks
	abortCommitHook func(result *istanbulCore.StateProcessResult) bool // Method to call upon committing a proposal
}

func (sb *Backend) isCoreStarted() bool {
	return sb.coreStarted.Load().(bool)
}

// IsProxy returns true if instance has proxy flag
func (sb *Backend) IsProxy() bool {
	return sb.config.Proxy
}

// GetProxyEngine returns the proxy engine object
func (sb *Backend) GetProxyEngine() proxy.ProxyEngine {
	return sb.proxyEngine
}

// IsProxiedValidator returns true if instance has proxied validator flag
func (sb *Backend) IsProxiedValidator() bool {
	return sb.config.Proxied && sb.IsValidator()
}

// GetProxiedValidatorEngine returns the proxied validator engine object
func (sb *Backend) GetProxiedValidatorEngine() proxy.ProxiedValidatorEngine {
	sb.proxiedValidatorEngineMu.RLock()
	defer sb.proxiedValidatorEngineMu.RUnlock()
	return sb.proxiedValidatorEngine
}

// IsValidating return true if instance is validating
func (sb *Backend) IsValidating() bool {
	// TODO: Maybe a little laggy, but primary / replica should track the core
	return sb.isCoreStarted()
}

// IsValidator return if instance is a validator (either proxied or standalone)
func (sb *Backend) IsValidator() bool {
	return sb.config.Validator
}

// ChainConfig returns the configuration from the embedded blockchain reader.
func (sb *Backend) ChainConfig() *params.ChainConfig {
	return sb.chain.Config()
}

// SendDelegateSignMsgToProxy sends an istanbulDelegateSign message to a proxy
// if one exists
func (sb *Backend) SendDelegateSignMsgToProxy(msg []byte, peerID enode.ID) error {
	if sb.IsProxiedValidator() {
		return sb.proxiedValidatorEngine.SendDelegateSignMsgToProxy(msg, peerID)
	} else {
		return errors.New("No Proxy found")
	}
}

// SendDelegateSignMsgToProxiedValidator sends an istanbulDelegateSign message to a
// proxied validator if one exists
func (sb *Backend) SendDelegateSignMsgToProxiedValidator(msg []byte) error {
	if sb.IsProxy() {
		return sb.proxyEngine.SendDelegateSignMsgToProxiedValidator(msg)
	} else {
		return errors.New("No Proxied Validator found")
	}
}

// Authorize implements istanbul.Backend.Authorize
func (sb *Backend) Authorize(ecdsaAddress, blsAddress common.Address, publicKey *ecdsa.PublicKey, decryptFn istanbul.DecryptFn, signFn istanbul.SignerFn, signBLSFn istanbul.BLSSignerFn, signHashFn istanbul.HashSignerFn) {
	bls := istanbul.NewBlsInfo(blsAddress, signBLSFn)
	ecdsa := istanbul.NewEcdsaInfo(ecdsaAddress, publicKey, decryptFn, signFn, signHashFn)
	w := &istanbul.Wallets{
		Ecdsa: *ecdsa,
		Bls:   *bls,
	}
	sb.aWallets.Store(w)
	sb.core.SetAddress(w.Ecdsa.Address)
}

func (sb *Backend) wallets() *istanbul.Wallets {
	return sb.aWallets.Load().(*istanbul.Wallets)
}

// Address implements istanbul.Backend.Address
func (sb *Backend) Address() common.Address {
	return sb.wallets().Ecdsa.Address
}

// SelfNode returns the owner's node (if this is a proxy, it will return the external node)
func (sb *Backend) SelfNode() *enode.Node {
	return sb.p2pserver.Self()
}

// Close the backend
func (sb *Backend) Close() error {
	sb.delegateSignScope.Close()
	var errs []error
	if err := sb.valEnodeTable.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := sb.announceManager.Close(); err != nil {
		errs = append(errs, err)
	}
	if sb.replicaState != nil {
		if err := sb.replicaState.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := sb.csvRecorder.Close(); err != nil {
		errs = append(errs, err)
	}
	var concatenatedErrs error
	for i, err := range errs {
		if i == 0 {
			concatenatedErrs = err
		} else {
			concatenatedErrs = fmt.Errorf("%v; %v", concatenatedErrs, err)
		}
	}
	return concatenatedErrs
}

// Validators implements istanbul.Backend.Validators
func (sb *Backend) Validators(proposal istanbul.Proposal) istanbul.ValidatorSet {
	return sb.getOrderedValidators(proposal.Number().Uint64(), proposal.Hash())
}

// ParentBlockValidators implements istanbul.Backend.ParentBlockValidators
func (sb *Backend) ParentBlockValidators(proposal istanbul.Proposal) istanbul.ValidatorSet {
	return sb.getOrderedValidators(proposal.Number().Uint64()-1, proposal.ParentHash())
}

func (sb *Backend) NextBlockValidators(proposal istanbul.Proposal) (istanbul.ValidatorSet, error) {
	istExtra, err := types.ExtractIstanbulExtra(proposal.Header())
	if err != nil {
		return nil, err
	}

	// There was no change
	if len(istExtra.AddedValidators) == 0 && istExtra.RemovedValidators.BitLen() == 0 {
		return sb.ParentBlockValidators(proposal), nil
	}

	snap, err := sb.snapshot(sb.chain, proposal.Number().Uint64()-1, common.Hash{}, nil)
	if err != nil {
		return nil, err
	}
	snap = snap.copy()

	addedValidators, err := istanbul.CombineIstanbulExtraToValidatorData(istExtra.AddedValidators, istExtra.AddedValidatorsPublicKeys)
	if err != nil {
		return nil, err
	}

	if !snap.ValSet.RemoveValidators(istExtra.RemovedValidators) {
		return nil, fmt.Errorf("could not obtain next block validators: failed at remove validators")
	}
	if !snap.ValSet.AddValidators(addedValidators) {
		return nil, fmt.Errorf("could not obtain next block validators: failed at add validators")
	}

	return snap.ValSet, nil
}

func (sb *Backend) GetValidators(blockNumber *big.Int, headerHash common.Hash) []istanbul.Validator {
	validatorSet := sb.getValidators(blockNumber.Uint64(), headerHash)
	return validatorSet.List()
}

// Commit implements istanbul.Backend.Commit
func (sb *Backend) Commit(proposal istanbul.Proposal, aggregatedSeal types.IstanbulAggregatedSeal, aggregatedEpochValidatorSetSeal types.IstanbulEpochValidatorSetSeal, result *istanbulCore.StateProcessResult) error {
	// Check if the proposal is a valid block
	block, ok := proposal.(*types.Block)
	if !ok {
		sb.logger.Error("Invalid proposal, %v", proposal)
		return errInvalidProposal
	}

	h := block.Header()
	// Append seals into extra-data
	err := writeAggregatedSeal(h, aggregatedSeal, false)
	if err != nil {
		return err
	}
	// update block's header
	block = block.WithHeader(h)
	block = block.WithEpochSnarkData(&types.EpochSnarkData{
		Bitmap:    aggregatedEpochValidatorSetSeal.Bitmap,
		Signature: aggregatedEpochValidatorSetSeal.Signature,
	})
	if sb.csvRecorder != nil {
		sb.recordBlockProductionTimes(block.Header().Number.Uint64(), len(block.Transactions()), block.GasUsed(), aggregatedSeal.Round.Uint64())
	}

	if sb.abortCommitHook != nil && sb.abortCommitHook(result) {
		return errors.New("nil StateProcessResult")
	}

	sb.logger.Info("Committed", "address", sb.Address(), "round", aggregatedSeal.Round.Uint64(), "hash", proposal.Hash(), "number", proposal.Number().Uint64())

	// If caller didn't provide a result, try verifying the block to produce one
	if result == nil {
		// This is a suboptimal path, since caller is expected to already have a result available
		// and thus to avoid doing the block processing again
		sb.logger.Warn("Potentially duplicated processing for block", "number", block.Number(), "hash", block.Hash())
		if result, _, err = sb.Verify(proposal); err != nil {
			return err
		}
	}
	go sb.onNewConsensusBlock(block, result.Receipts, result.Logs, result.State)

	return nil
}

// EventMux implements istanbul.Backend.EventMux
func (sb *Backend) EventMux() *event.TypeMux {
	return sb.istanbulEventMux
}

// Verify implements istanbul.Backend.Verify
func (sb *Backend) Verify(proposal istanbul.Proposal) (*istanbulCore.StateProcessResult, time.Duration, error) {
	// Check if the proposal is a valid block
	block, ok := proposal.(*types.Block)
	if !ok {
		sb.logger.Error("Invalid proposal, %v", proposal)
		return nil, 0, errInvalidProposal
	}

	// check bad block
	if sb.hasBadProposal(block.Hash()) {
		return nil, 0, core.ErrBlacklistedHash
	}

	// check block body
	txnHash := types.DeriveSha(block.Transactions(), new(trie.Trie))
	if txnHash != block.Header().TxHash {
		return nil, 0, errMismatchTxhashes
	}

	// If the current block occurred before the Donut hard fork, check that the author and coinbase are equal.
	if !sb.chain.Config().IsDonut(block.Number()) {
		addr, err := sb.Author(block.Header())
		if err != nil {
			sb.logger.Error("Could not recover original author of the block to verify against the header's coinbase", "err", err, "func", "Verify")
			return nil, 0, errInvalidProposal
		} else if addr != block.Header().Coinbase {
			sb.logger.Error("Block proposal author and coinbase must be the same when Donut hard fork is active")
			return nil, 0, errInvalidCoinbase
		}
	}

	err := sb.VerifyHeader(sb.chain, block.Header(), false)

	// ignore errEmptyAggregatedSeal error because we don't have the committed seals yet
	if err != nil && err != errEmptyAggregatedSeal {
		if err == consensus.ErrFutureBlock {
			return nil, time.Unix(int64(block.Header().Time), 0).Sub(now()), consensus.ErrFutureBlock
		} else {
			return nil, 0, err
		}
	}

	// Process the block to verify that the transactions are valid and to retrieve the resulting state and receipts
	// Get the state from this block's parent.
	state, err := sb.stateAt(block.Header().ParentHash)
	if err != nil {
		sb.logger.Error("verify - Error in getting the block's parent's state", "parentHash", block.Header().ParentHash.Hex(), "err", err)
		return nil, 0, err
	}

	// Apply this block's transactions to update the state
	receipts, logs, usedGas, err := sb.processBlock(block, state)
	if err != nil {
		sb.logger.Error("verify - Error in processing the block", "err", err)
		return nil, 0, err
	}

	// Validate the block
	if err := sb.validateState(block, state, receipts, usedGas); err != nil {
		sb.logger.Error("verify - Error in validating the block", "err", err)
		return nil, 0, err
	}

	// verify the validator set diff if this is the last block of the epoch
	if istanbul.IsLastBlockOfEpoch(block.Header().Number.Uint64(), sb.config.Epoch) {
		if err := sb.verifyValSetDiff(proposal, block, state); err != nil {
			sb.logger.Error("verify - Error in verifying the val set diff", "err", err)
			return nil, 0, err
		}
	}

	result := &istanbulCore.StateProcessResult{Receipts: receipts, Logs: logs, State: state}
	return result, 0, nil
}

func (sb *Backend) getNewValidatorSet(header *types.Header, state *state.StateDB) ([]istanbul.ValidatorData, error) {
	vmRunner := sb.chain.NewEVMRunner(header, state)
	newValSetAddresses, err := election.GetElectedValidators(vmRunner)
	if err != nil {
		return nil, err
	}
	newValSet, err := validators.GetValidatorData(vmRunner, newValSetAddresses)
	return newValSet, err
}

func (sb *Backend) verifyValSetDiff(proposal istanbul.Proposal, block *types.Block, state *state.StateDB) error {
	header := block.Header()

	// Ensure that the extra data format is satisfied
	istExtra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		return err
	}

	newValSet, err := sb.getNewValidatorSet(block.Header(), state)
	if err != nil {
		if len(istExtra.AddedValidators) != 0 || istExtra.RemovedValidators.BitLen() != 0 {
			sb.logger.Error("verifyValSetDiff - Invalid val set diff.  Non empty diff when it should be empty.", "addedValidators", common.ConvertToStringSlice(istExtra.AddedValidators), "removedValidators", istExtra.RemovedValidators.Text(16))
			return errInvalidValidatorSetDiff
		}
	} else {
		parentValidators := sb.ParentBlockValidators(proposal)
		oldValSet := make([]istanbul.ValidatorData, 0, parentValidators.Size())

		for _, val := range parentValidators.List() {
			oldValSet = append(oldValSet, istanbul.ValidatorData{
				Address:      val.Address(),
				BLSPublicKey: val.BLSPublicKey(),
			})
		}

		addedValidators, removedValidators := istanbul.ValidatorSetDiff(oldValSet, newValSet)

		addedValidatorsAddresses := make([]common.Address, 0, len(addedValidators))
		addedValidatorsPublicKeys := make([]blscrypto.SerializedPublicKey, 0, len(addedValidators))
		for _, val := range addedValidators {
			addedValidatorsAddresses = append(addedValidatorsAddresses, val.Address)
			addedValidatorsPublicKeys = append(addedValidatorsPublicKeys, val.BLSPublicKey)
		}

		if !istanbul.CompareValidatorSlices(addedValidatorsAddresses, istExtra.AddedValidators) || removedValidators.Cmp(istExtra.RemovedValidators) != 0 || !istanbul.CompareValidatorPublicKeySlices(addedValidatorsPublicKeys, istExtra.AddedValidatorsPublicKeys) {
			sb.logger.Error("verifyValSetDiff - Invalid val set diff. Comparison failed. ", "got addedValidators", common.ConvertToStringSlice(istExtra.AddedValidators), "got removedValidators", istExtra.RemovedValidators.Text(16), "got addedValidatorsPublicKeys", istanbul.ConvertPublicKeysToStringSlice(istExtra.AddedValidatorsPublicKeys), "expected addedValidators", common.ConvertToStringSlice(addedValidatorsAddresses), "expected removedValidators", removedValidators.Text(16), "expected addedValidatorsPublicKeys", istanbul.ConvertPublicKeysToStringSlice(addedValidatorsPublicKeys))
			return errInvalidValidatorSetDiff
		}
	}

	return nil
}

// Sign implements istanbul.Backend.Sign
func (sb *Backend) Sign(data []byte) ([]byte, error) {
	return sb.wallets().Ecdsa.Sign(data)
}

// Sign implements istanbul.Backend.SignBLS
func (sb *Backend) SignBLS(data []byte, extra []byte, useComposite, cip22 bool) (blscrypto.SerializedSignature, error) {
	w := sb.wallets()
	return w.Bls.Sign(data, extra, useComposite, cip22)
}

// CheckSignature implements istanbul.Backend.CheckSignature
func (sb *Backend) CheckSignature(data []byte, address common.Address, sig []byte) error {
	signer, err := istanbul.GetSignatureAddress(data, sig)
	if err != nil {
		sb.logger.Error("Failed to get signer address", "err", err)
		return err
	}
	// Compare derived addresses
	if signer != address {
		return errInvalidSignature
	}
	return nil
}

// HasBlock implements istanbul.Backend.HasBlock
func (sb *Backend) HasBlock(hash common.Hash, number *big.Int) bool {
	return sb.chain.GetHeader(hash, number.Uint64()) != nil
}

// AuthorForBlock returns the address of the block offer from a given number.
func (sb *Backend) AuthorForBlock(number uint64) common.Address {
	if h := sb.chain.GetHeaderByNumber(number); h != nil {
		a, _ := sb.Author(h)
		return a
	}
	return common.ZeroAddress
}

// HashForBlock returns the block hash from the canonical chain for the given number.
func (sb *Backend) HashForBlock(number uint64) common.Hash {
	if h := sb.chain.GetHeaderByNumber(number); h != nil {
		return h.Hash()
	}
	return common.Hash{}
}

func (sb *Backend) getValidators(number uint64, hash common.Hash) istanbul.ValidatorSet {
	snap, err := sb.snapshot(sb.chain, number, hash, nil)
	if err != nil {
		sb.logger.Warn("Error getting snapshot", "number", number, "hash", hash, "err", err)
		return validator.NewSet(nil)
	}
	return snap.ValSet
}

// validatorRandomnessAtBlockNumber calls into the EVM to get the randomness to use in proposer ordering at a given block.
func (sb *Backend) validatorRandomnessAtBlockNumber(number uint64, hash common.Hash) (common.Hash, error) {
	lastBlockInPreviousEpoch := number
	if number > 0 {
		lastBlockInPreviousEpoch = number - istanbul.GetNumberWithinEpoch(number, sb.config.Epoch)
	}
	vmRunner, err := sb.chain.NewEVMRunnerForCurrentBlock()
	if err != nil {
		return common.Hash{}, err
	}
	return random.BlockRandomness(vmRunner, lastBlockInPreviousEpoch)
}

func (sb *Backend) getOrderedValidators(number uint64, hash common.Hash) istanbul.ValidatorSet {
	valSet := sb.getValidators(number, hash)
	if valSet.Size() == 0 {
		return valSet
	}

	if sb.config.ProposerPolicy == istanbul.ShuffledRoundRobin {
		seed, err := sb.validatorRandomnessAtBlockNumber(number, hash)
		if err != nil {
			if err == contracts.ErrRegistryContractNotDeployed {
				sb.logger.Debug("Failed to set randomness for proposer selection", "block_number", number, "hash", hash, "error", err)
			} else {
				sb.logger.Warn("Failed to set randomness for proposer selection", "block_number", number, "hash", hash, "error", err)
			}
		}
		valSet.SetRandomness(seed)
	}

	return valSet
}

// GetCurrentHeadBlock retrieves the last block
func (sb *Backend) GetCurrentHeadBlock() istanbul.Proposal {
	return sb.currentBlock()
}

// GetCurrentHeadBlockAndAuthor retrieves the last block alongside the coinbase address for it
func (sb *Backend) GetCurrentHeadBlockAndAuthor() (istanbul.Proposal, common.Address) {
	block := sb.currentBlock()

	if block.Number().Cmp(common.Big0) == 0 {
		return block, common.ZeroAddress
	}

	proposer, err := sb.Author(block.Header())

	if err != nil {
		sb.logger.Error("Failed to get block proposer", "err", err)
		return nil, common.ZeroAddress
	}

	// Return header only block here since we don't need block body
	return block, proposer
}

func (sb *Backend) LastSubject() (istanbul.Subject, error) {
	lastProposal, _ := sb.GetCurrentHeadBlockAndAuthor()
	istExtra, err := types.ExtractIstanbulExtra(lastProposal.Header())
	if err != nil {
		return istanbul.Subject{}, err
	}
	lastView := &istanbul.View{Sequence: lastProposal.Number(), Round: istExtra.AggregatedSeal.Round}
	return istanbul.Subject{View: lastView, Digest: lastProposal.Hash()}, nil
}

func (sb *Backend) hasBadProposal(hash common.Hash) bool {
	if sb.hasBadBlock == nil {
		return false
	}
	return sb.hasBadBlock(hash)
}

// RefreshValPeers will create 'validator' type peers to all the valset validators, and disconnect from the
// peers that are not part of the valset.
// It will also disconnect all validator connections if this node is not a validator.
// Note that adding and removing validators are idempotent operations.  If the validator
// being added or removed is already added or removed, then a no-op will be done.
func (sb *Backend) RefreshValPeers() error {
	logger := sb.logger.New("func", "RefreshValPeers")
	logger.Trace("Called RefreshValPeers")

	if sb.broadcaster == nil {
		return errors.New("Broadcaster is not set")
	}

	valConnSet, err := sb.RetrieveValidatorConnSet()
	if err != nil {
		return err
	}
	sb.valEnodeTable.RefreshValPeers(valConnSet, sb.ValidatorAddress())
	return nil
}

func (sb *Backend) ValidatorAddress() common.Address {
	if sb.IsProxy() {
		return sb.config.ProxiedValidatorAddress
	}
	return sb.Address()
}

// RetrieveValidatorConnSet returns the cached validator conn set if the cache
// is younger than 20 blocks, younger than 1 minute, or if an epoch transition didn't occur since the last
// cached entry. In the event of a cache miss, this may block for a
// couple seconds while retrieving the uncached set.
func (sb *Backend) RetrieveValidatorConnSet() (map[common.Address]bool, error) {
	var valConnSetToReturn map[common.Address]bool = nil

	sb.cachedValidatorConnSetMu.RLock()

	// wait period in blocks
	waitPeriod := uint64(20)

	// wait period in seconds
	waitPeriodSec := 60 * time.Second

	// Check to see if there is a cached validator conn set
	if sb.cachedValidatorConnSet != nil {
		currentBlockNum := sb.currentBlock().Number().Uint64()
		pendingBlockNum := currentBlockNum + 1

		// We want to get the val conn set that is meant to validate the pending block
		desiredValSetEpochNum := istanbul.GetEpochNumber(pendingBlockNum, sb.config.Epoch)

		// Note that the cached validator conn set is applicable for the block right after the cached block num
		cachedEntryEpochNum := istanbul.GetEpochNumber(sb.cachedValidatorConnSetBlockNum+1, sb.config.Epoch)

		// Returned the cached entry if it's within the same current epoch and that it's within waitPeriod
		// blocks of the pending block.
		if cachedEntryEpochNum == desiredValSetEpochNum && (sb.cachedValidatorConnSetBlockNum+waitPeriod) > currentBlockNum && time.Since(sb.cachedValidatorConnSetTS) <= waitPeriodSec {
			valConnSetToReturn = sb.cachedValidatorConnSet
		}
	}

	if valConnSetToReturn == nil {
		sb.cachedValidatorConnSetMu.RUnlock()

		if err := sb.updateCachedValidatorConnSet(); err != nil {
			return nil, err
		}

		sb.cachedValidatorConnSetMu.RLock()
		valConnSetToReturn = sb.cachedValidatorConnSet
	}

	valConnSetCopy := make(map[common.Address]bool)
	for address, inSet := range valConnSetToReturn {
		valConnSetCopy[address] = inSet
	}
	defer sb.cachedValidatorConnSetMu.RUnlock()
	return valConnSetCopy, nil
}

// retrieveCachedValidatorConnSet returns the most recently cached validator conn set.
// If no set has ever been cached, nil is returned.
func (sb *Backend) retrieveCachedValidatorConnSet() map[common.Address]bool {
	sb.cachedValidatorConnSetMu.RLock()
	defer sb.cachedValidatorConnSetMu.RUnlock()
	return sb.cachedValidatorConnSet
}

// updateCachedValidatorConnSet updates the cached validator conn set. If another
// goroutine is simultaneously updating the cached set, this goroutine will wait
// for the update to be finished to prevent the update work from occurring
// simultaneously.
func (sb *Backend) updateCachedValidatorConnSet() (err error) {
	logger := sb.logger.New("func", "updateCachedValidatorConnSet")
	var waited bool
	sb.updatingCachedValidatorConnSetCond.L.Lock()
	// Checking the condition in a for loop as recommended to prevent a race
	for sb.updatingCachedValidatorConnSet {
		waited = true
		logger.Trace("Waiting for another goroutine to update the set")
		// If another goroutine is updating, wait for it to finish
		sb.updatingCachedValidatorConnSetCond.Wait()
	}
	if waited {
		defer sb.updatingCachedValidatorConnSetCond.L.Unlock()
		return sb.updatingCachedValidatorConnSetErr
	}
	// If we didn't wait, we show that we will start updating the cache
	sb.updatingCachedValidatorConnSet = true
	sb.updatingCachedValidatorConnSetCond.L.Unlock()

	defer func() {
		sb.updatingCachedValidatorConnSetCond.L.Lock()
		sb.updatingCachedValidatorConnSet = false
		// Share the error with other goroutines that are waiting on this one
		sb.updatingCachedValidatorConnSetErr = err
		// Broadcast to any waiting goroutines that the update is complete
		sb.updatingCachedValidatorConnSetCond.Broadcast()
		sb.updatingCachedValidatorConnSetCond.L.Unlock()
	}()

	validatorConnSet, blockNum, connSetTS, err := sb.retrieveUncachedValidatorConnSet()
	if err != nil {
		return err
	}
	sb.cachedValidatorConnSetMu.Lock()
	sb.cachedValidatorConnSet = validatorConnSet
	sb.cachedValidatorConnSetBlockNum = blockNum
	sb.cachedValidatorConnSetTS = connSetTS
	sb.cachedValidatorConnSetMu.Unlock()
	return nil
}

func (sb *Backend) retrieveUncachedValidatorConnSet() (map[common.Address]bool, uint64, time.Time, error) {
	logger := sb.logger.New("func", "retrieveUncachedValidatorConnSet")
	// Retrieve the validator conn set from the election smart contract
	validatorsSet := make(map[common.Address]bool)

	currentBlock := sb.currentBlock()
	currentState, err := sb.stateAt(currentBlock.Hash())
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	vmRunner := sb.chain.NewEVMRunner(currentBlock.Header(), currentState)
	electNValidators, err := election.ElectNValidatorSigners(vmRunner, sb.config.AnnounceAdditionalValidatorsToGossip)

	// The validator contract may not be deployed yet.
	// Even if it is deployed, it may not have any registered validators yet.
	if err == contracts.ErrSmartContractNotDeployed || err == contracts.ErrRegistryContractNotDeployed {
		logger.Trace("Can't elect N validators because smart contract not deployed. Setting validator conn set to current elected validators.", "err", err)
	} else if err != nil {
		logger.Error("Error in electing N validators. Setting validator conn set to current elected validators", "err", err)
	}

	for _, address := range electNValidators {
		validatorsSet[address] = true
	}

	// Add active validators regardless
	valSet := sb.getValidators(currentBlock.Number().Uint64(), currentBlock.Hash())
	for _, val := range valSet.List() {
		validatorsSet[val.Address()] = true
	}

	connSetTS := time.Now()

	logger.Trace("Returning validator conn set", "validatorsSet", validatorsSet)
	return validatorsSet, currentBlock.Number().Uint64(), connSetTS, nil
}

func (sb *Backend) AddProxy(node, externalNode *enode.Node) error {
	if sb.IsProxiedValidator() {
		return sb.proxiedValidatorEngine.AddProxy(node, externalNode)
	} else {
		return proxy.ErrNodeNotProxiedValidator
	}
}

func (sb *Backend) RemoveProxy(node *enode.Node) error {
	if sb.IsProxiedValidator() {
		return sb.proxiedValidatorEngine.RemoveProxy(node)
	} else {
		return proxy.ErrNodeNotProxiedValidator
	}
}

// VerifyPendingBlockValidatorSignature will verify that the message sender is a validator that is responsible
// for the current pending block (the next block right after the head block).
func (sb *Backend) VerifyPendingBlockValidatorSignature(data []byte, sig []byte) (common.Address, error) {
	block := sb.currentBlock()
	valSet := sb.getValidators(block.Number().Uint64(), block.Hash())
	return istanbul.CheckValidatorSignature(valSet, data, sig)
}

// VerifyValidatorConnectionSetSignature will verify that the message sender is a validator that is responsible
// for the current pending block (the next block right after the head block).
func (sb *Backend) VerifyValidatorConnectionSetSignature(data []byte, sig []byte) (common.Address, error) {
	if valConnSet, err := sb.RetrieveValidatorConnSet(); err != nil {
		return common.Address{}, err
	} else {
		validators := make([]istanbul.ValidatorData, len(valConnSet))
		i := 0
		for address := range valConnSet {
			validators[i].Address = address
			i++
		}

		return istanbul.CheckValidatorSignature(validator.NewSet(validators), data, sig)
	}
}

func (sb *Backend) IsPrimaryForSeq(seq *big.Int) bool {
	if sb.replicaState != nil {
		return sb.replicaState.IsPrimaryForSeq(seq)
	}
	return false
}

func (sb *Backend) IsPrimary() bool {
	if sb.replicaState != nil {
		return sb.replicaState.IsPrimary()
	}
	return false
}

// UpdateReplicaState updates the replica state with the latest seq.
func (sb *Backend) UpdateReplicaState(seq *big.Int) {
	if sb.replicaState != nil {
		sb.replicaState.NewChainHead(seq)
	}
}

// recordBlockProductionTimes records information about the block production cycle and reports it through the CSVRecorder
func (sb *Backend) recordBlockProductionTimes(blockNumber uint64, txCount int, gasUsed, round uint64) {
	cycle := time.Since(sb.cycleStart)
	sb.cycleStart = time.Now()
	sleepGauge := sb.sleepGauge
	consensusGauge := metrics.Get("consensus/istanbul/core/consensus_commit").(metrics.Gauge)
	verifyGauge := metrics.Get("consensus/istanbul/core/verify").(metrics.Gauge)
	blockConstructGauge := metrics.Get("miner/worker/block_construct").(metrics.Gauge)
	cpuSysLoadGauge := metrics.Get("system/cpu/sysload").(metrics.Gauge)
	cpuSysWaitGauge := metrics.Get("system/cpu/syswait").(metrics.Gauge)
	cpuProcLoadGauge := metrics.Get("system/cpu/procload").(metrics.Gauge)

	sb.csvRecorder.Write(blockNumber, txCount, gasUsed, round,
		cycle.Nanoseconds(), sleepGauge.Value(), consensusGauge.Value(), verifyGauge.Value(), blockConstructGauge.Value(),
		cpuSysLoadGauge.Value(), cpuSysWaitGauge.Value(), cpuProcLoadGauge.Value())

}

func (sb *Backend) GetValEnodeTableEntries(valAddresses []common.Address) (map[common.Address]*istanbul.AddressEntry, error) {
	addressEntries, err := sb.valEnodeTable.GetValEnodes(valAddresses)

	if err != nil {
		return nil, err
	}

	returnMap := make(map[common.Address]*istanbul.AddressEntry)

	for address, addressEntry := range addressEntries {
		returnMap[address] = addressEntry
	}

	return returnMap, nil
}

func (sb *Backend) RewriteValEnodeTableEntries(entries map[common.Address]*istanbul.AddressEntry) error {
	addressesToKeep := make(map[common.Address]bool)
	entriesToUpsert := make([]*istanbul.AddressEntry, 0, len(entries))

	for _, entry := range entries {
		addressesToKeep[entry.GetAddress()] = true
		entriesToUpsert = append(entriesToUpsert, entry)
	}

	sb.valEnodeTable.PruneEntries(addressesToKeep)
	sb.valEnodeTable.UpsertVersionAndEnode(entriesToUpsert)

	return nil
}

// AnnounceManager wrapping functions

// GetAnnounceVersion will retrieve the current announce version.
func (sb *Backend) GetAnnounceVersion() uint {
	return sb.announceManager.GetAnnounceVersion()
}

// UpdateAnnounceVersion will asynchronously update the announce version.
func (sb *Backend) UpdateAnnounceVersion() {
	sb.announceManager.UpdateAnnounceVersion()
}

// SetEnodeCertificateMsgMap will verify the given enode certificate message map, then update it on this struct.
func (sb *Backend) SetEnodeCertificateMsgMap(enodeCertMsgMap map[enode.ID]*istanbul.EnodeCertMsg) error {
	return sb.announceManager.SetEnodeCertificateMsgMap(enodeCertMsgMap)
}

// RetrieveEnodeCertificateMsgMap gets the most recent enode certificate messages.
// May be nil if no message was generated as a result of the core not being
// started, or if a proxy has not received a message from its proxied validator
func (sb *Backend) RetrieveEnodeCertificateMsgMap() map[enode.ID]*istanbul.EnodeCertMsg {
	return sb.announceManager.RetrieveEnodeCertificateMsgMap()
}

// StartAnnouncing implements consensus.Istanbul.StartAnnouncing
func (sb *Backend) StartAnnouncing() error {
	return sb.announceManager.StartAnnouncing(func() error {
		return sb.vph.startThread()
	})
}

// StopAnnouncing implements consensus.Istanbul.StopAnnouncing
func (sb *Backend) StopAnnouncing() error {
	return sb.announceManager.StopAnnouncing(func() error {
		return sb.vph.stopThread()
	})
}
