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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type proxyEngine struct {
	config  *istanbul.Config
	address common.Address
	logger  log.Logger
	backend istanbul.Backend

	// Proxied validator's proxy
	proxyNode *proxy

	// Proxy's validator
	// Right now, we assume that there is at most one proxied peer for a proxy
	// Proxy's validator
	proxiedValidator consensus.Peer

	valEnodesShareWg        *sync.WaitGroup
	valEnodesShareQuit      chan struct{}
	sendValEnodesShareMsgCh chan struct{}
}

// New creates a new proxy engine.  This is used by both
// proxies and proxied validators
func New(backend istanbul.Backend, config *istanbul.Config) ProxyEngine {
	p := &proxyEngine{
		config:    config,
		address:   backend.Address(),
		logger:    log.New(),
		backend:   backend,
		proxyNode: nil,

		valEnodesShareWg:        new(sync.WaitGroup),
		valEnodesShareQuit:      make(chan struct{}),
		sendValEnodesShareMsgCh: make(chan struct{}),
	}
	return p
}

func (p *proxyEngine) Start() error {
	if p.backend.IsProxiedValidator() {
		if p.config.ProxyInternalFacingNode != nil && p.config.ProxyExternalFacingNode != nil {
			if err := p.AddProxy(p.config.ProxyInternalFacingNode, p.config.ProxyExternalFacingNode); err != nil {
				p.logger.Error("Issue in adding proxy", "err", err)
				return err
			}
		}

		go p.sendValEnodesShareMsgEventLoop()
	}

	return nil
}

func (p *proxyEngine) Stop() error {
	if p.backend.IsProxiedValidator() {
		p.valEnodesShareQuit <- struct{}{}
		p.valEnodesShareWg.Wait()

		if p.proxyNode != nil {
			p.RemoveProxy(p.proxyNode.node)
		}
	}

	return nil
}

func (p *proxyEngine) GetProxyExternalNode() *enode.Node {
	return p.config.ProxyExternalFacingNode
}

func (p *proxyEngine) AddProxy(node, externalNode *enode.Node) error {
	if p.proxyNode != nil {
		return errProxyAlreadySet
	}
	p.proxyNode = &proxy{node: node, externalNode: externalNode}
	p.backend.UpdateAnnounceVersion()
	p.backend.AddPeer(node, p2p.ProxyPurpose)
	return nil
}

func (p *proxyEngine) RemoveProxy(node *enode.Node) {
	if p.proxyNode != nil && p.proxyNode.node.ID() == node.ID() {
		p.backend.RemovePeer(node, p2p.ProxyPurpose)
		p.proxyNode = nil
	}
}

func (p *proxyEngine) HandleMsg(peer consensus.Peer, msgCode uint64, payload []byte) (bool, error) {
	if msgCode == istanbul.ValEnodesShareMsg {
		// For now, handle this in a goroutine
		go p.handleValEnodesShareMsg(peer, payload)
		return true, nil
	} else if msgCode == istanbul.FwdMsg {
		return p.handleForwardMsg(peer, payload)
	} else if msgCode == istanbul.ConsensusMsg {
		return p.handleConsensusMsg(peer, payload)
	} else if msgCode == istanbul.EnodeCertificateMsg {
		go p.handleEnodeCertificateMsg(peer, payload)
		return true, nil
	}

	return false, nil
}

func (p *proxyEngine) RegisterProxiedValidator(proxiedValidatorPeer consensus.Peer) {
	// TODO: Does this need a lock?
	p.proxiedValidator = proxiedValidatorPeer
}

func (p *proxyEngine) RegisterProxy(proxyPeer consensus.Peer) error {
	logger := p.logger.New("func", "RegisterProxy")
	if p.proxyNode != nil && proxyPeer.Node().ID() == p.proxyNode.node.ID() {
		p.proxyNode.peer = proxyPeer
		// Share this node's enodeCertificate for the proxy to use for handshakes
		enodeCertificateMsg, err := p.backend.RetrieveEnodeCertificateMsg()
		if err != nil {
			logger.Warn("Error getting enode certificate message", "err", err)
		} else if enodeCertificateMsg != nil {
			payload, err := enodeCertificateMsg.Payload()
			if err != nil {
				logger.Error("Error getting payload of enode certificate message", "err", err)
				return err
			}
			if err := proxyPeer.Send(istanbul.ProxyEnodeCertificateShareMsg, payload); err != nil {
				logger.Error("Error in sending ProxyEnodeCertificateShare message to proxy", "err", err)
				return err
			}
		}

		// Share the whole val enode table
		p.SendValEnodesShareMsg()
	} else {
		logger.Error("Unauthorized connected peer to the proxied validator", "peerID", proxyPeer.Node().ID())
		return errUnauthorizedProxiedValidator
	}

	return nil
}

func (p *proxyEngine) UnregisterProxiedValidator(proxiedValidatorPeer consensus.Peer) {
	if p.proxiedValidator != nil && proxiedValidatorPeer.Node().ID() == p.proxiedValidator.Node().ID() {
		p.proxiedValidator = nil
	}
}

func (p *proxyEngine) UnregisterProxy(proxyPeer consensus.Peer) {
	if p.proxyNode != nil && proxyPeer.Node().ID() == p.proxyNode.node.ID() {
		p.proxyNode.peer = nil
	}
}
