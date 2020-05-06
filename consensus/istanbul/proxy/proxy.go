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

type proxy struct {
	config         *istanbul.Config
	address        common.Address
	logger         log.Logger
	backend        istanbul.Backend

	// Proxied validator's proxy
	proxyNode *proxyInfo
	proxyPeer consensus.Peer

	// Proxy's validator
	// Right now, we assume that there is at most one proxied peer for a proxy
	// Proxy's validator
	proxiedPeer consensus.Peer

	valEnodesShareWg   *sync.WaitGroup
	valEnodesShareQuit chan struct{}
}

// New creates an Istanbul consensus core
func New(backend istanbul.Backend, config *istanbul.Config) Proxy {
	p := &proxy{
		config:             config,
		address:            backend.Address(),
		logger:             log.New(),
		backend:            backend,
		proxyNode:          nil,
		proxyPeer:          nil,
		
		valEnodesShareWg:                   new(sync.WaitGroup),
		valEnodesShareQuit:                 make(chan struct{}),
	}
	return p
}

func (p *proxy) Start() error {
	if p.config.ProxyInternalFacingNode != nil && p.config.ProxyExternalFacingNode != nil {
		if err := p.AddProxy(p.config.ProxyInternalFacingNode, p.config.ProxyExternalFacingNode); err != nil {
			p.logger.Error("Issue in adding proxy", "err", err)
			return err
		}
	}
	
	go p.sendValEnodesShareMsgs()

	return nil
}

func (p *proxy) Stop() error {
	p.valEnodesShareQuit <- struct{}{}
	p.valEnodesShareWg.Wait()

	if p.proxyNode != nil {
		p.RemoveProxy(p.proxyNode.node)
	}

	return nil
}

func (p* proxy) GetProxyExternalNode() *enode.Node {
     return p.config.ProxyExternalFacingNode
}

func (p *proxy) AddProxy(node, externalNode *enode.Node) error {
	if p.proxyNode != nil {
		return errProxyAlreadySet
	}
	p.proxyNode = &proxyInfo{node: node, externalNode: externalNode}
	p.backend.UpdateAnnounceVersion()
	p.backend.AddPeer(node, p2p.ProxyPurpose)
	return nil
}

func (p *proxy) RemoveProxy(node *enode.Node) {
	if p.proxyNode != nil && p.proxyNode.node.ID() == node.ID() {
		p.backend.RemovePeer(node, p2p.ProxyPurpose)
		p.proxyNode = nil
	}
}

func (p *proxy) HandleMsg(peer consensus.Peer, msgCode uint64, payload []byte) (bool, error) {
     if msgCode == istanbul.ValEnodesShareMsg {
     	err := p.handleValEnodesShareMsg(peer, payload)
	return true, err
     } else if msgCode == istanbul.FwdMsg {
       handled, err := p.handleForwardMsg(peer, payload)
       return handled, err
     }

     return false, nil
}