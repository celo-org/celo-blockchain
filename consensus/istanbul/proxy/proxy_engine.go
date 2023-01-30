// Copyright 2017 The celo Authors
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

package proxy

import (
	"sync"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

// BackendForProxyEngine provides the Istanbul backend application specific functions for Istanbul proxy engine
type BackendForProxyEngine interface {
	// IsProxy returns true if this node is a proxy
	IsProxy() bool

	// SelfNode returns the owner's node (if this is a proxy, it will return the external node)
	SelfNode() *enode.Node

	// Multicast sends a message to it's connected nodes filtered on the 'addresses' parameter (where each address
	// is associated with those node's signing key)
	// If sendToSelf is set to true, then the function will send an event to self via a message event
	Multicast(addresses []common.Address, msg *istanbul.Message, ethMsgCode uint64, sendToSelf bool) error

	// Unicast will asynchronously send a celo message to peer
	Unicast(peer consensus.Peer, payload []byte, ethMsgCode uint64)

	// GetValEnodeTableEntries retrieves the entries in the valEnodeTable filtered on the "validators" parameter.
	// If the parameter is nil, then no filter will be applied.
	GetValEnodeTableEntries(validators []common.Address) (map[common.Address]*istanbul.AddressEntry, error)

	// RewriteValEnodeTableEntries will rewrite the val enode table with "entries".
	RewriteValEnodeTableEntries(entries map[common.Address]*istanbul.AddressEntry) error

	// SetEnodeCertificateMsgs will set this node's enodeCertificate to be used for connection handshakes
	SetEnodeCertificateMsgMap(enodeCertificateMsgMap map[enode.ID]*istanbul.EnodeCertMsg) error

	// RetrieveEnodeCertificateMsgMap will retrieve this node's handshake enodeCertificate
	RetrieveEnodeCertificateMsgMap() map[enode.ID]*istanbul.EnodeCertMsg

	// VerifyPendingBlockValidatorSignature is a message validation function to verify that a message's sender is within the validator set
	// of the current pending block and that the message's address field matches the message's signature's signer
	VerifyPendingBlockValidatorSignature(data []byte, sig []byte) (common.Address, error)

	// VerifyValidatorConnectionSetSignature is a message validation function to verify that a message's sender is within the
	// validator connection set and that the message's address field matches the message's signature's signer
	VerifyValidatorConnectionSetSignature(data []byte, sig []byte) (common.Address, error)

	// GetProxy returns the proxy engine created for this Backend.  Note: This should be only used for the unit tests.
	GetProxyEngine() ProxyEngine
}

type proxyEngine struct {
	config  *istanbul.Config
	logger  log.Logger
	backend BackendForProxyEngine

	// Proxied Validators peers and IDs
	proxiedValidators   map[consensus.Peer]bool
	proxiedValidatorIDs map[enode.ID]bool
	proxiedValidatorsMu sync.RWMutex
}

// NewProxyEngine creates a new proxy engine.
func NewProxyEngine(backend BackendForProxyEngine, config *istanbul.Config) (ProxyEngine, error) {
	if !backend.IsProxy() {
		return nil, ErrNodeNotProxy
	}

	p := &proxyEngine{
		config:              config,
		logger:              log.New(),
		backend:             backend,
		proxiedValidators:   make(map[consensus.Peer]bool),
		proxiedValidatorIDs: make(map[enode.ID]bool),
	}

	return p, nil
}

func (p *proxyEngine) HandleMsg(peer consensus.Peer, msgCode uint64, payload []byte) (bool, error) {
	if msgCode == istanbul.ValEnodesShareMsg {
		return p.handleValEnodesShareMsg(peer, payload)
	} else if msgCode == istanbul.FwdMsg {
		return p.handleForwardMsg(peer, payload)
	} else if msgCode == istanbul.ConsensusMsg {
		return p.handleConsensusMsg(peer, payload)
	} else if msgCode == istanbul.EnodeCertificateMsg {
		// See if the message is coming from the proxied validator
		p.proxiedValidatorsMu.RLock()
		msgFromProxiedVal := p.proxiedValidatorIDs[peer.Node().ID()]
		p.proxiedValidatorsMu.RUnlock()

		if msgFromProxiedVal {
			return p.handleEnodeCertificateMsgFromProxiedValidator(peer, payload)
		} else {
			return p.handleEnodeCertificateMsgFromRemoteVal(peer, payload)
		}
	}

	return false, nil
}

// Callback once validator dials us and is properly registered.
func (p *proxyEngine) RegisterProxiedValidatorPeer(proxiedValidatorPeer consensus.Peer) {
	p.proxiedValidatorsMu.Lock()
	defer p.proxiedValidatorsMu.Unlock()

	logger := p.logger.New("func", "RegisterProxiedValidatorPeer")
	logger.Info("Validator connected to proxy", "ID", proxiedValidatorPeer.Node().ID(), "enode", proxiedValidatorPeer.Node())

	p.proxiedValidators[proxiedValidatorPeer] = true
	p.proxiedValidatorIDs[proxiedValidatorPeer.Node().ID()] = true

}

func (p *proxyEngine) UnregisterProxiedValidatorPeer(proxiedValidatorPeer consensus.Peer) {
	p.proxiedValidatorsMu.Lock()
	defer p.proxiedValidatorsMu.Unlock()

	logger := p.logger.New("func", "UnregisterProxiedValidatorPeer")
	logger.Info("Removing validator", "ID", proxiedValidatorPeer.Node().ID(), "enode", proxiedValidatorPeer.Node())

	delete(p.proxiedValidators, proxiedValidatorPeer)
	delete(p.proxiedValidatorIDs, proxiedValidatorPeer.Node().ID())

}

func (p *proxyEngine) GetProxiedValidatorsInfo() ([]*ProxiedValidatorInfo, error) {
	p.proxiedValidatorsMu.RLock()
	defer p.proxiedValidatorsMu.RUnlock()

	proxiedValidatorsInfo := []*ProxiedValidatorInfo{}
	for proxiedValidatorPeer := range p.proxiedValidators {
		pubKey := proxiedValidatorPeer.Node().Pubkey()
		addr := crypto.PubkeyToAddress(*pubKey)
		proxiedValidatorInfo := &ProxiedValidatorInfo{
			Address:  addr,
			IsPeered: true,
			Node:     proxiedValidatorPeer.Node()}
		proxiedValidatorsInfo = append(proxiedValidatorsInfo, proxiedValidatorInfo)
	}
	return proxiedValidatorsInfo, nil
}

// SendMsgToProxiedValidators will send a `celo` message to the proxied validators.
func (p *proxyEngine) SendMsgToProxiedValidators(msgCode uint64, msg *istanbul.Message) error {
	logger := p.logger.New("func", "SendMsgToProxiedValidators")
	p.proxiedValidatorsMu.RLock()
	defer p.proxiedValidatorsMu.RUnlock()
	if len(p.proxiedValidators) == 0 {
		logger.Warn("Proxy has no connected proxied validator.  Not sending message.")
		return nil
	}
	payload, err := msg.Payload()
	if err != nil {
		logger.Error("Error getting payload of message", "err", err)
		return err
	}
	for proxiedValidator := range p.proxiedValidators {
		p.backend.Unicast(proxiedValidator, payload, msgCode)
	}
	return nil
}
