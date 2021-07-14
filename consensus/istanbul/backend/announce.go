// Copyright 2017 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package backend

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	vet "github.com/celo-org/celo-blockchain/consensus/istanbul/backend/internal/enodes"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/proxy"
	"github.com/celo-org/celo-blockchain/crypto/ecies"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/rlp"
)

// ==============================================
//
// define the constants and function for the sendAnnounce thread

const (
	queryEnodeGossipCooldownDuration         = 5 * time.Minute
	versionCertificateGossipCooldownDuration = 5 * time.Minute
)

var (
	errInvalidEnodeCertMsgMapInconsistentVersion = errors.New("invalid enode certificate message map because of inconsistent version")

	errNodeMissingEnodeCertificate = errors.New("Node is missing enode certificate")
)

// QueryEnodeGossipFrequencyState specifies how frequently to gossip query enode messages
type QueryEnodeGossipFrequencyState int

const (
	// HighFreqBeforeFirstPeerState will send out a query enode message every 1 minute until the first peer is established
	HighFreqBeforeFirstPeerState QueryEnodeGossipFrequencyState = iota

	// HighFreqAfterFirstPeerState will send out an query enode message every 1 minute for the first 10 query enode messages after the first peer is established.
	// This is on the assumption that when this node first establishes a peer, the p2p network that this node is in may
	// be partitioned with the broader p2p network. We want to give that p2p network some time to connect to the broader p2p network.
	HighFreqAfterFirstPeerState

	// LowFreqState will send out an query every config.AnnounceQueryEnodeGossipPeriod seconds
	LowFreqState
)

// AddressProvider provides the different addresses the announce manager needs
type AddressProvider interface {
	SelfNode() *enode.Node
	ValidatorAddress() common.Address
}

type ProxyContext interface {
	GetProxiedValidatorEngine() proxy.ProxiedValidatorEngine
}

type AnnounceNetwork interface {
	// Gossip gossips protocol messages
	Gossip(payload []byte, ethMsgCode uint64) error
	// RetrieveValidatorConnSet returns the validator connection set
	RetrieveValidatorConnSet() (map[common.Address]bool, error)
	// Multicast will send the eth message (with the message's payload and msgCode field set to the params
	// payload and ethMsgCode respectively) to the nodes with the signing address in the destAddresses param.
	Multicast(destAddresses []common.Address, payload []byte, ethMsgCode uint64, sendToSelf bool) error
}

type AnnounceManagerConfig struct {
	IsProxiedValidator bool
	AWallets           *atomic.Value
	Epoch              uint64 // The number of blocks after which to checkpoint and reset the pending votes
	Announce           *istanbul.AnnounceConfig
}

type PeerCounterFn func(purpose p2p.PurposeFlag) int

type AnnounceManager struct {
	logger log.Logger

	config AnnounceManagerConfig

	addrProvider AddressProvider
	proxyContext ProxyContext
	network      AnnounceNetwork
	peerCounter  PeerCounterFn

	vcGossiper VersionCertificateGossiper

	gossipCache GossipCache

	state  *AnnounceState
	pruner AnnounceStatePruner

	checker ValidatorChecker

	announceVersion         atomic.Value // uint
	updateAnnounceVersionCh chan struct{}

	announceRunning    bool
	announceMu         sync.RWMutex
	announceThreadWg   *sync.WaitGroup
	announceThreadQuit chan struct{}

	// The enode certificate message map contains the most recently generated
	// enode certificates for each external node ID (e.g. will have one entry per proxy
	// for a proxied validator, or just one entry if it's a standalone validator).
	// Each proxy will just have one entry for their own external node ID.
	// Used for proving itself as a validator in the handshake for externally exposed nodes,
	// or by saving latest generated certificate messages by proxied validators to send
	// to their proxies.
	enodeCertificateMsgMap     map[enode.ID]*istanbul.EnodeCertMsg
	enodeCertificateMsgVersion uint
	enodeCertificateMsgMapMu   sync.RWMutex // This protects both enodeCertificateMsgMap and enodeCertificateMsgVersion
}

// NewAnnounceManager creates a new AnnounceManager using the valEnodeTable given. It is
// the responsibility of the caller to close the valEnodeTable, the AnnounceManager will
// not do it.
func NewAnnounceManager(config AnnounceManagerConfig, network AnnounceNetwork, proxyContext ProxyContext,
	addrProvider AddressProvider, state *AnnounceState,
	gossipCache GossipCache,
	peerCounter PeerCounterFn,
	pruner AnnounceStatePruner,
	checker ValidatorChecker) *AnnounceManager {
	vcGossipFunction := func(payload []byte) error {
		return network.Gossip(payload, istanbul.VersionCertificatesMsg)
	}
	am := &AnnounceManager{
		logger:                  log.New("module", "announceManager"),
		config:                  config,
		network:                 network,
		peerCounter:             peerCounter,
		proxyContext:            proxyContext,
		addrProvider:            addrProvider,
		gossipCache:             gossipCache,
		vcGossiper:              NewVcGossiper(vcGossipFunction),
		state:                   state,
		pruner:                  pruner,
		checker:                 checker,
		updateAnnounceVersionCh: make(chan struct{}, 1),
		announceThreadWg:        new(sync.WaitGroup),
		announceRunning:         false,
	}
	return am
}

func (m *AnnounceManager) Close() error {
	// No need to close valEnodeTable since it's a reference,
	// the creator of this announce manager is the responsible for
	// closing it.
	return m.state.versionCertificateTable.Close()
}

func (m *AnnounceManager) wallets() *Wallets {
	return m.config.AWallets.Load().(*Wallets)
}

// The announceThread will:
// 1) Periodically poll to see if this node should be announcing
// 2) Periodically share the entire version certificate table with all peers
// 3) Periodically prune announce-related data structures
// 4) Gossip announce messages periodically when requested
// 5) Update announce version when requested
func (m *AnnounceManager) announceThread() {
	logger := m.logger.New("func", "announceThread")

	// Gossip the announce after a minute.
	// The delay allows for all receivers of the announce message to
	// have a more up-to-date cached registered/elected valset, and
	// hence more likely that they will be aware that this node is
	// within that set.
	waitPeriod := 1 * time.Minute
	if m.config.Epoch <= 10 {
		waitPeriod = 5 * time.Second
	}

	m.announceThreadWg.Add(1)
	defer m.announceThreadWg.Done()
	shouldQueryAndAnnounce := func() (bool, bool) {
		var err error
		shouldQuery, err := m.checker.IsElectedOrNearValidator()
		if err != nil {
			logger.Warn("Error in checking if should announce", err)
			return false, false
		}
		shouldAnnounce := shouldQuery && m.checker.IsValidating()
		return shouldQuery, shouldAnnounce
	}

	updateAnnounceVersion := m.updateAnnounceVersion
	generateAndGossipQueryEnode := m.generateAndGossipQueryEnode
	prune := func() error {
		return m.pruner.Prune(m.state)
	}

	gossipAllVCs := m.gossipAllVCs
	countPeers := m.peerCounter
	st := NewAnnounceTaskState(m.config.Announce)
	for {
		select {
		case <-st.checkIfShouldAnnounceTicker.C:
			logger.Trace("Checking if this node should announce it's enode")
			st.shouldQuery, st.shouldAnnounce = shouldQueryAndAnnounce()
			st.updateAnnounceThreadStatus(logger, waitPeriod, updateAnnounceVersion)

		case <-st.shareVersionCertificatesTicker.C:
			if err := gossipAllVCs(); err != nil {
				m.logger.Warn("Error gossiping all version certificates")
			}

		case <-st.updateAnnounceVersionTickerCh:
			if st.shouldAnnounce {
				updateAnnounceVersion()
			}

		case <-st.queryEnodeTickerCh:
			st.startGossipQueryEnodeTask()

		case <-st.generateAndGossipQueryEnodeCh:
			if st.shouldQuery {
				peers := countPeers(p2p.AnyPurpose)
				st.UpdateFrequencyOnGenerate(peers)
				// This node may have recently sent out an announce message within
				// the gossip cooldown period imposed by other nodes.
				// Regardless, send the queryEnode so that it will at least be
				// processed by this node's peers. This is especially helpful when a network
				// is first starting up.
				if _, err := generateAndGossipQueryEnode(st.queryEnodeFrequencyState == LowFreqState); err != nil {
					logger.Warn("Error in generating and gossiping queryEnode", "err", err)
				}
			}

		case <-m.updateAnnounceVersionCh:
			if st.shouldAnnounce {
				updateAnnounceVersion()
			}

		case <-st.pruneAnnounceDataStructuresTicker.C:
			if err := prune(); err != nil {
				logger.Warn("Error in pruning announce data structures", "err", err)
			}

		case <-m.announceThreadQuit:
			st.OnAnnounceThreadQuitting()
			return
		}
	}
}

func (m *AnnounceManager) updateAnnounceVersion() {
	version := getTimestamp()
	currVersion := m.GetAnnounceVersion()
	if version <= currVersion {
		m.logger.Debug("Announce version is not newer than the existing version", "existing version", currVersion, "attempted new version", version)
		return
	}
	if err := m.setAndShareUpdatedAnnounceVersion(version); err != nil {
		m.logger.Warn("Error updating announce version", "err", err)
		return
	}
	m.logger.Debug("Updating announce version", "announceVersion", version)
	m.announceVersion.Store(version)
}

func (m *AnnounceManager) gossipAllVCs() error {
	return m.vcGossiper.GossipAllFrom(m.state.versionCertificateTable)
}

// getValProxyAssignments returns the remote validator -> external node assignments.
// If this is a standalone validator, it will set the external node to itself.
// If this is a proxied validator, it will set external node to the proxy's external node.
func (m *AnnounceManager) getValProxyAssignments(valAddresses []common.Address) (map[common.Address]*enode.Node, error) {
	var valProxyAssignments map[common.Address]*enode.Node = make(map[common.Address]*enode.Node)
	var selfEnode *enode.Node = m.addrProvider.SelfNode()
	var proxies map[common.Address]*proxy.Proxy // This var is only used if this is a proxied validator

	for _, valAddress := range valAddresses {
		var externalNode *enode.Node

		if m.config.IsProxiedValidator {
			if proxies == nil {
				var err error
				proxies, err = m.proxyContext.GetProxiedValidatorEngine().GetValidatorProxyAssignments(nil)
				if err != nil {
					return nil, err
				}
			}
			proxyObj := proxies[valAddress]
			if proxyObj == nil {
				continue
			}

			externalNode = proxyObj.ExternalNode()
		} else {
			externalNode = selfEnode
		}

		valProxyAssignments[valAddress] = externalNode
	}

	return valProxyAssignments, nil
}

// generateAndGossipAnnounce will generate the lastest announce msg from this node
// and then broadcast it to it's peers, which should then gossip the announce msg
// message throughout the p2p network if there has not been a message sent from
// this node within the last announceGossipCooldownDuration.
// Note that this function must ONLY be called by the announceThread.
func (m *AnnounceManager) generateAndGossipQueryEnode(enforceRetryBackoff bool) (*istanbul.Message, error) {
	logger := m.logger.New("func", "generateAndGossipQueryEnode")
	logger.Trace("generateAndGossipQueryEnode called")

	version := m.GetAnnounceVersion()

	// Retrieve the set valEnodeEntries (and their publicKeys)
	// for the queryEnode message
	valEnodeEntries, err := m.getQueryEnodeValEnodeEntries(enforceRetryBackoff)
	if err != nil {
		return nil, err
	}

	valAddresses := make([]common.Address, len(valEnodeEntries))
	for i, valEnodeEntry := range valEnodeEntries {
		valAddresses[i] = valEnodeEntry.Address
	}
	valProxyAssignments, err := m.getValProxyAssignments(valAddresses)
	if err != nil {
		return nil, err
	}

	var enodeQueries []*enodeQuery
	for _, valEnodeEntry := range valEnodeEntries {
		if valEnodeEntry.PublicKey != nil {
			externalEnode := valProxyAssignments[valEnodeEntry.Address]
			if externalEnode == nil {
				continue
			}

			externalEnodeURL := externalEnode.URLv4()
			enodeQueries = append(enodeQueries, &enodeQuery{
				recipientAddress:   valEnodeEntry.Address,
				recipientPublicKey: valEnodeEntry.PublicKey,
				enodeURL:           externalEnodeURL,
			})
		}
	}

	var qeMsg *istanbul.Message
	if len(enodeQueries) > 0 {
		var err error
		qeMsg, err = m.generateQueryEnodeMsg(version, enodeQueries)
		if err != nil {
			return nil, err
		}

		if qeMsg == nil {
			return nil, nil
		}

		// Convert to payload
		payload, err := qeMsg.Payload()
		if err != nil {
			logger.Error("Error in converting Istanbul QueryEnode Message to payload", "QueryEnodeMsg", qeMsg.String(), "err", err)
			return nil, err
		}

		if err = m.network.Gossip(payload, istanbul.QueryEnodeMsg); err != nil {
			return nil, err
		}

		if err = m.state.valEnodeTable.UpdateQueryEnodeStats(valEnodeEntries); err != nil {
			return nil, err
		}
	}

	return qeMsg, err
}

func (m *AnnounceManager) getQueryEnodeValEnodeEntries(enforceRetryBackoff bool) ([]*istanbul.AddressEntry, error) {
	logger := m.logger.New("func", "getQueryEnodeValEnodeEntries")
	valEnodeEntries, err := m.state.valEnodeTable.GetValEnodes(nil)
	if err != nil {
		return nil, err
	}

	var queryEnodeValEnodeEntries []*istanbul.AddressEntry
	for address, valEnodeEntry := range valEnodeEntries {
		// Don't generate an announce record for ourselves
		if address == m.wallets().Ecdsa.Address {
			continue
		}

		if valEnodeEntry.Version == valEnodeEntry.HighestKnownVersion {
			continue
		}

		if valEnodeEntry.PublicKey == nil {
			logger.Warn("Cannot generate encrypted enode URL for a val enode entry without a PublicKey", "address", address)
			continue
		}

		if enforceRetryBackoff && valEnodeEntry.NumQueryAttemptsForHKVersion > 0 {
			timeoutFactorPow := math.Min(float64(valEnodeEntry.NumQueryAttemptsForHKVersion-1), 5)
			timeoutMinutes := int64(math.Pow(1.5, timeoutFactorPow) * 5)
			timeoutForQuery := time.Duration(timeoutMinutes) * time.Minute

			if time.Since(*valEnodeEntry.LastQueryTimestamp) < timeoutForQuery {
				continue
			}
		}

		queryEnodeValEnodeEntries = append(queryEnodeValEnodeEntries, valEnodeEntry)
	}

	return queryEnodeValEnodeEntries, nil
}

// generateQueryEnodeMsg returns a queryEnode message from this node with a given version.
// A query enode message contains a number of individual enode queries, each of which is intended
// for a single recipient validator. A query contains of this nodes external enode URL, to which
// the recipient validator is intended to connect, and is ECIES encrypted with the recipient's
// public key, from which their validator signer address is derived.
// Note: It is referred to as a "query" because the sender does not know the recipients enode.
// The recipient is expected to respond by opening a direct connection with an enode certificate.
func (m *AnnounceManager) generateQueryEnodeMsg(version uint, enodeQueries []*enodeQuery) (*istanbul.Message, error) {
	logger := m.logger.New("func", "generateQueryEnodeMsg")

	encryptedEnodeURLs, err := m.generateEncryptedEnodeURLs(enodeQueries)
	if err != nil {
		logger.Warn("Error generating encrypted enodeURLs", "err", err)
		return nil, err
	}
	if len(encryptedEnodeURLs) == 0 {
		logger.Trace("No encrypted enodeURLs were generated, will not generate encryptedEnodeMsg")
		return nil, nil
	}
	queryEnodeData := &queryEnodeData{
		EncryptedEnodeURLs: encryptedEnodeURLs,
		Version:            version,
		Timestamp:          getTimestamp(),
	}

	queryEnodeBytes, err := rlp.EncodeToBytes(queryEnodeData)
	if err != nil {
		logger.Error("Error encoding queryEnode content", "QueryEnodeData", queryEnodeData.String(), "err", err)
		return nil, err
	}
	w := m.wallets()

	msg := &istanbul.Message{
		Code:      istanbul.QueryEnodeMsg,
		Msg:       queryEnodeBytes,
		Address:   w.Ecdsa.Address,
		Signature: []byte{},
	}

	// Sign the announce message
	if err := msg.Sign(w.Ecdsa.Sign); err != nil {
		logger.Error("Error in signing a QueryEnode Message", "QueryEnodeMsg", msg.String(), "err", err)
		return nil, err
	}

	logger.Debug("Generated a queryEnode message", "IstanbulMsg", msg.String(), "QueryEnodeData", queryEnodeData.String())

	return msg, nil
}

type enodeQuery struct {
	recipientAddress   common.Address
	recipientPublicKey *ecdsa.PublicKey
	enodeURL           string
}

// generateEncryptedEnodeURLs returns the encryptedEnodeURLs to be sent in an enode query.
func (m *AnnounceManager) generateEncryptedEnodeURLs(enodeQueries []*enodeQuery) ([]*encryptedEnodeURL, error) {
	logger := m.logger.New("func", "generateEncryptedEnodeURLs")

	var encryptedEnodeURLs []*encryptedEnodeURL
	for _, param := range enodeQueries {
		logger.Debug("encrypting enodeURL", "externalEnodeURL", param.enodeURL, "publicKey", param.recipientPublicKey)
		publicKey := ecies.ImportECDSAPublic(param.recipientPublicKey)
		encEnodeURL, err := ecies.Encrypt(rand.Reader, publicKey, []byte(param.enodeURL), nil, nil)
		if err != nil {
			logger.Error("Error in encrypting enodeURL", "enodeURL", param.enodeURL, "publicKey", publicKey)
			return nil, err
		}

		encryptedEnodeURLs = append(encryptedEnodeURLs, &encryptedEnodeURL{
			DestAddress:       param.recipientAddress,
			EncryptedEnodeURL: encEnodeURL,
		})
	}

	return encryptedEnodeURLs, nil
}

// This function will handle a queryEnode message.
func (m *AnnounceManager) handleQueryEnodeMsg(addr common.Address, peer consensus.Peer, payload []byte) error {
	logger := m.logger.New("func", "handleQueryEnodeMsg")

	msg := new(istanbul.Message)

	// Since this is a gossiped messaged, mark that the peer gossiped it (and presumably processed it) and check to see if this node already processed it
	m.gossipCache.MarkMessageProcessedByPeer(addr, payload)
	if m.gossipCache.CheckIfMessageProcessedBySelf(payload) {
		return nil
	}
	defer m.gossipCache.MarkMessageProcessedBySelf(payload)

	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Announce message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger.Trace("Handling a queryEnode message", "from", msg.Address)

	// Check if the sender is within the validator connection set
	validatorConnSet, err := m.network.RetrieveValidatorConnSet()
	if err != nil {
		logger.Trace("Error in retrieving validator connection set", "err", err)
		return err
	}

	if !validatorConnSet[msg.Address] {
		logger.Debug("Received a message from a validator not within the validator connection set. Ignoring it.", "sender", msg.Address)
		return errUnauthorizedAnnounceMessage
	}

	var qeData queryEnodeData
	err = rlp.DecodeBytes(msg.Msg, &qeData)
	if err != nil {
		logger.Warn("Error in decoding received Istanbul QueryEnode message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}

	logger = logger.New("msgAddress", msg.Address, "msgVersion", qeData.Version)

	// Do some validation checks on the queryEnodeData
	if isValid, err := m.validateQueryEnode(msg.Address, &qeData); !isValid || err != nil {
		logger.Warn("Validation of queryEnode message failed", "isValid", isValid, "err", err)
		return err
	}

	// Only elected or nearly elected validators processes the queryEnode message
	shouldProcess, err := m.checker.IsElectedOrNearValidator()
	if err != nil {
		logger.Warn("Error in checking if should process queryEnode", err)
	}

	if shouldProcess {
		logger.Trace("Processing an queryEnode message", "queryEnode records", qeData.EncryptedEnodeURLs)
		w := m.wallets()
		for _, encEnodeURL := range qeData.EncryptedEnodeURLs {
			// Only process an encEnodURL intended for this node
			if encEnodeURL.DestAddress != w.Ecdsa.Address {
				continue
			}
			enodeBytes, err := w.Ecdsa.Decrypt(encEnodeURL.EncryptedEnodeURL)
			if err != nil {
				m.logger.Warn("Error decrypting endpoint", "err", err, "encEnodeURL.EncryptedEnodeURL", encEnodeURL.EncryptedEnodeURL)
				return err
			}
			enodeURL := string(enodeBytes)
			node, err := enode.ParseV4(enodeURL)
			if err != nil {
				logger.Warn("Error parsing enodeURL", "enodeUrl", enodeURL)
				return err
			}

			// queryEnode messages should only be processed once because selfRecentMessages
			// will cache seen queryEnode messages, so it's safe to answer without any throttling
			if err := m.answerQueryEnodeMsg(msg.Address, node, qeData.Version); err != nil {
				logger.Warn("Error answering an announce msg", "target node", node.URLv4(), "error", err)
				return err
			}

			break
		}
	}

	// Regossip this queryEnode message
	return m.regossipQueryEnode(msg, qeData.Version, payload)
}

// answerQueryEnodeMsg will answer a received queryEnode message from an origin
// node. If the origin node is already a peer of any kind, an enodeCertificate will be sent.
// Regardless, the origin node will be upserted into the val enode table
// to ensure this node designates the origin node as a ValidatorPurpose peer.
func (m *AnnounceManager) answerQueryEnodeMsg(address common.Address, node *enode.Node, version uint) error {
	logger := m.logger.New("func", "answerQueryEnodeMsg", "address", address)

	// Get the external enode that this validator is assigned to
	externalEnodeMap, err := m.getValProxyAssignments([]common.Address{address})
	if err != nil {
		logger.Warn("Error in retrieving assigned proxy for remote validator", "address", address, "err", err)
		return err
	}

	// Only answer query when validating
	if externalEnode := externalEnodeMap[address]; externalEnode != nil && m.checker.IsValidating() {
		enodeCertificateMsgs := m.RetrieveEnodeCertificateMsgMap()

		enodeCertMsg := enodeCertificateMsgs[externalEnode.ID()]
		if enodeCertMsg == nil {
			return errNodeMissingEnodeCertificate
		}

		payload, err := enodeCertMsg.Msg.Payload()
		if err != nil {
			logger.Warn("Error getting payload of enode certificate message", "err", err)
			return err
		}

		if err := m.network.Multicast([]common.Address{address}, payload, istanbul.EnodeCertificateMsg, false); err != nil {
			return err
		}
	}

	// Upsert regardless to account for the case that the target is a non-ValidatorPurpose
	// peer but should be.
	// If the target is not a peer and should be a ValidatorPurpose peer, this
	// will designate the target as a ValidatorPurpose peer and send an enodeCertificate
	// during the istanbul handshake.
	if err := m.state.valEnodeTable.UpsertVersionAndEnode([]*istanbul.AddressEntry{{Address: address, Node: node, Version: version}}); err != nil {
		return err
	}
	return nil
}

// validateQueryEnode will do some validation to check the contents of the queryEnode
// message. This is to force all validators that send a queryEnode message to
// create as succint message as possible, and prevent any possible network DOS attacks
// via extremely large queryEnode message.
func (m *AnnounceManager) validateQueryEnode(msgAddress common.Address, qeData *queryEnodeData) (bool, error) {
	logger := m.logger.New("func", "validateQueryEnode", "msg address", msgAddress)

	// Check if there are any duplicates in the queryEnode message
	var encounteredAddresses = make(map[common.Address]bool)
	for _, encEnodeURL := range qeData.EncryptedEnodeURLs {
		if encounteredAddresses[encEnodeURL.DestAddress] {
			logger.Info("QueryEnode message has duplicate entries", "address", encEnodeURL.DestAddress)
			return false, nil
		}

		encounteredAddresses[encEnodeURL.DestAddress] = true
	}

	// Check if the number of rows in the queryEnodePayload is at most 2 times the size of the current validator connection set.
	// Note that this is a heuristic of the actual size of validator connection set at the time the validator constructed the announce message.
	validatorConnSet, err := m.network.RetrieveValidatorConnSet()
	if err != nil {
		return false, err
	}

	if len(qeData.EncryptedEnodeURLs) > 2*len(validatorConnSet) {
		logger.Info("Number of queryEnode message encrypted enodes is more than two times the size of the current validator connection set", "num queryEnode enodes", len(qeData.EncryptedEnodeURLs), "reg/elected val set size", len(validatorConnSet))
		return false, err
	}

	return true, nil
}

// regossipQueryEnode will regossip a received queryEnode message.
// If this node regossiped a queryEnode from the same source address within the last
// 5 minutes, then it won't regossip. This is to prevent a malicious validator from
// DOS'ing the network with very frequent announce messages.
// This opens an attack vector where any malicious node could continue to gossip
// a previously gossiped announce message from any validator, causing other nodes to regossip and
// enforce the cooldown period for future messages originating from the origin validator.
// This is circumvented by caching the hashes of messages that are regossiped
// with sb.selfRecentMessages to prevent future regossips.
func (m *AnnounceManager) regossipQueryEnode(msg *istanbul.Message, msgTimestamp uint, payload []byte) error {
	logger := m.logger.New("func", "regossipQueryEnode", "queryEnodeSourceAddress", msg.Address, "msgTimestamp", msgTimestamp)

	// Don't throttle messages from our own address so that proxies always regossip
	// query enode messages sent from the proxied validator
	if msg.Address != m.addrProvider.ValidatorAddress() {
		if lastGossiped, ok := m.state.lastQueryEnodeGossiped.Get(msg.Address); ok {
			if time.Since(lastGossiped) < queryEnodeGossipCooldownDuration {
				logger.Trace("Already regossiped msg from this source address within the cooldown period, not regossiping.")
				return nil
			}
		}
	}

	logger.Trace("Regossiping the istanbul queryEnode message", "IstanbulMsg", msg.String())
	if err := m.network.Gossip(payload, istanbul.QueryEnodeMsg); err != nil {
		return err
	}

	m.state.lastQueryEnodeGossiped.Set(msg.Address, time.Now())

	return nil
}

// SendVersionCertificateTable sends all VersionCertificates this node
// has to a peer
func (m *AnnounceManager) SendVersionCertificateTable(peer consensus.Peer) error {
	return m.vcGossiper.SendAllFrom(m.state.versionCertificateTable, peer)
}

func (m *AnnounceManager) handleVersionCertificatesMsg(addr common.Address, peer consensus.Peer, payload []byte) error {
	logger := m.logger.New("func", "handleVersionCertificatesMsg")
	logger.Trace("Handling version certificates msg")

	// Since this is a gossiped messaged, mark that the peer gossiped it (and presumably processed it) and check to see if this node already processed it
	m.gossipCache.MarkMessageProcessedByPeer(addr, payload)
	if m.gossipCache.CheckIfMessageProcessedBySelf(payload) {
		return nil
	}
	defer m.gossipCache.MarkMessageProcessedBySelf(payload)

	var msg istanbul.Message
	if err := msg.FromPayload(payload, nil); err != nil {
		logger.Error("Error in decoding version certificates message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger = logger.New("msg address", msg.Address)

	var versionCertificates []*versionCertificate
	if err := rlp.DecodeBytes(msg.Msg, &versionCertificates); err != nil {
		logger.Warn("Error in decoding received version certificates msg", "err", err)
		return err
	}

	// If the announce's valAddress is not within the validator connection set, then ignore it
	validatorConnSet, err := m.network.RetrieveValidatorConnSet()
	if err != nil {
		logger.Trace("Error in retrieving validator conn set", "err", err)
		return err
	}

	var validEntries []*vet.VersionCertificateEntry
	validAddresses := make(map[common.Address]bool)
	// Verify all entries are valid and remove duplicates
	for _, versionCertificate := range versionCertificates {
		// The public key and address are not RLP encoded/decoded and must be
		// explicitly recovered.
		if err := versionCertificate.RecoverPublicKeyAndAddress(); err != nil {
			logger.Warn("Error recovering version certificates public key and address from signature", "err", err)
			continue
		}
		if !validatorConnSet[versionCertificate.Address] {
			logger.Debug("Found version certificate from an address not in the validator conn set", "address", versionCertificate.Address)
			continue
		}
		if _, ok := validAddresses[versionCertificate.Address]; ok {
			logger.Debug("Found duplicate version certificate in message", "address", versionCertificate.Address)
			continue
		}
		validAddresses[versionCertificate.Address] = true
		validEntries = append(validEntries, versionCertificate.Entry())
	}
	if err := m.upsertAndGossipVersionCertificateEntries(validEntries); err != nil {
		logger.Warn("Error upserting and gossiping entries", "err", err)
		return err
	}
	return nil
}

func (m *AnnounceManager) upsertAndGossipVersionCertificateEntries(entries []*vet.VersionCertificateEntry) error {
	logger := m.logger.New("func", "upsertAndGossipVersionCertificateEntries")
	shouldProcess, err := m.checker.IsElectedOrNearValidator()
	if err != nil {
		logger.Warn("Error in checking if should process queryEnode", err)
	}

	if shouldProcess {
		// Update entries in val enode db
		var valEnodeEntries []*istanbul.AddressEntry
		for _, entry := range entries {
			// Don't add ourselves into the val enode table
			if entry.Address == m.wallets().Ecdsa.Address {
				continue
			}
			// Update the HighestKnownVersion for this address. Upsert will
			// only update this entry if the HighestKnownVersion is greater
			// than the existing one.
			// Also store the PublicKey for future encryption in queryEnode msgs
			valEnodeEntries = append(valEnodeEntries, &istanbul.AddressEntry{
				Address:             entry.Address,
				PublicKey:           entry.PublicKey,
				HighestKnownVersion: entry.Version,
			})
		}
		if err := m.state.valEnodeTable.UpsertHighestKnownVersion(valEnodeEntries); err != nil {
			logger.Warn("Error upserting val enode table entries", "err", err)
		}
	}

	newEntries, err := m.state.versionCertificateTable.Upsert(entries)
	if err != nil {
		logger.Warn("Error upserting version certificate table entries", "err", err)
	}

	// Only regossip entries that do not originate from an address that we have
	// gossiped a version certificate for within the last 5 minutes, excluding
	// our own address.
	var versionCertificatesToRegossip []*versionCertificate

	for _, entry := range newEntries {
		lastGossipTime, ok := m.state.lastVersionCertificatesGossiped.Get(entry.Address)
		if ok && time.Since(lastGossipTime) >= versionCertificateGossipCooldownDuration && entry.Address != m.addrProvider.ValidatorAddress() {
			continue
		}
		versionCertificatesToRegossip = append(versionCertificatesToRegossip, newVersionCertificateFromEntry(entry))
		m.state.lastVersionCertificatesGossiped.Set(entry.Address, time.Now())
	}

	if len(versionCertificatesToRegossip) > 0 {
		return m.vcGossiper.Gossip(versionCertificatesToRegossip)
	}
	return nil
}

// UpdateAnnounceVersion will asynchronously update the announce version.
func (m *AnnounceManager) UpdateAnnounceVersion() {
	// Send to the channel iff it does not already have a message.
	select {
	case m.updateAnnounceVersionCh <- struct{}{}:
	default:
	}
}

// GetAnnounceVersion will retrieve the current announce version.
func (m *AnnounceManager) GetAnnounceVersion() uint {
	v := m.announceVersion.Load()
	if v == nil {
		return 0
	}
	return v.(uint)
}

// setAndShareUpdatedAnnounceVersion generates announce data structures and
// and shares them with relevant nodes.
// It will:
//  1) Generate a new enode certificate
//  2) Multicast the new enode certificate to all peers in the validator conn set
//	   * Note: If this is a proxied validator, it's multicast message will be wrapped within a forward
//       message to the proxy, which will in turn send the enode certificate to remote validators.
//  3) Generate a new version certificate
//  4) Gossip the new version certificate to all peers
func (m *AnnounceManager) setAndShareUpdatedAnnounceVersion(version uint) error {
	logger := m.logger.New("func", "setAndShareUpdatedAnnounceVersion")
	// Send new versioned enode msg to all other registered or elected validators
	validatorConnSet, err := m.network.RetrieveValidatorConnSet()
	if err != nil {
		return err
	}
	w := m.wallets()
	// Don't send any of the following messages if this node is not in the validator conn set
	if !validatorConnSet[w.Ecdsa.Address] {
		logger.Trace("Not in the validator conn set, not updating announce version")
		return nil
	}

	enodeCertificateMsgs, err := m.generateEnodeCertificateMsgs(version)
	if err != nil {
		return err
	}

	if len(enodeCertificateMsgs) > 0 {
		if err := m.SetEnodeCertificateMsgMap(enodeCertificateMsgs); err != nil {
			logger.Error("Error in SetEnodeCertificateMsgMap", "err", err)
			return err
		}
	}

	valConnArray := make([]common.Address, 0, len(validatorConnSet))
	for address := range validatorConnSet {
		valConnArray = append(valConnArray, address)
	}

	for _, enodeCertMsg := range enodeCertificateMsgs {
		var destAddresses []common.Address
		if enodeCertMsg.DestAddresses != nil {
			destAddresses = enodeCertMsg.DestAddresses
		} else {
			// Send to all of the addresses in the validator connection set
			destAddresses = valConnArray
		}

		payload, err := enodeCertMsg.Msg.Payload()
		if err != nil {
			logger.Error("Error getting payload of enode certificate message", "err", err)
			return err
		}

		if err := m.network.Multicast(destAddresses, payload, istanbul.EnodeCertificateMsg, false); err != nil {
			return err
		}
	}

	if m.config.IsProxiedValidator {
		m.proxyContext.GetProxiedValidatorEngine().SendEnodeCertsToAllProxies(enodeCertificateMsgs)
	}

	// Generate and gossip a new version certificate
	newVersionCertificate, err := generateVersionCertificate(w.Ecdsa.Address, w.Ecdsa.PublicKey, version, w.Ecdsa.Sign)
	if err != nil {
		return err
	}
	return m.upsertAndGossipVersionCertificateEntries([]*vet.VersionCertificateEntry{
		newVersionCertificate.Entry(),
	})
}

func getTimestamp() uint {
	// Unix() returns a int64, but we need a uint for the golang rlp encoding implmentation. Warning: This timestamp value will be truncated in 2106.
	return uint(time.Now().Unix())
}

// RetrieveEnodeCertificateMsgMap gets the most recent enode certificate messages.
// May be nil if no message was generated as a result of the core not being
// started, or if a proxy has not received a message from its proxied validator
func (m *AnnounceManager) RetrieveEnodeCertificateMsgMap() map[enode.ID]*istanbul.EnodeCertMsg {
	m.enodeCertificateMsgMapMu.Lock()
	defer m.enodeCertificateMsgMapMu.Unlock()
	return m.enodeCertificateMsgMap
}

// getEnodeCertNodesAndDestAddresses will retrieve all the external facing external nodes for this validator
// (one for each of it's proxies, or itself for standalone validators) for the purposes of generating enode certificates
// for those enodes.  It will also return the destination validators for each enode certificate.  If the destAddress is a
// `nil` value, then that means that the associated enode certificate should be sent to all of the connected validators.
func (m *AnnounceManager) getEnodeCertNodesAndDestAddresses() ([]*enode.Node, map[enode.ID][]common.Address, error) {
	var externalEnodes []*enode.Node
	var valDestinations map[enode.ID][]common.Address
	if m.config.IsProxiedValidator {
		var proxies []*proxy.Proxy
		var err error

		proxies, valDestinations, err = m.proxyContext.GetProxiedValidatorEngine().GetProxiesAndValAssignments()
		if err != nil {
			return nil, nil, err
		}

		externalEnodes = make([]*enode.Node, len(proxies))
		for i, proxy := range proxies {
			externalEnodes[i] = proxy.ExternalNode()
		}
	} else {
		externalEnodes = make([]*enode.Node, 1)
		externalEnodes[0] = m.addrProvider.SelfNode()
		valDestinations = make(map[enode.ID][]common.Address)
		valDestinations[externalEnodes[0].ID()] = nil
	}

	return externalEnodes, valDestinations, nil
}

// generateEnodeCertificateMsgs generates a map of enode certificate messages.
// One certificate message is generated for each external enode this node possesses generated for
// each external enode this node possesses. A unproxied validator will have one enode, while a
// proxied validator may have one for each proxy.. Each enode is a key in the returned map, and the
// value is the certificate message.
func (m *AnnounceManager) generateEnodeCertificateMsgs(version uint) (map[enode.ID]*istanbul.EnodeCertMsg, error) {
	logger := m.logger.New("func", "generateEnodeCertificateMsgs")

	enodeCertificateMsgs := make(map[enode.ID]*istanbul.EnodeCertMsg)
	externalEnodes, valDestinations, err := m.getEnodeCertNodesAndDestAddresses()
	if err != nil {
		return nil, err
	}

	for _, externalNode := range externalEnodes {
		enodeCertificate := &istanbul.EnodeCertificate{
			EnodeURL: externalNode.URLv4(),
			Version:  version,
		}
		enodeCertificateBytes, err := rlp.EncodeToBytes(enodeCertificate)
		if err != nil {
			return nil, err
		}
		msg := &istanbul.Message{
			Code:    istanbul.EnodeCertificateMsg,
			Address: m.wallets().Ecdsa.Address,
			Msg:     enodeCertificateBytes,
		}
		// Sign the message
		if err := msg.Sign(m.wallets().Ecdsa.Sign); err != nil {
			return nil, err
		}

		enodeCertificateMsgs[externalNode.ID()] = &istanbul.EnodeCertMsg{Msg: msg, DestAddresses: valDestinations[externalNode.ID()]}
	}

	logger.Trace("Generated Istanbul Enode Certificate messages", "enodeCertificateMsgs", enodeCertificateMsgs)
	return enodeCertificateMsgs, nil
}

// handleEnodeCertificateMsg handles an enode certificate message for proxied and standalone validators.
func (m *AnnounceManager) handleEnodeCertificateMsg(_ consensus.Peer, payload []byte) error {
	logger := m.logger.New("func", "handleEnodeCertificateMsg")

	var msg istanbul.Message
	// Decode payload into msg
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Enode Certificate message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger = logger.New("msg address", msg.Address)

	var enodeCertificate istanbul.EnodeCertificate
	if err := rlp.DecodeBytes(msg.Msg, &enodeCertificate); err != nil {
		logger.Warn("Error in decoding received Istanbul Enode Certificate message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}
	logger.Trace("Received Istanbul Enode Certificate message", "enodeCertificate", enodeCertificate)

	parsedNode, err := enode.ParseV4(enodeCertificate.EnodeURL)
	if err != nil {
		logger.Warn("Malformed v4 node in received Istanbul Enode Certificate message", "enodeCertificate", enodeCertificate, "err", err)
		return err
	}

	// Ensure this node is a validator in the validator conn set
	shouldSave, err := m.checker.IsElectedOrNearValidator()
	if err != nil {
		logger.Error("Error checking if should save received validator enode url", "err", err)
		return err
	}
	if !shouldSave {
		logger.Debug("This node should not save validator enode urls, ignoring enodeCertificate")
		return nil
	}

	validatorConnSet, err := m.network.RetrieveValidatorConnSet()
	if err != nil {
		logger.Debug("Error in retrieving registered/elected valset", "err", err)
		return err
	}

	if !validatorConnSet[msg.Address] {
		logger.Debug("Received Istanbul Enode Certificate message originating from a node not in the validator conn set")
		return errUnauthorizedAnnounceMessage
	}

	if err := m.state.valEnodeTable.UpsertVersionAndEnode([]*istanbul.AddressEntry{{Address: msg.Address, Node: parsedNode, Version: enodeCertificate.Version}}); err != nil {
		logger.Warn("Error in upserting a val enode table entry", "error", err)
		return err
	}

	// Send a valEnodesShare message to the proxy when it's the primary
	if m.config.IsProxiedValidator && m.checker.IsValidating() {
		m.proxyContext.GetProxiedValidatorEngine().SendValEnodesShareMsgToAllProxies()
	}

	return nil
}

func (m *AnnounceManager) SetEnodeCertificateMsgMap(enodeCertMsgMap map[enode.ID]*istanbul.EnodeCertMsg) error {
	logger := m.logger.New("func", "SetEnodeCertificateMsgMap")
	var enodeCertVersion *uint

	// Verify that all of the certificates have the same version
	for _, enodeCertMsg := range enodeCertMsgMap {
		var enodeCert istanbul.EnodeCertificate
		if err := rlp.DecodeBytes(enodeCertMsg.Msg.Msg, &enodeCert); err != nil {
			return err
		}

		if enodeCertVersion == nil {
			enodeCertVersion = &enodeCert.Version
		} else {
			if enodeCert.Version != *enodeCertVersion {
				logger.Error("enode certificate messages within enode certificate msgs array don't all have the same version")
				return errInvalidEnodeCertMsgMapInconsistentVersion
			}
		}
	}

	m.enodeCertificateMsgMapMu.Lock()
	defer m.enodeCertificateMsgMapMu.Unlock()

	// Already have a more recent enodeCertificate
	if *enodeCertVersion < m.enodeCertificateMsgVersion {
		logger.Error("Ignoring enode certificate msgs since it's an older version", "enodeCertVersion", *enodeCertVersion, "sb.enodeCertificateMsgVersion", m.enodeCertificateMsgVersion)
		return istanbul.ErrInvalidEnodeCertMsgMapOldVersion
	} else if *enodeCertVersion == m.enodeCertificateMsgVersion {
		// This function may be called with the same enode certificate.
		// Proxied validators will periodically send the same enode certificate to it's proxies,
		// to ensure that the proxies to eventually get their enode certificates.
		logger.Trace("Attempting to set an enode certificate with the same version as the previous set enode certificate's")
	} else {
		logger.Debug("Setting enode certificate", "version", *enodeCertVersion)
		m.enodeCertificateMsgMap = enodeCertMsgMap
		m.enodeCertificateMsgVersion = *enodeCertVersion
	}

	return nil
}

func (m *AnnounceManager) StartAnnouncing(onStart func() error) error {
	m.announceMu.Lock()
	defer m.announceMu.Unlock()
	if m.announceRunning {
		return istanbul.ErrStartedAnnounce
	}

	go m.announceThread()

	m.announceThreadQuit = make(chan struct{})
	m.announceRunning = true

	if err := onStart(); err != nil {
		m.StopAnnouncing(func() error { return nil })
		return err
	}

	return nil
}

func (m *AnnounceManager) StopAnnouncing(onStop func() error) error {
	m.announceMu.Lock()
	defer m.announceMu.Unlock()

	if !m.announceRunning {
		return istanbul.ErrStoppedAnnounce
	}

	close(m.announceThreadQuit)
	m.announceThreadWg.Wait()

	m.announceRunning = false

	return onStop()
}

func (m *AnnounceManager) GetVersionCertificateTableInfo() (map[string]*vet.VersionCertificateEntryInfo, error) {
	return m.state.versionCertificateTable.Info()
}
