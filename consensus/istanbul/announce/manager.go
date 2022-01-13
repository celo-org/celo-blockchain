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

package announce

import (
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/proxy"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

var (
	errNodeMissingEnodeCertificate = errors.New("Node is missing enode certificate")

	// errUnauthorizedAnnounceMessage is returned when the received announce message is from
	// an unregistered validator
	errUnauthorizedAnnounceMessage = errors.New("unauthorized announce message")
)

type ProxyContext interface {
	GetProxiedValidatorEngine() proxy.ProxiedValidatorEngine
}

// Manager is the facade and entry point for the implementation of the announce protocol. It exposes methods to
// start and stop the announce Worker, and to handle announce messages.
type Manager struct {
	logger log.Logger

	config *istanbul.Config

	aWallets *atomic.Value

	addrProvider AddressProvider
	proxyContext ProxyContext
	network      Network

	vcGossiper VersionCertificateGossiper

	gossipCache istanbul.GossipCache

	state *AnnounceState

	checker ValidatorChecker

	ovcp OutboundVersionCertificateProcessor

	worker Worker

	vpap ValProxyAssigmnentProvider

	announceRunning  bool
	announceMu       sync.RWMutex
	announceThreadWg *sync.WaitGroup

	ecertHolder EnodeCertificateMsgHolder
}

// NewManager creates a new Manager using the valEnodeTable given. It is
// the responsibility of the caller to close the valEnodeTable, the Manager will
// not do it.
func NewManager(
	config *istanbul.Config,
	aWallets *atomic.Value,
	network Network, proxyContext ProxyContext,
	addrProvider AddressProvider, state *AnnounceState,
	gossipCache istanbul.GossipCache,
	checker ValidatorChecker,
	ovcp OutboundVersionCertificateProcessor,
	ecertHolder EnodeCertificateMsgHolder,
	vcGossiper VersionCertificateGossiper,
	vpap ValProxyAssigmnentProvider,
	worker Worker) *Manager {

	am := &Manager{
		logger:           log.New("module", "announceManager"),
		aWallets:         aWallets,
		config:           config,
		network:          network,
		proxyContext:     proxyContext,
		addrProvider:     addrProvider,
		gossipCache:      gossipCache,
		vcGossiper:       vcGossiper,
		state:            state,
		checker:          checker,
		ovcp:             ovcp,
		ecertHolder:      ecertHolder,
		vpap:             vpap,
		worker:           worker,
		announceThreadWg: new(sync.WaitGroup),
		announceRunning:  false,
	}

	return am
}

func (m *Manager) isProxiedValidator() bool {
	return m.config.Proxied && m.config.Validator
}

func (m *Manager) Close() error {
	// No need to close valEnodeTable since it's a reference,
	// the creator of this announce manager is the responsible for
	// closing it.
	return m.state.VersionCertificateTable.Close()
}

func (m *Manager) wallets() *istanbul.Wallets {
	return m.aWallets.Load().(*istanbul.Wallets)
}

// The announceThread will:
// 1) Periodically poll to see if this node should be announcing
// 2) Periodically share the entire version certificate table with all peers
// 3) Periodically prune announce-related data structures
// 4) Gossip announce messages periodically when requested
// 5) Update announce version when requested
func (m *Manager) announceThread() {
	defer m.announceThreadWg.Done()
	m.worker.Run()
}

// HandleQueryEnodeMsg handles a queryEnodeMsg received by the p2p network, according to the announce protocol spec.
func (m *Manager) HandleQueryEnodeMsg(addr common.Address, peer consensus.Peer, payload []byte) error {
	logger := m.logger.New("func", "HandleQueryEnodeMsg")

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

	qeData := msg.QueryEnodeMsg()
	logger = logger.New("msgAddress", msg.Address, "msgVersion", qeData.Version)

	// Do some validation checks on the istanbul.QueryEnodeData
	if isValid, err := m.validateQueryEnode(msg.Address, msg.QueryEnodeMsg()); !isValid || err != nil {
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
			node, err := DecryptEnodeURL(&w.Ecdsa, encEnodeURL.EncryptedEnodeURL)
			if err != nil {
				logger.Error("Can't process encEnodeURL. err", err, "encEnodeURL.EncryptedEnodeURL", encEnodeURL.EncryptedEnodeURL)
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
func (m *Manager) answerQueryEnodeMsg(address common.Address, node *enode.Node, version uint) error {
	logger := m.logger.New("func", "answerQueryEnodeMsg", "address", address)

	// Get the external enode that this validator is assigned to
	externalEnodeMap, err := m.vpap.GetValProxyAssignments([]common.Address{address})
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
	if err := m.state.ValEnodeTable.UpsertVersionAndEnode([]*istanbul.AddressEntry{{Address: address, Node: node, Version: version}}); err != nil {
		return err
	}
	return nil
}

// validateQueryEnode will do some validation to check the contents of the queryEnode
// message. This is to force all validators that send a queryEnode message to
// create as succint message as possible, and prevent any possible network DOS attacks
// via extremely large queryEnode message.
func (m *Manager) validateQueryEnode(msgAddress common.Address, qeData *istanbul.QueryEnodeData) (bool, error) {
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
func (m *Manager) regossipQueryEnode(msg *istanbul.Message, msgTimestamp uint, payload []byte) error {
	logger := m.logger.New("func", "regossipQueryEnode", "queryEnodeSourceAddress", msg.Address, "msgTimestamp", msgTimestamp)

	// Don't throttle messages from our own address so that proxies always regossip
	// query enode messages sent from the proxied validator
	if msg.Address != m.addrProvider.ValidatorAddress() {
		if lastGossiped, ok := m.state.LastQueryEnodeGossiped.Get(msg.Address); ok {
			if time.Since(lastGossiped) < QueryEnodeGossipCooldownDuration {
				logger.Trace("Already regossiped msg from this source address within the cooldown period, not regossiping.")
				return nil
			}
		}
	}

	logger.Trace("Regossiping the istanbul queryEnode message", "IstanbulMsg", msg.String())
	if err := m.network.Gossip(payload, istanbul.QueryEnodeMsg); err != nil {
		return err
	}

	m.state.LastQueryEnodeGossiped.Set(msg.Address, time.Now())

	return nil
}

// SendVersionCertificateTable sends all VersionCertificates this node
// has to a peer
func (m *Manager) SendVersionCertificateTable(peer consensus.Peer) error {
	return m.vcGossiper.SendAllFrom(m.state.VersionCertificateTable, peer)
}

// HandleVersionCertificatesMsg handles a versionCertificates received by the p2p network, according to the announce protocol spec.
func (m *Manager) HandleVersionCertificatesMsg(addr common.Address, peer consensus.Peer, payload []byte) error {
	logger := m.logger.New("func", "HandleVersionCertificatesMsg")
	logger.Trace("Handling version certificates msg")

	// Since this is a gossiped messaged, mark that the peer gossiped it (and presumably processed it) and check to see if this node already processed it
	m.gossipCache.MarkMessageProcessedByPeer(addr, payload)
	if m.gossipCache.CheckIfMessageProcessedBySelf(payload) {
		return nil
	}
	defer m.gossipCache.MarkMessageProcessedBySelf(payload)

	msg := &istanbul.Message{}
	if err := msg.FromPayload(payload, nil); err != nil {
		logger.Error("Error in decoding version certificates message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger = logger.New("msg address", msg.Address)

	versionCertificates := msg.VersionCertificates()

	// If the announce's valAddress is not within the validator connection set, then ignore it
	validatorConnSet, err := m.network.RetrieveValidatorConnSet()
	if err != nil {
		logger.Trace("Error in retrieving validator conn set", "err", err)
		return err
	}

	var validEntries []*istanbul.VersionCertificate
	validAddresses := make(map[common.Address]bool)
	// Verify all entries are valid and remove duplicates
	for _, versionCertificate := range versionCertificates {
		address := versionCertificate.Address()
		if !validatorConnSet[address] {
			logger.Debug("Found version certificate from an address not in the validator conn set", "address", address)
			continue
		}
		if _, ok := validAddresses[address]; ok {
			logger.Debug("Found duplicate version certificate in message", "address", address)
			continue
		}
		validAddresses[address] = true
		validEntries = append(validEntries, versionCertificate)
	}
	if err := m.ovcp.Process(m.state, validEntries, m.wallets().Ecdsa.Address); err != nil {
		logger.Warn("Error upserting and gossiping entries", "err", err)
		return err
	}
	return nil
}

// UpdateAnnounceVersion will asynchronously update the announce version.
func (m *Manager) UpdateAnnounceVersion() {
	m.worker.UpdateVersion()
}

// GetAnnounceVersion will retrieve the current announce version.
func (m *Manager) GetAnnounceVersion() uint {
	return m.worker.GetVersion()
}

// RetrieveEnodeCertificateMsgMap gets the most recent enode certificate messages.
// May be nil if no message was generated as a result of the core not being
// started, or if a proxy has not received a message from its proxied validator
func (m *Manager) RetrieveEnodeCertificateMsgMap() map[enode.ID]*istanbul.EnodeCertMsg {
	return m.ecertHolder.Get()
}

// HandleEnodeCertificateMsg handles an enode certificate message for proxied and standalone validators.
func (m *Manager) HandleEnodeCertificateMsg(_ consensus.Peer, payload []byte) error {
	logger := m.logger.New("func", "HandleEnodeCertificateMsg")

	var msg istanbul.Message
	// Decode payload into msg
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Enode Certificate message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger = logger.New("msg address", msg.Address)

	enodeCertificate := msg.EnodeCertificate()
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

	if err := m.state.ValEnodeTable.UpsertVersionAndEnode([]*istanbul.AddressEntry{{Address: msg.Address, Node: parsedNode, Version: enodeCertificate.Version}}); err != nil {
		logger.Warn("Error in upserting a val enode table entry", "error", err)
		return err
	}

	// Send a valEnodesShare message to the proxy when it's the primary
	if m.isProxiedValidator() && m.checker.IsValidating() {
		m.proxyContext.GetProxiedValidatorEngine().SendValEnodesShareMsgToAllProxies()
	}

	return nil
}

func (m *Manager) SetEnodeCertificateMsgMap(enodeCertMsgMap map[enode.ID]*istanbul.EnodeCertMsg) error {
	return m.ecertHolder.Set(enodeCertMsgMap)
}

func (m *Manager) StartAnnouncing(onStart func() error) error {
	m.announceMu.Lock()
	defer m.announceMu.Unlock()
	if m.announceRunning {
		return istanbul.ErrStartedAnnounce
	}

	m.announceRunning = true
	m.announceThreadWg.Add(1)
	go m.announceThread()

	if err := onStart(); err != nil {
		m.unlockedStopAnnouncing(func() error { return nil })
		return err
	}

	return nil
}

func (m *Manager) StopAnnouncing(onStop func() error) error {
	m.announceMu.Lock()
	defer m.announceMu.Unlock()

	if !m.announceRunning {
		return istanbul.ErrStoppedAnnounce
	}

	return m.unlockedStopAnnouncing(onStop)
}

func (m *Manager) unlockedStopAnnouncing(onStop func() error) error {
	m.worker.Stop()
	m.announceThreadWg.Wait()

	m.announceRunning = false

	return onStop()
}

func (m *Manager) GetVersionCertificateTableInfo() (map[string]*VersionCertificateEntryInfo, error) {
	return m.state.VersionCertificateTable.Info()
}

// IsAnnounceRunning returns true iff the anounce Worker thread is running, without locking
// for access.
func (m *Manager) IsAnnounceRunning() bool {
	m.announceMu.RLock()
	defer m.announceMu.RUnlock()
	return m.announceRunning
}

// Worker returns the worker used by this manager.
func (m *Manager) Worker() Worker {
	return m.worker
}
