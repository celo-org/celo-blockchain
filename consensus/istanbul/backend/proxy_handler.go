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
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
    "github.com/serialx/hashring"
)

// This type defines a proxy
type proxy struct {
	internalNode *enode.Node    // Enode for the proxy's internal network interface
	externalNode *enode.Node    // Enode for the proxy's external network interface
	peer         consensus.Peer // Connected proxy peer.  Is nil if this node is not connected to the proxy
	addTimestamp time.Time      // Timestamp when this proxy was added to the proxy set.  If it hasn't been connected within 60 seconds, then it's removed from the proxy set.
}

func (p proxy) ID() enode.ID {
    return p.internalNode.ID()
}

// This type defines the set of proxies that the validator is aware of (communicated via the command line and/or the rpc api).
type proxySet struct {
	mu sync.RWMutex // protects proxiesByID
	proxiesByID map[enode.ID]*proxy
    valAssigner assignmentPolicy
}

func newProxySet(assignmentPolicy assignmentPolicy) *proxySet {
	return &proxySet{
		proxiesByID: make(map[enode.ID]*proxy),
		valAssigner: assignmentPolicy,
	}
}

func (ps *proxySet) addProxy(proxyNodes *istanbul.ProxyNodes) {
    internalID := proxyNodes.InternalFacingNode.ID()
	if _, ok := ps.proxiesByID[internalID]; !ok {
        p := &proxy{
            internalNode: proxyNodes.InternalFacingNode,
            externalNode: proxyNodes.ExternalFacingNode,
            peer: nil,
            addTimestamp: time.Now(),
        }
		ps.mu.Lock()
		ps.proxiesByID[internalID] = p
		ps.mu.Unlock()
        ps.valAssigner.addProxy(p)
        // TODO log that the proxy was added
	} else {
        // TODO log that the proxy already is in the proxy set
    }
}

func (ps *proxySet) getProxy(proxyID enode.ID) *proxy {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	return ps.proxiesByID[proxyID]
}

func (ps *proxySet) removeProxy(proxyID enode.ID) {
	proxy := ps.getProxy(proxyID)
	if proxy != nil {
		ps.valAssigner.removeProxy(proxy)
		ps.mu.Lock()
		delete(ps.proxiesByID, proxyID)
		ps.mu.Unlock()
	}
}

func (ps *proxySet) setProxyPeer(proxyID enode.ID, peer consensus.Peer) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.proxiesByID[proxyID] != nil {
		ps.proxiesByID[proxyID].peer = peer
	}
	log.Warn("after:", "ps.proxiesByID[proxyID]", ps.proxiesByID[proxyID])
}

// called when a proxy peer is disconnected
func (ps *proxySet) removeProxyPeer(proxyID enode.ID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

    if ps.proxiesByID[proxyID] != nil && ps.proxiesByID[proxyID].peer != nil {
		ps.proxiesByID[proxyID].peer = nil
	}
}

func (ps *proxySet) addValidators(validators map[common.Address]bool) {
	ps.valAssigner.addValidators(validators)
}

func (ps *proxySet) removeValidators(validators map[common.Address]bool) {
	ps.valAssigner.removeValidators(validators)
}

// getValidatorProxies returns a map of validator -> proxy for each validator
// address specified in validators. If validators is nil, all pairings are returned
func (ps *proxySet) getValidatorProxies(validators map[common.Address]bool) map[common.Address]*proxy {
	proxies := make(map[common.Address]*proxy)

	if validators == nil {
		for val, proxyID := range ps.valAssigner.getValAssignments().valToProxy {
			if proxyID != nil {
				proxy := ps.getProxy(*proxyID)
				if proxy.peer != nil {
					proxies[val] = proxy
				}
			}
		}
	} else {
		for val := range validators {
			proxyID := ps.valAssigner.getValAssignments().valToProxy[val]
			if proxyID != nil {
				proxy := ps.getProxy(*proxyID)
				if proxy.peer != nil {
					proxies[val] = proxy
				}
			}
		}
	}

	return proxies
}

func (ps *proxySet) getValidatorProxyPeers(validators []common.Address) map[enode.ID]consensus.Peer {
	peers := make(map[enode.ID]consensus.Peer)

	if validators == nil {
		for _, proxy := range ps.proxiesByID {
			if proxy.peer != nil {
				peers[proxy.ID()] = proxy.peer
			}
		}
	} else {
		for _, val := range validators {
			proxyID := ps.valAssigner.getValAssignments().valToProxy[val]
			proxy := ps.getProxy(*proxyID)
			if proxy != nil && proxy.peer != nil {
				peers[proxy.ID()] = proxy.peer
			}
		}
	}

	return peers
}

// GetNonPeeredProxies returns proxies that are not peered with and were added
// as a proxy at least minAge ago
func (ps *proxySet) getNonPeeredProxyNodes(minAge time.Duration) []*enode.Node {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var nonPeeredProxies []*enode.Node
	for _, proxy := range ps.proxiesByID {
		if proxy.peer == nil && time.Now().Sub(proxy.addTimestamp) >= minAge {
			nonPeeredProxies = append(nonPeeredProxies, proxy.internalNode)
		}
	}
	return nonPeeredProxies
}

// getProxyValidators returns the validators that a proxy is assigned to
func (ps *proxySet) getProxyValidators(proxyID enode.ID) map[common.Address]bool {
	return ps.valAssigner.getValAssignments().getAssignedValidatorsForProxy(proxyID)
}

func (ps *proxySet) getValidators() map[common.Address]bool {
	return ps.valAssigner.getValAssignments().getValidators()
}

// TODO come back to this
func (ps *proxySet) hasConnectedProxies() bool {
	return true
}

// assignmentPolicy is intended to allow different
// solutions for assigning validators to proxies
type assignmentPolicy interface {
    addProxy(proxy *proxy)
    removeProxy(proxy *proxy)

    getValAssignments() *valAssignments

	addValidators(map[common.Address]bool)
	removeValidators(map[common.Address]bool)

	reassignValidators()
}

// consistentHashingPolicy uses consistent hashing to assign validators to proxies
type consistentHashingPolicy struct {
	mu sync.RWMutex // protects hashRing
    hashRing *hashring.HashRing // used for consistent hashing
    valAssignments *valAssignments
}

func newConsistentHashingPolicy() *consistentHashingPolicy {
    return &consistentHashingPolicy{
        // TODO add initial proxies
        hashRing: hashring.New(nil),
		valAssignments: newValAssignments(),
    }
}

func (ch *consistentHashingPolicy) getValAssignments() *valAssignments {
    return ch.valAssignments.copy()
}

// addProxy adds a proxy to the consistent hasher and recalculates all validator assignments
func (ch *consistentHashingPolicy) addProxy(proxy *proxy) {
	ch.mu.Lock()
    ch.hashRing = ch.hashRing.AddNode(proxy.ID().String())
	ch.mu.Unlock()
    ch.reassignValidators()
}

// removeProxy removes a proxy to the consistent hasher and recalculates all validator assignments
func (ch *consistentHashingPolicy) removeProxy(proxy *proxy) {
	ch.mu.Lock()
    ch.hashRing = ch.hashRing.RemoveNode(proxy.ID().String())
	ch.mu.Unlock()
    ch.reassignValidators()
}

func (ch *consistentHashingPolicy) addValidators(vals map[common.Address]bool) {
	ch.valAssignments.addValidators(vals)
	ch.reassignValidators()
}

func (ch *consistentHashingPolicy) removeValidators(vals map[common.Address]bool) {
	ch.valAssignments.removeValidators(vals)
	ch.reassignValidators()
}

// reassignValidators recalculates all validator <-> proxy pairings
func (ch *consistentHashingPolicy) reassignValidators() {
	assignments := ch.getValAssignments()
    for val, proxyID := range assignments.valToProxy {
		ch.mu.RLock()
        newProxyID, ok := ch.hashRing.GetNode(val.Hex())
		ch.mu.RUnlock()
        if ok && (proxyID == nil || newProxyID != proxyID.String()) {
            ch.valAssignments.disassociateValidator(val)
            ch.valAssignments.assignValidator(val, enode.HexID(newProxyID))
        }
    }
}

// This struct maintains the validators assignments to proxies
type valAssignments struct {
	mu          sync.RWMutex // protects both valToProxy and proxyToVals
	valToProxy  map[common.Address]*enode.ID         // map of validator address -> proxy ID.  If the proxy ID is nil, then the validator is unassigned
	proxyToVals map[enode.ID]map[common.Address]bool // map of proxy ID to array of validator addresses
}

func newValAssignments() *valAssignments {
	return &valAssignments{
		valToProxy: make(map[common.Address]*enode.ID),
		proxyToVals: make(map[enode.ID]map[common.Address]bool),
	}
}

func (va *valAssignments) copy() *valAssignments {
	va.mu.RLock()
	defer va.mu.RUnlock()

	c := newValAssignments()
	for k, v := range va.valToProxy {
		c.valToProxy[k] = v
	}
	for k, v := range va.proxyToVals {
		c.proxyToVals[k] = make(map[common.Address]bool)
		for k1, v1 := range v {
			c.proxyToVals[k][k1] = v1
		}
	}
	return c
}

func (va *valAssignments) addValidators(vals map[common.Address]bool) {
	va.mu.Lock()
	defer va.mu.Unlock()
	for val := range vals {
		va.valToProxy[val] = nil
	}
}

func (va *valAssignments) removeValidators(vals map[common.Address]bool) {
	for val := range vals {
		va.disassociateValidator(val)
		va.mu.Lock()
		delete(va.valToProxy, val)
		va.mu.Unlock()
	}
}

func (va *valAssignments) getAssignedValidatorsForProxy(proxyID enode.ID) map[common.Address]bool {
	va.mu.RLock()
	defer va.mu.RUnlock()

	return va.proxyToVals[proxyID]
}

func (va *valAssignments) assignValidator(valAddress common.Address, proxyID enode.ID) {
	va.mu.Lock()
	defer va.mu.Unlock()

	va.valToProxy[valAddress] = &proxyID

	if _, ok := va.proxyToVals[proxyID]; !ok {
		va.proxyToVals[proxyID] = make(map[common.Address]bool)
	}

	va.proxyToVals[proxyID][valAddress] = true
}

// TODO implement this to remove all validators from a proxy
func (va *valAssignments) disassociateProxyValidators(proxyID enode.ID) {

}

func (va *valAssignments) disassociateValidator(valAddress common.Address) {
	va.mu.Lock()
	defer va.mu.Unlock()

	proxyID := va.valToProxy[valAddress]

	if proxyID != nil {
		va.valToProxy[valAddress] = nil
		delete(va.proxyToVals[*proxyID], valAddress)

		if len(va.proxyToVals[*proxyID]) == 0 {
			delete(va.proxyToVals, *proxyID)
		}
	}
}

func (va *valAssignments) getValidators() map[common.Address]bool {
	va.mu.RLock()
	defer va.mu.RUnlock()

	vals := make(map[common.Address]bool)

	for val := range va.valToProxy {
		vals[val] = true
	}
	return vals
}

// This struct defines the handler that will manage all of the proxies and
// validator assignments to them
type proxyHandler struct {
	lock    sync.Mutex // protects the "running" field
	running bool

	loopWG sync.WaitGroup
	quit   chan struct{}

	addProxies    chan []*istanbul.ProxyNodes // This channel is for adding new proxies specified via command line or rpc api
	removeProxies chan []*enode.Node          // This channel is for removing proxies specified via rpc api

	addProxyPeer chan consensus.Peer // This channel is for newly peered proxies
	delProxyPeer chan consensus.Peer // This channel is for newly disconnected peers

	newBlockchainEpoch chan struct{} // This channel is when a new blockchain epoch has started and we need to check if any validators are removed or added

	proxyHandlerEpochLength time.Duration

	sb *Backend
	p2pserver consensus.P2PServer

	ps *proxySet
}

func (ph *proxyHandler) Start() error {
	ph.lock.Lock()
	defer ph.lock.Unlock()
	if ph.running {
		return errors.New("proxyHandler already running")
	}

	ph.running = true

	ph.quit = make(chan struct{})
	ph.addProxies = make(chan []*istanbul.ProxyNodes)
	ph.removeProxies = make(chan []*enode.Node)
	ph.addProxyPeer = make(chan consensus.Peer)
	ph.delProxyPeer = make(chan consensus.Peer)
	ph.ps = newProxySet(newConsistentHashingPolicy())

	ph.proxyHandlerEpochLength = time.Minute / 6.0

	ph.loopWG.Add(1)
	go ph.run()
	return nil
}

func (ph *proxyHandler) setP2PServer(p2pserver consensus.P2PServer) {
	ph.p2pserver = p2pserver
}

func (ph *proxyHandler) Stop() {
	ph.lock.Lock()
	defer ph.lock.Unlock()
	if !ph.running {
		return
	}
	ph.running = false
	close(ph.quit)
	ph.loopWG.Wait()
}

func (ph *proxyHandler) getValidatorProxies(validators map[common.Address]bool) map[common.Address]*proxy {
	return ph.ps.getValidatorProxies(validators)
}

func (ph *proxyHandler) getValidatorProxyPeers(validators []common.Address) map[enode.ID]consensus.Peer {
	return ph.ps.getValidatorProxyPeers(validators)
}

func (ph *proxyHandler) run() {
	defer ph.loopWG.Done()

	phEpochTicker := time.NewTicker(ph.proxyHandlerEpochLength)
	defer phEpochTicker.Stop()

	ph.updateValidators()

running:
	for {
		select {
		case <-ph.quit:
			// The proxyHandler was stopped. Run the cleanup logic.
			break running

		case addProxyNodes := <-ph.addProxies:
			// Got command to add proxy nodes.
			// Add any unseen proxies to the proxies set and add p2p static connections to them.
			for _, proxyNode := range addProxyNodes {
				proxyID := proxyNode.InternalFacingNode.ID()
				if ph.ps.getProxy(proxyID) == nil && ph.p2pserver != nil {
					log.Warn("Adding proxy node", "proxyNode", proxyNode)

					ph.ps.addProxy(proxyNode)
					ph.p2pserver.AddPeer(proxyNode.InternalFacingNode, p2p.ProxyPurpose)
				}
			}

		case rmProxyNodes := <-ph.removeProxies:
			// Got command to remove proxy nodes.
			// Remove the proxy and remove the p2p static connection
			for _, proxyNode := range rmProxyNodes {
				proxyID := proxyNode.ID()
				proxy := ph.ps.getProxy(proxyID)

				if proxy != nil {
					log.Warn("Removing proxy node", "proxyNode", proxyNode)

					ph.ps.removeProxy(proxyID)
					ph.p2pserver.RemovePeer(proxy.internalNode, p2p.ProxyPurpose)
				}
			}

        // When any peer on the p2p level is connected
		case connectedPeer := <-ph.addProxyPeer:
			// Proxied peer just connected.  Set the corresponding proxyInfo's peer info and add this peer to the recentlyPeeredProxies set
			peerNode := connectedPeer.Node()
			peerID := peerNode.ID()

            if ph.ps.getProxy(peerID) != nil {
				log.Info("Setting proxy peer", "peerID", peerID)
                ph.ps.setProxyPeer(peerID, connectedPeer)
            }

        // When any peer on the p2p level is disconnected
		case disconnectedPeer := <-ph.delProxyPeer:
			peerID := disconnectedPeer.Node().ID()
			if ph.ps.getProxy(peerID) != nil {
				log.Info("Disconnected proxy peer", "peerID", peerID)
                ph.ps.setProxyPeer(peerID, nil)
            }

		case <-ph.newBlockchainEpoch:
			// New epoch.  Need to see if any of the validators changed.
			ph.updateValidators()

		case <-phEpochTicker.C:
			log.Info("PH Epoch ticker", "valAssignments", ph.ps.valAssigner.getValAssignments(), "proxiesByID", ph.ps.proxiesByID)

			// At every proxy handler epoch, do the following
			// 1) Check for validator changes
			// 2) Kick out proxy nodes that haven't connected within 60 seconds of when it was added.
			// 3) Redistribute unassigned validators to ready peers or existing peers
			// 4) Send out val_enode_share messages to the proxies
			// 5) Do consistency checks with the proxy peers in proxy handler and proxy peers in the p2p server

			// 1) Check for validator changes
			ph.updateValidators()

			// 2) Kick out proxy nodes that haven't connected within 60 seconds of when it was added.
			// I think this might be a bad idea-- no step 3 and 4 do not wait for this to happen...
			nonPeeredProxies := ph.ps.getNonPeeredProxyNodes(time.Minute)
			if nonPeeredProxies != nil && len(nonPeeredProxies) > 0 {
				log.Info("Non peered proxies", "proxies", nonPeeredProxies)
				ph.removeProxies <- nonPeeredProxies
			}

			// 3) Redistribute unassigned validators to ready peers and existing peers.
			ph.ps.valAssigner.reassignValidators()

			// 4) Send out val_enode_share messages to the proxies
			for _, proxy := range ph.ps.proxiesByID {
				if proxy.peer != nil {
					assignedValidators := ph.ps.getProxyValidators(proxy.ID())
					log.Warn("Hey we're sending out a val enode sharm msg", "proxy.ID()", proxy.ID(), "proxy", proxy, "assignedValidators", assignedValidators)
					go ph.sb.sendValEnodesShareMsg(proxy.peer, proxy.externalNode, assignedValidators)
				}
			}

			// 5) Do consistency checks with the proxy peers in proxy handler and proxy peers in the p2p server
		}
	}
}

func (ph *proxyHandler) updateValidators() error {
	newVals, rmVals, err := ph.checkForActiveRegValChanges(ph.ps.getValidators())
	log.Warn("Inside updateValidators", "newVals", newVals, "rmVals", rmVals, "err", err)
	if err != nil {
		return err
	}
	ph.ps.addValidators(newVals)
	ph.ps.removeValidators(rmVals)
	return nil
}

// This function will see if there are any changes in the ActiveAndRegisteredValidator set,
// compared to the `validators` parameter.
func (ph *proxyHandler) checkForActiveRegValChanges(validators map[common.Address]bool) (newVals map[common.Address]bool, rmVals map[common.Address]bool, err error) {
	// Get the set of active and registered validators
	activeAndRegVals, err := ph.sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		log.Warn("Proxy Handler couldn't get the active and registered validators", "err", err)
		return nil, nil, err
	}

	newVals = activeAndRegVals
	rmVals = make(map[common.Address]bool)

	for oldVal := range validators {
		if newVals[oldVal] {
			delete(newVals, oldVal)
		} else {
			rmVals[oldVal] = true
		}
	}
	return newVals, rmVals, nil
}
