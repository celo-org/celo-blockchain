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
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	istanbulCore "github.com/ethereum/go-ethereum/consensus/istanbul/core"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/contract_comm/election"
	"github.com/ethereum/go-ethereum/contract_comm/validators"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	lru "github.com/hashicorp/golang-lru"
)

const (
	// fetcherID is the ID indicates the block is from Istanbul engine
	fetcherID = "istanbul"
)

var (
	// errInvalidSigningFn is returned when the consensus signing function is invalid.
	errInvalidSigningFn = errors.New("invalid signing function for istanbul messages")

	// errSentryAlreadySet is returned if a user tries to add a sentry that is already set.
	// TODO - When we support multiple sentries per validator, this error will become irrelevant.
	errSentryAlreadySet = errors.New("sentry already set")
)

// Entries for the recent announce messages
type AnnounceGossipTimestamp struct {
	enodeURLHash common.Hash
	timestamp    time.Time
}

type sentryInfo struct {
	node         *enode.Node
	externalNode *enode.Node
	peer         consensus.Peer
}

// New creates an Ethereum backend for Istanbul core engine.
func New(config *istanbul.Config, db ethdb.Database, dataDir string) consensus.Istanbul {
	// Allocate the snapshot caches and create the engine
	recents, _ := lru.NewARC(inmemorySnapshots)
	recentMessages, _ := lru.NewARC(inmemoryPeers)
	knownMessages, _ := lru.NewARC(inmemoryMessages)
	backend := &Backend{
		config:               config,
		istanbulEventMux:     new(event.TypeMux),
		logger:               log.New(),
		db:                   db,
		commitCh:             make(chan *types.Block, 1),
		recents:              recents,
		coreStarted:          false,
		recentMessages:       recentMessages,
		knownMessages:        knownMessages,
		announceWg:           new(sync.WaitGroup),
		announceQuit:         make(chan struct{}),
		lastAnnounceGossiped: make(map[common.Address]*AnnounceGossipTimestamp),
		valEnodeShareWg:      new(sync.WaitGroup),
		valEnodeShareQuit:    make(chan struct{}),
		dataDir:              dataDir,
	}
	backend.core = istanbulCore.New(backend, backend.config)
	backend.valEnodeTable = newValidatorEnodeTable()
	return backend
}

// ----------------------------------------------------------------------------

type Backend struct {
	config           *istanbul.Config
	istanbulEventMux *event.TypeMux

	address          common.Address           // Ethereum address of the signing key
	signFn           istanbul.SignerFn        // Signer function to authorize hashes with
	signHashBLSFn    istanbul.SignerFn        // Signer function to authorize hashes using BLS with
	signMessageBLSFn istanbul.MessageSignerFn // Signer function to authorize messages using BLS with
	signFnMu         sync.RWMutex             // Protects the signer fields

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
	recents *lru.ARCCache

	// event subscription for ChainHeadEvent event
	broadcaster consensus.Broadcaster

	// interface to the p2p server
	p2pserver consensus.P2PServer

	recentMessages *lru.ARCCache // the cache of peer's messages
	knownMessages  *lru.ARCCache // the cache of self messages

	lastAnnounceGossiped   map[common.Address]*AnnounceGossipTimestamp
	lastAnnounceGossipedMu sync.RWMutex

	valEnodeTable *validatorEnodeTable

	announceWg   *sync.WaitGroup
	announceQuit chan struct{}

	valEnodeShareWg   *sync.WaitGroup
	valEnodeShareQuit chan struct{}

	// TODO: Figure out any needed changes for concurrent changes to this
	// Right now, we assume that there is at most one sentry for a proxied validator
	sentryNode *sentryInfo

	// TODO: Figure out any needed changes for concurrent changes to this
	// Right now, we assume that there is at most one proxied peer for a sentry
	proxiedPeer consensus.Peer

	dataDir string // A read-write data dir to persist files across restarts
}

// Authorize implements istanbul.Backend.Authorize
func (sb *Backend) Authorize(address common.Address, signFn istanbul.SignerFn, signHashBLSFn istanbul.SignerFn, signMessageBLSFn istanbul.MessageSignerFn) {
	sb.signFnMu.Lock()
	defer sb.signFnMu.Unlock()

	sb.address = address
	sb.signFn = signFn
	sb.signHashBLSFn = signHashBLSFn
	sb.signMessageBLSFn = signMessageBLSFn
	sb.core.SetAddress(address)
}

// Address implements istanbul.Backend.Address
func (sb *Backend) Address() common.Address {
	return sb.address
}

func (sb *Backend) Close() error {
	return nil
}

// Validators implements istanbul.Backend.Validators
func (sb *Backend) Validators(proposal istanbul.Proposal) istanbul.ValidatorSet {
	return sb.getValidators(proposal.Number().Uint64(), proposal.Hash())
}

func (sb *Backend) GetValidators(blockNumber *big.Int, headerHash common.Hash) []istanbul.Validator {
	validatorSet := sb.getValidators(blockNumber.Uint64(), headerHash)
	return validatorSet.FilteredList()
}

// Broadcast implements istanbul.Backend.Broadcast
func (sb *Backend) Broadcast(destAddresses []common.Address, istMsg *istanbul.Message, sendMsgToSelf bool, signMsg bool) error {

	// If the validator is proxied, then add the destAddresses to the message so that the sentry
	// will know which addresses to forward the message to
	if sb.config.Proxied {
		istMsg.DestAddresses = destAddresses
	}

	sb.logger.Debug("Called broadcast", "istMsg", istMsg, "destAddresses", common.ConvertToStringSlice(destAddresses))

	return sb.sendMessage(sb.getPeersForMessage(destAddresses), istanbulMsg, istMsg, nil, false, sendMsgToSelf, signMsg)
}

// Gossip implements istanbul.Backend.Gossip
func (sb *Backend) Gossip(ethMsgCode uint64, istMsg *istanbul.Message, payload []byte, ignoreCache bool) error {
	return sb.sendMessage(sb.getPeersForMessage(nil), ethMsgCode, istMsg, payload, ignoreCache, false, true)
}

func (sb *Backend) getPeersForMessage(destAddresses []common.Address) map[enode.ID]consensus.Peer {
	if sb.config.Proxied && sb.sentryNode != nil && sb.sentryNode.peer != nil {
		returnMap := make(map[enode.ID]consensus.Peer)
		returnMap[sb.sentryNode.peer.Node().ID()] = sb.sentryNode.peer

		return returnMap
	} else {
		var targets map[enode.ID]bool = nil

		if destAddresses != nil {
			targets = make(map[enode.ID]bool)
			for _, addr := range destAddresses {
				valNode := sb.valEnodeTable.getUsingAddress(addr)

				if valNode != nil {
					targets[valNode.node.ID()] = true
				}
			}
		}
		return sb.broadcaster.FindPeers(targets, "")
	}
}

func (sb *Backend) sendMessage(peers map[enode.ID]consensus.Peer, ethMsgCode uint64, msg *istanbul.Message, payload []byte, ignoreCache bool, sendMsgToSelf bool, signMsg bool) error {
	ids := []string{}
	enodeURLs := []string{}

	for a, p := range peers {
		ids = append(ids, a.String())
		enodeURLs = append(enodeURLs, p.Node().String())
	}

	sb.logger.Debug("Called sendMessage", "ethMsgCode", ethMsgCode, "ids", ids, "enodeURLs", enodeURLs, "istmsg", msg)
	if msg != nil {
		var err error

		if signMsg {
			// Sign the message
			if err = msg.Sign(sb.Sign); err != nil {
				sb.logger.Error("Error in signing an Istanbul Message", "ethMsgCode", ethMsgCode, "IstanbulMsg", msg.String(), "err", err)
				return err
			}
		}

		// Convert to payload
		payload, err = msg.Payload()
		if err != nil {
			sb.logger.Error("Error in converting Istanbul Message to payload", "IstanbulMsg", msg.String(), "IstMsg", msg.String(), "err", err)
			return err
		}
	}

	var hash common.Hash
	if !ignoreCache {
		hash = istanbul.RLPHash(payload)
		sb.knownMessages.Add(hash, true)
	}

	if len(peers) > 0 {
		for addr, p := range peers {
			if !ignoreCache {
				ms, ok := sb.recentMessages.Get(addr)
				var m *lru.ARCCache
				if ok {
					m, _ = ms.(*lru.ARCCache)
					if _, k := m.Get(hash); k {
						// This peer had this event, skip it
						continue
					}
				} else {
					m, _ = lru.NewARC(inmemoryMessages)
				}

				m.Add(hash, true)
				sb.recentMessages.Add(addr, m)
			}

			go p.Send(ethMsgCode, payload)
		}
	}

	if sendMsgToSelf {
		msg := istanbul.MessageEvent{
			Payload: payload,
		}
		go sb.istanbulEventMux.Post(msg)
	}

	return nil
}

func (sb *Backend) Enode() *enode.Node {
	return sb.p2pserver.Self()
}

func (sb *Backend) GetDataDir() string {
	return sb.dataDir
}

// Commit implements istanbul.Backend.Commit
func (sb *Backend) Commit(proposal istanbul.Proposal, bitmap *big.Int, seals []byte) error {
	// Check if the proposal is a valid block
	block := &types.Block{}
	block, ok := proposal.(*types.Block)
	if !ok {
		sb.logger.Error("Invalid proposal, %v", proposal)
		return errInvalidProposal
	}

	h := block.Header()
	// Append seals into extra-data
	err := writeCommittedSeals(h, bitmap, seals)
	if err != nil {
		return err
	}
	// update block's header
	block = block.WithSeal(h)

	sb.logger.Info("Committed", "address", sb.Address(), "hash", proposal.Hash(), "number", proposal.Number().Uint64())
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
	block := &types.Block{}
	block, ok := proposal.(*types.Block)
	if !ok {
		sb.logger.Error("Invalid proposal, %v", proposal)
		return 0, errInvalidProposal
	}

	// check bad block
	if sb.HasBadProposal(block.Hash()) {
		return 0, core.ErrBlacklistedHash
	}

	// check block body
	txnHash := types.DeriveSha(block.Transactions())
	uncleHash := types.CalcUncleHash(block.Uncles())
	if txnHash != block.Header().TxHash {
		return 0, errMismatchTxhashes
	}
	if uncleHash != nilUncleHash {
		return 0, errInvalidUncleHash
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

	// ignore errEmptyCommittedSeals error because we don't have the committed seals yet
	if err != nil && err != errEmptyCommittedSeals {
		if err == consensus.ErrFutureBlock {
			return time.Unix(block.Header().Time.Int64(), 0).Sub(now()), consensus.ErrFutureBlock
		} else {
			return 0, err
		}
	}

	// Process the block to verify that the transactions are valid and to retrieve the resulting state and receipts
	// Get the state from this block's parent.
	state, err := sb.stateAt(block.Header().ParentHash)
	if err != nil {
		log.Error("verify - Error in getting the block's parent's state", "parentHash", block.Header().ParentHash.Hex(), "err", err)
		return 0, err
	}

	// Make a copy of the state
	state = state.Copy()

	// Apply this block's transactions to update the state
	receipts, _, usedGas, err := sb.processBlock(block, state)
	if err != nil {
		log.Error("verify - Error in processing the block", "err", err)
		return 0, err
	}

	// Validate the block
	if err := sb.validateState(block, state, receipts, usedGas); err != nil {
		log.Error("verify - Error in validating the block", "err", err)
		return 0, err
	}

	// verify the validator set diff if this is the last block of the epoch
	if istanbul.IsLastBlockOfEpoch(block.Header().Number.Uint64(), sb.config.Epoch) {
		if err := sb.verifyValSetDiff(proposal, block, state); err != nil {
			log.Error("verify - Error in verifying the val set diff", "err", err)
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
		log.Error("Istanbul.verifyValSetDiff - Error in retrieving the validator set. Verifying val set diff empty.", "err", err)
		if len(istExtra.AddedValidators) != 0 || istExtra.RemovedValidators.BitLen() != 0 {
			log.Warn("verifyValSetDiff - Invalid val set diff.  Non empty diff when it should be empty.", "addedValidators", common.ConvertToStringSlice(istExtra.AddedValidators), "removedValidators", istExtra.RemovedValidators.Text(16))
			return errInvalidValidatorSetDiff
		}
	} else {
		parentValidators := sb.ParentValidators(proposal)
		oldValSet := make([]istanbul.ValidatorData, 0, parentValidators.Size())

		for _, val := range parentValidators.List() {
			oldValSet = append(oldValSet, istanbul.ValidatorData{
				val.Address(),
				val.BLSPublicKey(),
			})
		}

		addedValidators, removedValidators := istanbul.ValidatorSetDiff(oldValSet, newValSet)

		addedValidatorsAddresses := make([]common.Address, 0, len(addedValidators))
		addedValidatorsPublicKeys := make([][]byte, 0, len(addedValidators))
		for _, val := range addedValidators {
			addedValidatorsAddresses = append(addedValidatorsAddresses, val.Address)
			addedValidatorsPublicKeys = append(addedValidatorsPublicKeys, val.BLSPublicKey)
		}

		if !istanbul.CompareValidatorSlices(addedValidatorsAddresses, istExtra.AddedValidators) || removedValidators.Cmp(istExtra.RemovedValidators) != 0 || !istanbul.CompareValidatorPublicKeySlices(addedValidatorsPublicKeys, istExtra.AddedValidatorsPublicKeys) {
			log.Warn("verifyValSetDiff - Invalid val set diff. Comparison failed. ", "got addedValidators", common.ConvertToStringSlice(istExtra.AddedValidators), "got removedValidators", istExtra.RemovedValidators.Text(16), "got addedValidatorsPublicKeys", istanbul.ConvertPublicKeysToStringSlice(istExtra.AddedValidatorsPublicKeys), "expected addedValidators", common.ConvertToStringSlice(addedValidatorsAddresses), "expected removedValidators", removedValidators.Text(16), "expected addedValidatorsPublicKeys", istanbul.ConvertPublicKeysToStringSlice(addedValidatorsPublicKeys))
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
	hashData := crypto.Keccak256(data)
	sb.signFnMu.RLock()
	defer sb.signFnMu.RUnlock()
	return sb.signFn(accounts.Account{Address: sb.address}, hashData)
}

func (sb *Backend) SignBlockHeader(data []byte) ([]byte, error) {
	if sb.signHashBLSFn == nil {
		return nil, errInvalidSigningFn
	}
	sb.signFnMu.RLock()
	defer sb.signFnMu.RUnlock()
	return sb.signHashBLSFn(accounts.Account{Address: sb.address}, data)
}

// CheckSignature implements istanbul.Backend.CheckSignature
func (sb *Backend) CheckSignature(data []byte, address common.Address, sig []byte) error {
	signer, err := istanbul.GetSignatureAddress(data, sig)
	if err != nil {
		log.Error("Failed to get signer address", "err", err)
		return err
	}
	// Compare derived addresses
	if signer != address {
		return errInvalidSignature
	}
	return nil
}

// HasProposal implements istanbul.Backend.HasProposal
func (sb *Backend) HasProposal(hash common.Hash, number *big.Int) bool {
	return sb.chain.GetHeader(hash, number.Uint64()) != nil
}

// GetProposer implements istanbul.Backend.GetProposer
func (sb *Backend) GetProposer(number uint64) common.Address {
	if h := sb.chain.GetHeaderByNumber(number); h != nil {
		a, _ := sb.Author(h)
		return a
	}
	return common.Address{}
}

// ParentValidators implements istanbul.Backend.GetParentValidators
func (sb *Backend) ParentValidators(proposal istanbul.Proposal) istanbul.ValidatorSet {
	if block, ok := proposal.(*types.Block); ok {
		return sb.getValidators(block.Number().Uint64()-1, block.ParentHash())
	}
	return validator.NewSet(nil, sb.config.ProposerPolicy)
}

func (sb *Backend) getValidators(number uint64, hash common.Hash) istanbul.ValidatorSet {
	snap, err := sb.snapshot(sb.chain, number, hash, nil)
	if err != nil {
		return validator.NewSet(nil, sb.config.ProposerPolicy)
	}
	return snap.ValSet
}

func (sb *Backend) LastProposal() (istanbul.Proposal, common.Address) {
	block := sb.currentBlock()

	var proposer common.Address
	if block.Number().Cmp(common.Big0) > 0 {
		var err error
		proposer, err = sb.Author(block.Header())
		if err != nil {
			sb.logger.Error("Failed to get block proposer", "err", err)
			return nil, common.Address{}
		}
	}

	// Return header only block here since we don't need block body
	return block, proposer
}

func (sb *Backend) HasBadProposal(hash common.Hash) bool {
	if sb.hasBadBlock == nil {
		return false
	}
	return sb.hasBadBlock(hash)
}

func (sb *Backend) addSentry(node, externalNode *enode.Node) error {
	if sb.sentryNode != nil {
		return errSentryAlreadySet
	}

	sb.p2pserver.AddPeerLabel(node, "sentry")

	sb.sentryNode = &sentryInfo{node: node, externalNode: externalNode}
	return nil
}

func (sb *Backend) removeSentry(node *enode.Node) {
	if sb.sentryNode != nil && sb.sentryNode.node.ID() == node.ID() {
		sb.p2pserver.RemovePeerLabel(node, "sentry")
		sb.sentryNode = nil
	}
}

// This will create 'validator' type peers to all the valset validators, and disconnect from the
// peers that are not part of the valset.
// It will also disconnect all validator connections if this node is not a validator.
// Note that adding and removing validators are idempotent operations.  If the validator
// being added or removed is already added or removed, then a no-op will be done.
func (sb *Backend) RefreshValPeers(valset istanbul.ValidatorSet) {
	sb.logger.Trace("Called RefreshValPeers", "valset length", valset.Size())

	currentValPeers := sb.broadcaster.FindPeers(nil, "validator")

	// Disconnect all validator peers if this node is not in the valset
	if _, val := valset.GetByAddress(sb.ValidatorAddress()); val == nil {
		for _, valPeer := range currentValPeers {
			sb.p2pserver.RemovePeerLabel(valPeer.Node(), "validator")
		}
	} else {
		sb.valEnodeTable.refreshValPeers(valset, currentValPeers)
	}
}

func (sb *Backend) ValidatorAddress() common.Address {
	var localAddress common.Address
	if sb.config.Sentry && sb.proxiedPeer != nil {
		localAddress = crypto.PubkeyToAddress(*sb.proxiedPeer.Node().Pubkey())
	} else {
		localAddress = sb.Address()
	}
	return localAddress
}
