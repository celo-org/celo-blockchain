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
	"sync"
	"time"

	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/backend/internal/enodes"
	istanbulCore "github.com/ethereum/go-ethereum/consensus/istanbul/core"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/contract_comm/election"
	comm_errors "github.com/ethereum/go-ethereum/contract_comm/errors"
	"github.com/ethereum/go-ethereum/contract_comm/random"
	"github.com/ethereum/go-ethereum/contract_comm/validators"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
)

const (
	// fetcherID is the ID indicates the block is from Istanbul engine
	fetcherID = "istanbul"
)

var (
	// errInvalidSigningFn is returned when the consensus signing function is invalid.
	errInvalidSigningFn = errors.New("invalid signing function for istanbul messages")

	// errProxyAlreadySet is returned if a user tries to add a proxy that is already set.
	// TODO - When we support multiple sentries per validator, this error will become irrelevant.
	errProxyAlreadySet = errors.New("proxy already set")

	// errNoProxyConnection is returned when a proxied validator is not connected to a proxy
	errNoProxyConnection = errors.New("proxied validator not connected to a proxy")

	// errNoBlockHeader is returned when the requested block header could not be found.
	errNoBlockHeader = errors.New("failed to retrieve block header")
)

// Information about the proxy for a proxied validator
type proxyInfo struct {
	node         *enode.Node    // Enode for the internal network interface
	externalNode *enode.Node    // Enode for the external network interface
	peer         consensus.Peer // Connected proxy peer.  Is nil if this node is not connected to the proxy
}

// New creates an Ethereum backend for Istanbul core engine.
func New(config *istanbul.Config, db ethdb.Database) consensus.Istanbul {
	// Allocate the snapshot caches and create the engine
	logger := log.New()
	recentSnapshots, err := lru.NewARC(inmemorySnapshots)
	if err != nil {
		logger.Crit("Failed to create recent snapshots cache", "err", err)
	}
	peerRecentMessages, err := lru.NewARC(inmemoryPeers)
	if err != nil {
		logger.Crit("Failed to create recent messages cache", "err", err)
	}
	selfRecentMessages, err := lru.NewARC(inmemoryMessages)
	if err != nil {
		logger.Crit("Failed to create known messages cache", "err", err)
	}
	backend := &Backend{
		config:                             config,
		istanbulEventMux:                   new(event.TypeMux),
		logger:                             logger,
		db:                                 db,
		commitCh:                           make(chan *types.Block, 1),
		recentSnapshots:                    recentSnapshots,
		coreStarted:                        false,
		announceRunning:                    false,
		peerRecentMessages:                 peerRecentMessages,
		selfRecentMessages:                 selfRecentMessages,
		announceThreadWg:                   new(sync.WaitGroup),
		announceThreadQuit:                 make(chan struct{}),
		generateAndGossipQueryEnodeCh:      make(chan struct{}, 1),
		updateAnnounceVersionCh:            make(chan struct{}),
		updateAnnounceVersionCompleteCh:    make(chan struct{}),
		lastQueryEnodeGossiped:             make(map[common.Address]time.Time),
		lastVersionCertificatesGossiped:    make(map[common.Address]time.Time),
		valEnodesShareWg:                   new(sync.WaitGroup),
		valEnodesShareQuit:                 make(chan struct{}),
		updatingCachedValidatorConnSetCond: sync.NewCond(&sync.Mutex{}),
		finalizationTimer:                  metrics.NewRegisteredTimer("consensus/istanbul/backend/finalize", nil),
		rewardDistributionTimer:            metrics.NewRegisteredTimer("consensus/istanbul/backend/rewards", nil),
		blocksElectedMeter:                 metrics.NewRegisteredMeter("consensus/istanbul/blocks/elected", nil),
		blocksElectedAndSignedMeter:        metrics.NewRegisteredMeter("consensus/istanbul/blocks/signedbyus", nil),
		blocksElectedButNotSignedMeter:     metrics.NewRegisteredMeter("consensus/istanbul/blocks/missedbyus", nil),
		blocksTotalSigsGauge:               metrics.NewRegisteredGauge("consensus/istanbul/blocks/totalsigs", nil),
		blocksTotalMissedRoundsMeter:       metrics.NewRegisteredMeter("consensus/istanbul/blocks/missedrounds", nil),
	}
	backend.core = istanbulCore.New(backend, backend.config)

	backend.logger = istanbul.NewIstLogger(
		func() *big.Int {
			if backend.core != nil && backend.core.CurrentView() != nil {
				return backend.core.CurrentView().Round
			}
			return common.Big0
		},
	)

	backend.vph = newVPH(backend)
	valEnodeTable, err := enodes.OpenValidatorEnodeDB(config.ValidatorEnodeDBPath, backend.vph)
	if err != nil {
		logger.Crit("Can't open ValidatorEnodeDB", "err", err, "dbpath", config.ValidatorEnodeDBPath)
	}
	backend.valEnodeTable = valEnodeTable

	versionCertificateTable, err := enodes.OpenVersionCertificateDB(config.VersionCertificateDBPath)
	if err != nil {
		logger.Crit("Can't open VersionCertificateDB", "err", err, "dbpath", config.VersionCertificateDBPath)
	}
	backend.versionCertificateTable = versionCertificateTable

	// Set the handler functions for each istanbul message type
	backend.istanbulAnnounceMsgHandlers = make(map[uint64]announceMsgHandler)
	backend.istanbulAnnounceMsgHandlers[istanbulQueryEnodeMsg] = backend.handleQueryEnodeMsg
	backend.istanbulAnnounceMsgHandlers[istanbulValEnodesShareMsg] = backend.handleValEnodesShareMsg
	backend.istanbulAnnounceMsgHandlers[istanbulVersionCertificatesMsg] = backend.handleVersionCertificatesMsg
	backend.istanbulAnnounceMsgHandlers[istanbulEnodeCertificateMsg] = backend.handleEnodeCertificateMsg

	return backend
}

// ----------------------------------------------------------------------------

type Backend struct {
	config           *istanbul.Config
	istanbulEventMux *event.TypeMux

	address          common.Address              // Ethereum address of the signing key
	publicKey        *ecdsa.PublicKey            // The signer public key
	decryptFn        istanbul.DecryptFn          // Decrypt function to decrypt ECIES ciphertext
	signFn           istanbul.SignerFn           // Signer function to authorize hashes with
	signHashBLSFn    istanbul.BLSSignerFn        // Signer function to authorize hashes using BLS with
	signMessageBLSFn istanbul.BLSMessageSignerFn // Signer function to authorize messages using BLS with
	signFnMu         sync.RWMutex                // Protects the signer fields

	core         istanbulCore.Engine
	logger       log.Logger
	db           ethdb.Database
	chain        consensus.ChainReader
	currentBlock func() *types.Block
	hasBadBlock  func(hash common.Hash) bool
	stateAt      func(hash common.Hash) (*state.StateDB, error)

	processBlock  func(block *types.Block, statedb *state.StateDB) (types.Receipts, []*types.Log, uint64, error)
	validateState func(block *types.Block, statedb *state.StateDB, receipts types.Receipts, usedGas uint64) error

	// the channels for istanbul engine notifications
	commitCh          chan *types.Block
	proposedBlockHash common.Hash
	sealMu            sync.Mutex
	coreStarted       bool
	coreMu            sync.RWMutex

	// Snapshots for recent blocks to speed up reorgs
	recentSnapshots *lru.ARCCache

	// event subscription for ChainHeadEvent event
	broadcaster consensus.Broadcaster

	// interface to the p2p server
	p2pserver consensus.P2PServer

	peerRecentMessages *lru.ARCCache // the cache of peer's recent messages
	selfRecentMessages *lru.ARCCache // the cache of self recent messages

	lastQueryEnodeGossiped   map[common.Address]time.Time
	lastQueryEnodeGossipedMu sync.RWMutex

	valEnodeTable *enodes.ValidatorEnodeDB

	versionCertificateTable           *enodes.VersionCertificateDB
	lastVersionCertificatesGossiped   map[common.Address]time.Time
	lastVersionCertificatesGossipedMu sync.RWMutex

	announceRunning               bool
	announceMu                    sync.RWMutex
	announceThreadWg              *sync.WaitGroup
	announceThreadQuit            chan struct{}
	generateAndGossipQueryEnodeCh chan struct{}

	updateAnnounceVersionCh         chan struct{}
	updateAnnounceVersionCompleteCh chan struct{}

	// The enode certificate message most recently generated if this is a validator
	// or received by a proxied validator if this is a proxy.
	// Used for proving itself as a validator in the handshake. The entire
	// istanbul.Message is saved to keep the signature.
	enodeCertificateMsg        *istanbul.Message
	enodeCertificateMsgVersion uint
	enodeCertificateMsgMu      sync.RWMutex

	valEnodesShareWg   *sync.WaitGroup
	valEnodesShareQuit chan struct{}

	// Validator's proxy
	proxyNode *proxyInfo

	// Right now, we assume that there is at most one proxied peer for a proxy
	// Proxy's validator
	proxiedPeer consensus.Peer

	delegateSignFeed  event.Feed
	delegateSignScope event.SubscriptionScope

	// Metric timer used to record block finalization times.
	finalizationTimer metrics.Timer
	// Metric timer used to record epoch reward distribution times.
	rewardDistributionTimer metrics.Timer

	// Meters for number of blocks seen for which the current validator signer has been elected,
	// for which it was elected and has signed, and elected but not signed.
	blocksElectedMeter             metrics.Meter
	blocksElectedAndSignedMeter    metrics.Meter
	blocksElectedButNotSignedMeter metrics.Meter

	// Gauge for total signatures in parentSeal of last received block (how much better than quorum are we doing)
	blocksTotalSigsGauge metrics.Gauge

	// Meter counting cumulative number of round changes that had to happen to get blocks agreed.
	blocksTotalMissedRoundsMeter metrics.Meter

	istanbulAnnounceMsgHandlers map[uint64]announceMsgHandler

	// Cache for the return values of the method retrieveValidatorConnSet
	cachedValidatorConnSet          map[common.Address]bool
	cachedValidatorConnSetTimestamp time.Time
	cachedValidatorConnSetMu        sync.RWMutex

	// Used for ensuring that only one goroutine is doing the work of updating
	// the validator conn set cache at a time.
	updatingCachedValidatorConnSet     bool
	updatingCachedValidatorConnSetErr  error
	updatingCachedValidatorConnSetCond *sync.Cond

	vph *validatorPeerHandler
}

func (sb *Backend) IsProxy() bool {
	return sb.proxiedPeer != nil
}

func (sb *Backend) IsProxiedValidator() bool {
	return sb.proxyNode != nil && sb.proxyNode.peer != nil
}

// SendDelegateSignMsgToProxy sends an istanbulDelegateSign message to a proxy
// if one exists
func (sb *Backend) SendDelegateSignMsgToProxy(msg []byte) error {
	if !sb.IsProxiedValidator() {
		err := errors.New("No Proxy found")
		sb.logger.Error("SendDelegateSignMsgToProxy failed", "err", err)
		return err
	}
	return sb.proxyNode.peer.Send(istanbulDelegateSign, msg)
}

// SendDelegateSignMsgToProxiedValidator sends an istanbulDelegateSign message to a
// proxied validator if one exists
func (sb *Backend) SendDelegateSignMsgToProxiedValidator(msg []byte) error {
	if !sb.IsProxy() {
		err := errors.New("No Proxied Validator found")
		sb.logger.Error("SendDelegateSignMsgToProxiedValidator failed", "err", err)
		return err
	}
	return sb.proxiedPeer.Send(istanbulDelegateSign, msg)
}

// Authorize implements istanbul.Backend.Authorize
func (sb *Backend) Authorize(address common.Address, publicKey *ecdsa.PublicKey, decryptFn istanbul.DecryptFn, signFn istanbul.SignerFn, signHashBLSFn istanbul.BLSSignerFn, signMessageBLSFn istanbul.BLSMessageSignerFn) {
	sb.signFnMu.Lock()
	defer sb.signFnMu.Unlock()

	sb.address = address
	sb.publicKey = publicKey
	sb.decryptFn = decryptFn
	sb.signFn = signFn
	sb.signHashBLSFn = signHashBLSFn
	sb.signMessageBLSFn = signMessageBLSFn
	sb.core.SetAddress(address)
}

// Address implements istanbul.Backend.Address
func (sb *Backend) Address() common.Address {
	return sb.address
}

// Close the backend
func (sb *Backend) Close() error {
	sb.delegateSignScope.Close()
	var errs []error
	if err := sb.valEnodeTable.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := sb.versionCertificateTable.Close(); err != nil {
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

// This function will return the peers with the addresses in the "destAddresses" parameter.
// If this is a proxied validator, then it will return the proxy.
func (sb *Backend) getPeersForMessage(destAddresses []common.Address) map[enode.ID]consensus.Peer {
	if sb.config.Proxied {
		if sb.proxyNode != nil && sb.proxyNode.peer != nil {
			returnMap := make(map[enode.ID]consensus.Peer)
			returnMap[sb.proxyNode.peer.Node().ID()] = sb.proxyNode.peer

			return returnMap
		}
		return nil
	}

	var targets map[enode.ID]bool
	if destAddresses != nil {
		targets = make(map[enode.ID]bool)
		for _, addr := range destAddresses {
			if valNode, err := sb.valEnodeTable.GetNodeFromAddress(addr); valNode != nil && err == nil {
				targets[valNode.ID()] = true
			}
		}
	}
	return sb.broadcaster.FindPeers(targets, p2p.AnyPurpose)
}

// BroadcastConsensusMsg implements istanbul.Backend.BroadcastConsensusMsg
// This function will wrap the consensus message in a fwdMessage if it's a proxied validator.  It will then
// multicast the msg (the wrapped or original version) to the other validators and send it to itself.
func (sb *Backend) BroadcastConsensusMsg(destAddresses []common.Address, payload []byte) error {
	sb.logger.Trace("Broadcasting an istanbul message", "destAddresses", common.ConvertToStringSlice(destAddresses))

	// Send to others
	if err := sb.Multicast(destAddresses, payload, istanbulConsensusMsg); err != nil {
		return err
	}

	// Send to self.  Note that it will never be a wrapped version of the consensus message.
	msg := istanbul.MessageEvent{
		Payload: payload,
	}
	go sb.istanbulEventMux.Post(msg)
	return nil
}

// Multicast implements istanbul.Backend.Multicast
// Multicast will send the eth message (with the message's payload and msgCode field set to the params
// payload and ethMsgCode respectively) to the nodes with the signing address in the destAddresses param.
// If this node is proxied and destAddresses is not nil, the message will be wrapped
// in an istanbul.ForwardMessage to ensure the proxy sends it to the correct
// destAddresses.
// If the destAddresses param is set to nil, then this function will send the message to all connected
// peers.
func (sb *Backend) Multicast(destAddresses []common.Address, payload []byte, ethMsgCode uint64) error {
	logger := sb.logger.New("func", "Multicast")

	if sb.config.Proxied && destAddresses != nil {
		// Convert the message to a fwdMessage
		fwdMessage := &istanbul.ForwardMessage{
			Code:          ethMsgCode,
			DestAddresses: destAddresses,
			Msg:           payload,
		}
		fwdMsgBytes, err := rlp.EncodeToBytes(fwdMessage)
		if err != nil {
			logger.Error("Failed to encode", "fwdMessage", fwdMessage)
			return err
		}

		// Note that we are not signing message.  The message that is being wrapped is already signed.
		msg := istanbul.Message{Code: istanbulFwdMsg, Msg: fwdMsgBytes, Address: sb.Address()}
		fwdMsgPayload, err := msg.Payload()
		if err != nil {
			return err
		}

		return sb.multicast(destAddresses, fwdMsgPayload, istanbulFwdMsg)
	}
	return sb.multicast(destAddresses, payload, ethMsgCode)
}

// multicast will send the eth message (with the message's payload and msgCode field set to the params
// payload and ethMsgCode respectively) to the nodes with the signing address in the destAddresses param.
// If the destAddresses param is set to nil, then this function will send the message to all connected peers.
func (sb *Backend) multicast(destAddresses []common.Address, payload []byte, ethMsgCode uint64) error {
	logger := sb.logger.New("func", "multicast")
	// Get peers to send.
	peers := sb.getPeersForMessage(destAddresses)

	logger.Trace("Going to multicast a message", "peers", peers, "ethMsgCode", ethMsgCode)

	// Only cache for the announceMsg, as that is the only message that is gossiped.
	var hash common.Hash
	if sb.isGossipedMsgCode(ethMsgCode) {
		hash = istanbul.RLPHash(payload)
		sb.selfRecentMessages.Add(hash, true)
	}

	if len(peers) > 0 {
		for _, p := range peers {
			if sb.isGossipedMsgCode(ethMsgCode) {
				nodePubKey := p.Node().Pubkey()
				nodeAddr := crypto.PubkeyToAddress(*nodePubKey)
				ms, ok := sb.peerRecentMessages.Get(nodeAddr)
				var m *lru.ARCCache
				if ok {
					m, _ = ms.(*lru.ARCCache)
					if _, k := m.Get(hash); k {
						// This peer had this event, skip it
						logger.Trace("Message already cached for peer.  Not sending it to peer", "peer", p)
						continue
					}
				} else {
					m, _ = lru.NewARC(inmemoryMessages)
				}

				m.Add(hash, true)
				sb.peerRecentMessages.Add(nodeAddr, m)
			}
			logger.Trace("Sending istanbul message to peer", "peer", p)

			go p.Send(ethMsgCode, payload)
		}
	}
	return nil
}

// Commit implements istanbul.Backend.Commit
func (sb *Backend) Commit(proposal istanbul.Proposal, aggregatedSeal types.IstanbulAggregatedSeal, aggregatedEpochValidatorSetSeal types.IstanbulEpochValidatorSetSeal) error {
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
	block = block.WithSeal(h)
	block = block.WithEpochSnarkData(&types.EpochSnarkData{
		Signature: aggregatedEpochValidatorSetSeal.Signature,
	})

	sb.logger.Info("Committed", "address", sb.Address(), "round", aggregatedSeal.Round.Uint64(), "hash", proposal.Hash(), "number", proposal.Number().Uint64())
	// - if the proposed and committed blocks are the same, send the proposed hash
	//   to commit channel, which is being watched inside the engine.Seal() function.
	// - otherwise, we try to insert the block.
	// -- if success, the ChainHeadEvent event will be broadcasted, try to build
	//    the next block and the previous Seal() will be stopped.
	// -- otherwise, a error will be returned and a round change event will be fired.
	if sb.proposedBlockHash == block.Hash() {
		// feed block hash to Seal() and wait the Seal() result
		sb.commitCh <- block
		return nil
	}

	if sb.broadcaster != nil {
		sb.broadcaster.Enqueue(fetcherID, block)
	}
	return nil
}

// EventMux implements istanbul.Backend.EventMux
func (sb *Backend) EventMux() *event.TypeMux {
	return sb.istanbulEventMux
}

// Verify implements istanbul.Backend.Verify
func (sb *Backend) Verify(proposal istanbul.Proposal) (time.Duration, error) {
	// Check if the proposal is a valid block
	block, ok := proposal.(*types.Block)
	if !ok {
		sb.logger.Error("Invalid proposal, %v", proposal)
		return 0, errInvalidProposal
	}

	// check bad block
	if sb.hasBadProposal(block.Hash()) {
		return 0, core.ErrBlacklistedHash
	}

	// check block body
	txnHash := types.DeriveSha(block.Transactions())
	if txnHash != block.Header().TxHash {
		return 0, errMismatchTxhashes
	}

	// The author should be the first person to propose the block to ensure that randomness matches up.
	addr, err := sb.Author(block.Header())
	if err != nil {
		sb.logger.Error("Could not recover orignal author of the block to verify the randomness", "err", err, "func", "Verify")
		return 0, errInvalidProposal
	} else if addr != block.Header().Coinbase {
		sb.logger.Error("Original author of the block does not match the coinbase", "addr", addr, "coinbase", block.Header().Coinbase, "func", "Verify")
		return 0, errInvalidCoinbase
	}

	err = sb.VerifyHeader(sb.chain, block.Header(), false)

	// ignore errEmptyAggregatedSeal error because we don't have the committed seals yet
	if err != nil && err != errEmptyAggregatedSeal {
		if err == consensus.ErrFutureBlock {
			return time.Unix(int64(block.Header().Time), 0).Sub(now()), consensus.ErrFutureBlock
		} else {
			return 0, err
		}
	}

	// Process the block to verify that the transactions are valid and to retrieve the resulting state and receipts
	// Get the state from this block's parent.
	state, err := sb.stateAt(block.Header().ParentHash)
	if err != nil {
		sb.logger.Error("verify - Error in getting the block's parent's state", "parentHash", block.Header().ParentHash.Hex(), "err", err)
		return 0, err
	}

	// Make a copy of the state
	state = state.Copy()

	// Apply this block's transactions to update the state
	receipts, _, usedGas, err := sb.processBlock(block, state)
	if err != nil {
		sb.logger.Error("verify - Error in processing the block", "err", err)
		return 0, err
	}

	// Validate the block
	if err := sb.validateState(block, state, receipts, usedGas); err != nil {
		sb.logger.Error("verify - Error in validating the block", "err", err)
		return 0, err
	}

	// verify the validator set diff if this is the last block of the epoch
	if istanbul.IsLastBlockOfEpoch(block.Header().Number.Uint64(), sb.config.Epoch) {
		if err := sb.verifyValSetDiff(proposal, block, state); err != nil {
			sb.logger.Error("verify - Error in verifying the val set diff", "err", err)
			return 0, err
		}
	}

	return 0, err
}

func (sb *Backend) getNewValidatorSet(header *types.Header, state *state.StateDB) ([]istanbul.ValidatorData, error) {
	newValSetAddresses, err := election.GetElectedValidators(header, state)
	if err != nil {
		return nil, err
	}
	newValSet, err := validators.GetValidatorData(header, state, newValSetAddresses)
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
	if sb.signFn == nil {
		return nil, errInvalidSigningFn
	}
	sb.signFnMu.RLock()
	defer sb.signFnMu.RUnlock()
	return sb.signFn(accounts.Account{Address: sb.address}, accounts.MimetypeIstanbul, data)
}

func (sb *Backend) SignBlockHeader(data []byte) (blscrypto.SerializedSignature, error) {
	if sb.signHashBLSFn == nil {
		return blscrypto.SerializedSignature{}, errInvalidSigningFn
	}
	sb.signFnMu.RLock()
	defer sb.signFnMu.RUnlock()
	return sb.signHashBLSFn(accounts.Account{Address: sb.address}, data)
}

func (sb *Backend) SignBLSWithCompositeHash(data []byte) (blscrypto.SerializedSignature, error) {
	if sb.signMessageBLSFn == nil {
		return blscrypto.SerializedSignature{}, errInvalidSigningFn
	}
	sb.signFnMu.RLock()
	defer sb.signFnMu.RUnlock()
	// Currently, ExtraData is unused. In the future, it could include data that could be used to introduce
	// "firmware-level" protection. Such data could include data that the SNARK doesn't necessarily need,
	// such as the block number, which can be used by a hardware wallet to see that the block number
	// is incrementing, without having to perform the two-level hashing, just one-level fast hashing.
	return sb.signMessageBLSFn(accounts.Account{Address: sb.address}, data, []byte{})
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

// AuthorForBlock implements istanbul.Backend.AuthorForBlock
func (sb *Backend) AuthorForBlock(number uint64) common.Address {
	if h := sb.chain.GetHeaderByNumber(number); h != nil {
		a, _ := sb.Author(h)
		return a
	}
	return common.ZeroAddress
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
	header := sb.chain.CurrentHeader()
	if header == nil {
		return common.Hash{}, errNoBlockHeader
	}
	state, err := sb.stateAt(header.Hash())
	if err != nil {
		return common.Hash{}, err
	}
	return random.BlockRandomness(header, state, lastBlockInPreviousEpoch)
}

func (sb *Backend) getOrderedValidators(number uint64, hash common.Hash) istanbul.ValidatorSet {
	valSet := sb.getValidators(number, hash)
	if valSet.Size() == 0 {
		return valSet
	}

	if sb.config.ProposerPolicy == istanbul.ShuffledRoundRobin {
		seed, err := sb.validatorRandomnessAtBlockNumber(number, hash)
		if err != nil {
			if err == comm_errors.ErrRegistryContractNotDeployed {
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

func (sb *Backend) addProxy(node, externalNode *enode.Node) error {
	if sb.proxyNode != nil {
		return errProxyAlreadySet
	}
	sb.proxyNode = &proxyInfo{node: node, externalNode: externalNode}
	sb.updateAnnounceVersion()
	sb.p2pserver.AddPeer(node, p2p.ProxyPurpose)
	return nil
}

func (sb *Backend) removeProxy(node *enode.Node) {
	if sb.proxyNode != nil && sb.proxyNode.node.ID() == node.ID() {
		sb.p2pserver.RemovePeer(node, p2p.ProxyPurpose)
		sb.proxyNode = nil
	}
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

	valConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		return err
	}
	sb.valEnodeTable.RefreshValPeers(valConnSet, sb.ValidatorAddress())
	return nil
}

func (sb *Backend) ValidatorAddress() common.Address {
	var localAddress common.Address
	if sb.config.Proxy {
		localAddress = sb.config.ProxiedValidatorAddress
	} else {
		localAddress = sb.Address()
	}
	return localAddress
}

// retrieveValidatorConnSet returns the cached validator conn set if the cache
// is younger than 1 minute. In the event of a cache miss, this may block for a
// couple seconds while retrieving the uncached set.
func (sb *Backend) retrieveValidatorConnSet() (map[common.Address]bool, error) {
	sb.cachedValidatorConnSetMu.RLock()

	waitPeriod := 1 * time.Minute
	if sb.config.Epoch <= 10 {
		waitPeriod = 1 * time.Second
	}
	// Check to see if there is a cached validator conn set, and if it's for the current block
	if sb.cachedValidatorConnSet != nil && time.Since(sb.cachedValidatorConnSetTimestamp) <= waitPeriod {
		defer sb.cachedValidatorConnSetMu.RUnlock()
		return sb.cachedValidatorConnSet, nil
	}
	sb.cachedValidatorConnSetMu.RUnlock()

	if err := sb.updateCachedValidatorConnSet(); err != nil {
		return nil, err
	}

	sb.cachedValidatorConnSetMu.RLock()
	defer sb.cachedValidatorConnSetMu.RUnlock()
	return sb.cachedValidatorConnSet, nil
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

	validatorConnSet, err := sb.retrieveUncachedValidatorConnSet()
	if err != nil {
		return err
	}
	sb.cachedValidatorConnSetMu.Lock()
	sb.cachedValidatorConnSet = validatorConnSet
	sb.cachedValidatorConnSetTimestamp = time.Now()
	sb.cachedValidatorConnSetMu.Unlock()
	return nil
}

func (sb *Backend) retrieveUncachedValidatorConnSet() (map[common.Address]bool, error) {
	logger := sb.logger.New("func", "retrieveUncachedValidatorConnSet")
	// Retrieve the validator conn set from the election smart contract
	validatorsSet := make(map[common.Address]bool)

	currentBlock := sb.currentBlock()
	currentState, err := sb.stateAt(currentBlock.Hash())
	if err != nil {
		return nil, err
	}
	electNValidators, err := election.ElectNValidatorSigners(currentBlock.Header(), currentState, sb.config.AnnounceAdditionalValidatorsToGossip)

	// The validator contract may not be deployed yet.
	// Even if it is deployed, it may not have any registered validators yet.
	if err == comm_errors.ErrSmartContractNotDeployed || err == comm_errors.ErrRegistryContractNotDeployed {
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

	logger.Trace("Returning validator conn set", "validatorsSet", validatorsSet)
	return validatorsSet, nil
}
