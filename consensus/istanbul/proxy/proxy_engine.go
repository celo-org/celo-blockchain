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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type proxyEngine struct {
	config  *istanbul.Config
	logger  log.Logger
	backend BackendForProxyEngine

	// Proxy's validator
	// Right now, we assume that there is at most one proxied peer for a proxy
	// Proxy's validator
	proxiedValidator consensus.Peer

	// Proxied validator's proxy handler
	ph *proxyHandler
}

// New creates a new proxy engine.  This is used by both
// proxies and proxied validators
func NewEngine(backend BackendForProxyEngine, config *istanbul.Config) ProxyEngine {
	p := &proxyEngine{
		config:  config,
		logger:  log.New(),
		backend: backend,
	}

	if backend.IsProxiedValidator() {
		p.ph = newProxyHandler(backend, p)
	}

	return p
}

func (p *proxyEngine) Start() error {
	if p.backend.IsProxiedValidator() {
		if err := p.ph.Start(); err != nil {
			return err
		}

		if len(p.config.ProxyConfigs) > 0 {
			p.ph.addProxies <- p.config.ProxyConfigs
		}

	}

	p.logger.Info("Proxy engine started")
	return nil
}

func (p *proxyEngine) Stop() error {
	if p.backend.IsProxiedValidator() {
		if err := p.ph.Stop(); err != nil {
			return err
		}
	}

	p.logger.Info("Proxy engine stopped")
	return nil
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

func (p *proxyEngine) AddProxy(node, externalNode *enode.Node) error {
	if p.backend.IsProxiedValidator() {
		p.ph.addProxies <- []*istanbul.ProxyConfig{&istanbul.ProxyConfig{InternalNode: node, ExternalNode: externalNode}}
	} else {
		return ErrNodeNotProxiedValidator
	}

	return nil
}

func (p *proxyEngine) RemoveProxy(node *enode.Node) error {
	if p.backend.IsProxiedValidator() {
		p.ph.removeProxies <- []*enode.Node{node}
	} else {
		return ErrNodeNotProxiedValidator
	}

	return nil
}

func (p *proxyEngine) RegisterProxyPeer(proxyPeer consensus.Peer) error {
	logger := p.logger.New("func", "RegisterProxyPeer")
	if p.backend.IsProxiedValidator() {
		if proxyPeer.PurposeIsSet(p2p.ProxyPurpose) {
			logger.Info("Got new proxy peer", "proxyPeer", proxyPeer)
			p.ph.addProxyPeer <- proxyPeer
		} else {
			logger.Error("Unauthorized connected peer to the proxied validator", "peerID", proxyPeer.Node().ID())
			return errUnauthorizedProxiedValidator
		}
	}

	return nil
}

func (p *proxyEngine) UnregisterProxyPeer(proxyPeer consensus.Peer) {
	if p.backend.IsProxiedValidator() && proxyPeer.PurposeIsSet(p2p.ProxyPurpose) {
		p.ph.removeProxyPeer <- proxyPeer
	}
}

func (p *proxyEngine) RegisterProxiedValidatorPeer(proxiedValidatorPeer consensus.Peer) {
	// TODO: Does this need a lock?
	if p.backend.IsProxy() {
		p.proxiedValidator = proxiedValidatorPeer
	}
}

func (p *proxyEngine) UnregisterProxiedValidatorPeer(proxiedValidatorPeer consensus.Peer) {
	if p.backend.IsProxy() && p.proxiedValidator != nil && proxiedValidatorPeer.Node().ID() == p.proxiedValidator.Node().ID() {
		p.proxiedValidator = nil
	}
}

func (p *proxyEngine) GetValidatorProxyAssignments() (map[common.Address]*enode.Node, error) {
	logger := p.logger.New("func", "GetValidatorProxyAssignments")
	if p.backend.IsProxiedValidator() {
		if !p.ph.Running() {
			return nil, ErrStoppedProxyHandler
		}

		valAssignments, err := p.ph.GetValidatorAssignments(nil)
		if err != nil {
			logger.Warn("Error in retrieving all of the validator assignments", "err", err)
			return nil, err
		}

		valProxyAssignments := make(map[common.Address]*enode.Node)

		for address, proxy := range valAssignments {
			if proxy != nil {
				valProxyAssignments[address] = proxy.externalNode
			} else {
				valProxyAssignments[address] = nil
			}
		}

		return valProxyAssignments, nil
	} else {
		return nil, ErrNodeNotProxiedValidator
	}
}

func (p *proxyEngine) GetProxiesAndValAssignments() ([]*proxy, map[enode.ID][]common.Address, error) {
	return p.ph.GetProxiesAndValAssignments()
}

func (p *proxyEngine) GetProxiedValidatorsInfo() ([]ProxiedValidatorInfo, error) {
	if p.proxiedValidator != nil {
		proxiedValidatorInfo := ProxiedValidatorInfo{Address: p.config.ProxiedValidatorAddress,
			IsPeered: true,
			Node:     p.proxiedValidator.Node()}
		return []ProxiedValidatorInfo{proxiedValidatorInfo}, nil
	} else {
		return []ProxiedValidatorInfo{}, nil
	}
}
