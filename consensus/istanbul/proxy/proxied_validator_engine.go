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

// BackendForProxiedValidatorEngine provides the Istanbul backend application specific functions for Istanbul proxied validator engine
type BackendForProxiedValidatorEngine interface {
	// Address returns the validator's signing address
	Address() common.Address

	// IsProxiedValidator returns true if this node is a proxied validator
	IsProxiedValidator() bool

	// SelfNode returns the owner's node (if this is a proxy, it will return the external node)
	SelfNode() *enode.Node

	// Sign signs input data with the backend's ecdsa signing key
	Sign([]byte) ([]byte, error)

	// Multicast sends a message to it's connected nodes filtered on the 'addresses' parameter (where each address
	// is associated with those node's signing key)
	// If sendToSelf is set to true, then the function will send an event to self via a message event
	Multicast(addresses []common.Address, payload []byte, ethMsgCode uint64, sendToSelf bool) error

	// GetValEnodeTableEntries retrieves the entries in the valEnodeTable filtered on the "validators" parameter.
	// If the parameter is nil, then no filter will be applied.
	GetValEnodeTableEntries(validators []common.Address) (map[common.Address]*istanbul.AddressEntry, error)

	// UpdateAnnounceVersion will notify the announce protocol that this validator's valEnodeTable entry has been updated
	UpdateAnnounceVersion()

	// RetrieveEnodeCertificateMsgMap will retrieve this node's handshake enodeCertificate
	RetrieveEnodeCertificateMsgMap() map[enode.ID]*istanbul.Message

	// RetrieveValidatorConnSet will retrieve the validator connection set.
	// The parameter `retrieveCachedVersion` will specify if the function should retrieve the
	// set directly from making an EVM call (which is relatively expensive), or from the cached
	// version (which will be no more than one minute old).
	RetrieveValidatorConnSet(retrieveCachedVersion bool) (map[common.Address]bool, error)

	// AddPeer will add a static peer
	AddPeer(node *enode.Node, purpose p2p.PurposeFlag)

	// RemovePeer will remove a static peer
	RemovePeer(node *enode.Node, purpose p2p.PurposeFlag)
}

type proxiedValidatorEngine struct {
	config  *istanbul.Config
	logger  log.Logger
	backend BackendForProxiedValidatorEngine

	// Proxied validator's proxy handler
	ph *proxyHandler
}

// New creates a new proxied validator engine.
func NewProxiedValidatorEngine(backend BackendForProxiedValidatorEngine, config *istanbul.Config) (ProxiedValidatorEngine, error) {
	if !backend.IsProxiedValidator() {
		return nil, ErrNodeNotProxiedValidator
	}

	pv := &proxiedValidatorEngine{
		config:  config,
		logger:  log.New(),
		backend: backend,
	}

	pv.ph = newProxyHandler(backend, pv)

	return pv, nil
}

func (pv *proxiedValidatorEngine) Start() error {
	if err := pv.ph.Start(); err != nil {
		return err
	}

	if len(pv.config.ProxyConfigs) > 0 {
		pv.ph.addProxies <- pv.config.ProxyConfigs
	}

	pv.logger.Info("Proxied validator engine started")
	return nil
}

func (pv *proxiedValidatorEngine) Stop() error {
	if err := pv.ph.Stop(); err != nil {
		return err
	}

	pv.logger.Info("Proxy engine stopped")
	return nil
}

func (pv *proxiedValidatorEngine) AddProxy(node, externalNode *enode.Node) error {
	pv.ph.addProxies <- []*istanbul.ProxyConfig{&istanbul.ProxyConfig{InternalNode: node, ExternalNode: externalNode}}

	return nil
}

func (pv *proxiedValidatorEngine) RemoveProxy(node *enode.Node) error {
	pv.ph.removeProxies <- []*enode.Node{node}

	return nil
}

func (pv *proxiedValidatorEngine) RegisterProxyPeer(proxyPeer consensus.Peer) error {
	logger := pv.logger.New("func", "RegisterProxyPeer")
	if proxyPeer.PurposeIsSet(p2p.ProxyPurpose) {
		logger.Info("Got new proxy peer", "proxyPeer", proxyPeer)
		pv.ph.addProxyPeer <- proxyPeer
	} else {
		logger.Error("Unauthorized connected peer to the proxied validator", "peerID", proxyPeer.Node().ID())
		return errUnauthorizedProxiedValidator
	}

	return nil
}

func (pv *proxiedValidatorEngine) UnregisterProxyPeer(proxyPeer consensus.Peer) {
	if proxyPeer.PurposeIsSet(p2p.ProxyPurpose) {
		pv.ph.removeProxyPeer <- proxyPeer
	}
}

func (pv *proxiedValidatorEngine) GetValidatorProxyAssignments() (map[common.Address]*enode.Node, error) {
	logger := pv.logger.New("func", "GetValidatorProxyAssignments")
	if !pv.ph.Running() {
		return nil, ErrStoppedProxyHandler
	}

	valAssignments, err := pv.ph.GetValidatorAssignments(nil)
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
}

func (pv *proxiedValidatorEngine) GetProxiesAndValAssignments() ([]*proxy, map[enode.ID][]common.Address, error) {
	return pv.ph.GetProxiesAndValAssignments()
}
