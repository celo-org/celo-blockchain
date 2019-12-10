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
	"github.com/ethereum/go-ethereum/consensus/istanbul/backend/internal/enodes"
	istanbulCore "github.com/ethereum/go-ethereum/consensus/istanbul/core"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/contract_comm/election"
	"github.com/ethereum/go-ethereum/contract_comm/random"
	"github.com/ethereum/go-ethereum/contract_comm/validators"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
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

	// errOldAnnounceMessage is returned when the received announce message's block number is earlier
	// than a previous received message
	errOldAnnounceMessage = errors.New("old announce message")
)

// Entries for the recent announce messages
type AnnounceGossipTimestamp struct {
	enodeURLHash      common.Hash
	destAddressesHash common.Hash
	timestamp         time.Time
}

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
	recentMessages, err := lru.NewARC(inmemoryPeers)
	if err != nil {
		logger.Crit("Failed to create recent messages cache", "err", err)
	}
	knownMessages, err := lru.NewARC(inmemoryMessages)
	if err != nil {
		logger.Crit("Failed to create known messages cache", "err", err)
	}
	backend := &Backend{
		config:               config,
		istanbulEventMux:     new(event.TypeMux),
		logger:               logger,
		db:                   db,
		commitCh:             make(chan *types.Block, 1),
		recentSnapshots:      recentSnapshots,
		coreStarted:          false,
		recentMessages:       recentMessages,
		knownMessages:        knownMessages,
		announceWg:           new(sync.WaitGroup),
		announceQuit:         make(chan struct{}),
		lastAnnounceGossiped: make(map[common.Address]*AnnounceGossipTimestamp),
		valEnodesShareWg:     new(sync.WaitGroup),
		valEnodesShareQuit:   make(chan struct{}),
	}
	backend.core = istanbulCore.New(backend, backend.config)

	vph := &validatorPeerHandler{sb: backend}
	table, err := enodes.OpenValidatorEnodeDB(config.ValidatorEnodeDBPath, vph)
	if err != nil {
		logger.Crit("Can't open ValidatorEnodeDB", "err", err, "dbpath", config.ValidatorEnodeDBPath)
	}
	backend.valEnodeTable = table

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
	recentSnapshots *lru.ARCCache

	// event subscription for ChainHeadEvent event
	broadcaster consensus.Broadcaster

	// interface to the p2p server
	p2pserver consensus.P2PServer

	recentMessages *lru.ARCCache // the cache of peer's messages
	knownMessages  *lru.ARCCache // the cache of self messages

	lastAnnounceGossiped   map[common.Address]*AnnounceGossipTimestamp
	lastAnnounceGossipedMu sync.RWMutex

	valEnodeTable *enodes.ValidatorEnodeDB

	announceWg   *sync.WaitGroup
	announceQuit chan struct{}

	valEnodesShareWg   *sync.WaitGroup
	valEnodesShareQuit chan struct{}

	proxyNode *proxyInfo

	// Right now, we assume that there is at most one proxied peer for a proxy
	proxiedPeer consensus.Peer

	newEpochCh chan struct{}

	delegateSignFeed  event.Feed
	delegateSignScope event.SubscriptionScope
}

func (sb *Backend) IsProxy() bool {
	return sb.proxiedPeer != nil
}

func (sb *Backend) IsProxiedValidator() bool {
	return sb.proxyNode != nil
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

// Close the backend
func (sb *Backend) Close() error {
	sb.delegateSignScope.Close()
	return sb.valEnodeTable.Close()
}

// Validators implements istanbul.Backend.Validators
func (sb *Backend) Validators(proposal istanbul.Proposal) istanbul.ValidatorSet {
	return sb.getOrderedValidators(proposal.Number().Uint64(), proposal.Hash())
}

// ParentBlockValidators implements istanbul.Backend.GetParentValidators
func (sb *Backend) ParentBlockValidators(proposal istanbul.Proposal) istanbul.ValidatorSet {
	return sb.getOrderedValidators(proposal.Number().Uint64()-1, proposal.ParentHash())
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
		} else {
			return nil
		}
	} else {
		var targets map[enode.ID]bool = nil

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
}

// Broadcast implements istanbul.Backend.BroadcastConsensusMsg
func (sb *Backend) BroadcastConsensusMsg(destAddresses []common.Address, payload []byte) error {
	sb.logger.Trace("Broadcasting an istanbul message", "destAddresses", common.ConvertToStringSlice(destAddresses))

	// send to others
	if err := sb.Gossip(destAddresses, payload, istanbulConsensusMsg, false); err != nil {
		return err
	}

	// send to self
	msg := istanbul.MessageEvent{
		Payload: payload,
	}
	go sb.istanbulEventMux.Post(msg)
	return nil
}

// Gossip implements istanbul.Backend.Gossip
func (sb *Backend) Gossip(destAddresses []common.Address, payload []byte, ethMsgCode uint64, ignoreCache bool) error {
	// If this is a proxied validator and it wants to send a consensus message,
	// wrap the consensus message in a forward message.
	if sb.config.Proxied && ethMsgCode == istanbulConsensusMsg {
		var err error

		fwdMessage := &istanbul.ForwardMessage{DestAddresses: destAddresses, Msg: payload}
		fwdMsgBytes, err := rlp.EncodeToBytes(fwdMessage)
		if err != nil {
			sb.logger.Error("Failed to encode", "fwdMessage", fwdMessage)
			return err
		}

		// Note that we are not signing message.  The message that is being wrapped is already signed.
		msg := istanbul.Message{Code: istanbulFwdMsg, Msg: fwdMsgBytes, Address: sb.Address()}
		payload, err = msg.Payload()
		if err != nil {
			return err
		}

		ethMsgCode = istanbulFwdMsg
	}

	peers := sb.getPeersForMessage(destAddresses)

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
			sb.logger.Trace("Sending istanbul message to peer", "msg_code", ethMsgCode, "address", addr)

			go p.Send(ethMsgCode, payload)
		}
	}
	return nil
}

// Commit implements istanbul.Backend.Commit
func (sb *Backend) Commit(proposal istanbul.Proposal, aggregatedSeal types.IstanbulAggregatedSeal) error {
	// Check if the proposal is a valid block
	block := &types.Block{}
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
	block := &types.Block{}
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
	if err != nil && err != errEmptyAggregatedSeal {
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
		if len(istExtra.AddedValidators) != 0 || istExtra.RemovedValidators.BitLen() != 0 {
			log.Error("verifyValSetDiff - Invalid val set diff.  Non empty diff when it should be empty.", "addedValidators", common.ConvertToStringSlice(istExtra.AddedValidators), "removedValidators", istExtra.RemovedValidators.Text(16))
			return errInvalidValidatorSetDiff
		}
	} else {
		parentValidators := sb.ParentBlockValidators(proposal)
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
			log.Error("verifyValSetDiff - Invalid val set diff. Comparison failed. ", "got addedValidators", common.ConvertToStringSlice(istExtra.AddedValidators), "got removedValidators", istExtra.RemovedValidators.Text(16), "got addedValidatorsPublicKeys", istanbul.ConvertPublicKeysToStringSlice(istExtra.AddedValidatorsPublicKeys), "expected addedValidators", common.ConvertToStringSlice(addedValidatorsAddresses), "expected removedValidators", removedValidators.Text(16), "expected addedValidatorsPublicKeys", istanbul.ConvertPublicKeysToStringSlice(addedValidatorsPublicKeys))
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
	header := sb.chain.GetHeaderByNumber(lastBlockInPreviousEpoch)
	if header == nil {
		return common.Hash{}, errNoBlockHeader
	}
	state, err := sb.stateAt(header.Hash())
	if err != nil {
		return common.Hash{}, err
	}
	return random.Random(header, state)
}

func (sb *Backend) getOrderedValidators(number uint64, hash common.Hash) istanbul.ValidatorSet {
	valSet := sb.getValidators(number, hash)
	if valSet.Size() == 0 {
		return valSet
	}

	if sb.config.ProposerPolicy == istanbul.ShuffledRoundRobin {
		seed, err := sb.validatorRandomnessAtBlockNumber(number, hash)
		if err != nil {
			sb.logger.Error("Failed to set randomness for proposer selection", "block_number", number, "hash", hash, "error", err)
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

	sb.p2pserver.AddPeer(node, p2p.ProxyPurpose)

	sb.proxyNode = &proxyInfo{node: node, externalNode: externalNode}
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
func (sb *Backend) RefreshValPeers(valset istanbul.ValidatorSet) {
	sb.logger.Trace("Called RefreshValPeers", "valset length", valset.Size())

	if sb.broadcaster == nil {
		return
	}

	sb.valEnodeTable.RefreshValPeers(valset, sb.ValidatorAddress())
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

func (sb *Backend) ConnectToVals() {
	// If this is a proxy, then refresh the val peers.  Note that this will be done within Backend.Start
	// for non proxied validators
	if sb.config.Proxy {
		headBlock := sb.GetCurrentHeadBlock()
		valset := sb.getValidators(headBlock.Number().Uint64(), headBlock.Hash())
		sb.RefreshValPeers(valset)
	}
}
