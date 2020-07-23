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
	"github.com/ethereum/go-ethereum/p2p/enode"
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
	Multicast(addresses []common.Address, payload []byte, ethMsgCode uint64, sendToSelf bool) error

	// GetValEnodeTableEntries retrieves the entries in the valEnodeTable filtered on the "validators" parameter.
	// If the parameter is nil, then no filter will be applied.
	GetValEnodeTableEntries(validators []common.Address) (map[common.Address]*istanbul.AddressEntry, error)

	// RewriteValEnodeTableEntries will rewrite the val enode table with "entries" rows.
	RewriteValEnodeTableEntries(entries []*istanbul.AddressEntry) error

	// SetEnodeCertificateMsg will set this node's enodeCertificate to be used for connection handshakes
	SetEnodeCertificateMsgMap(enodeCertificateMsgMap map[enode.ID]*istanbul.Message) error

	// VerifyPendingBlockValidatorSignature is a message validation function to verify that a message's sender is within the validator set
	// of the current pending block and that the message's address field matches the message's signature's signer
	VerifyPendingBlockValidatorSignature(data []byte, sig []byte) (common.Address, error)

	// VerifyValidatorConnectionSetSignature is a message validation function to verify that a message's sender is within the
	// validator connection set and that the message's address field matches the message's signature's signer
	VerifyValidatorConnectionSetSignature(data []byte, sig []byte) (common.Address, error)
}

type proxyEngine struct {
	config  *istanbul.Config
	logger  log.Logger
	backend BackendForProxyEngine

	// Proxy's validator
	// Right now, we assume that there is at most one proxied peer for a proxy
	// Proxy's validator
	proxiedValidator consensus.Peer
}

// New creates a new proxy engine.
func NewProxyEngine(backend BackendForProxyEngine, config *istanbul.Config) (ProxyEngine, error) {
	if !backend.IsProxy() {
		return nil, ErrNodeNotProxy
	}

	p := &proxyEngine{
		config:  config,
		logger:  log.New(),
		backend: backend,
	}

	return p, nil
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

func (p *proxyEngine) RegisterProxiedValidatorPeer(proxiedValidatorPeer consensus.Peer) {
	// TODO: Does this need a lock?
	p.proxiedValidator = proxiedValidatorPeer
}

func (p *proxyEngine) UnregisterProxiedValidatorPeer(proxiedValidatorPeer consensus.Peer) {
	if p.proxiedValidator != nil && proxiedValidatorPeer.Node().ID() == p.proxiedValidator.Node().ID() {
		p.proxiedValidator = nil
	}
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
