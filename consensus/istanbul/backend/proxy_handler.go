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
// TODO might need this later
func (p proxy) ID() enode.ID {
    return p.internalNode.ID()
}

// This type defines the set of proxies that the validator is aware of (communicated via the command line and/or the rpc api).
type proxySet struct {
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
		ps.proxiesByID[internalID] = p
        ps.valAssigner.addProxy(p)
        // TODO log that the proxy was added
	} else {
        // TODO log that the proxy already is in the proxy set
    }
}

func (ps *proxySet) getProxy(proxyID enode.ID) *proxy {
	return ps.proxiesByID[proxyID]
}

func (ps *proxySet) removeProxy(proxyID enode.ID) {
	if ps.proxiesByID[proxyID] != nil {
		delete(ps.proxiesByID, proxyID)
	}
}

func (ps *proxySet) setProxyPeer(proxyID enode.ID, peer consensus.Peer) {
	if ps.proxiesByID[proxyID] != nil && ps.proxiesByID[proxyID].peer == nil {
		ps.proxiesByID[proxyID].peer = peer
	}
	log.Warn("after:", "ps.proxiesByID[proxyID]", ps.proxiesByID[proxyID])
}

// called when a proxy peer is disconnected
func (ps *proxySet) removeProxyPeer(proxyID enode.ID) {
    if ps.proxiesByID[proxyID] != nil && ps.proxiesByID[proxyID].peer != nil {
		ps.proxiesByID[proxyID].peer = nil
	}
}

func (ps *proxySet) addValidators(validators map[common.Address]bool) {
	log.Warn("like cmon")
	ps.valAssigner.addValidators(validators)
}

func (ps *proxySet) removeValidators(validators map[common.Address]bool) {
	ps.valAssigner.removeValidators(validators)
}

func (ps *proxySet) getDisconnectedProxies(minAge time.Duration) []*enode.Node {
	return nil
}

// func (ps *proxySet) proxyAssignmentChanged(proxyID enode.ID) {
// 	// if ps.proxiesByID[ID].index != -1 {
// 	// 	heap.Fix(&ps, ps.proxiesByID[ID].index)
// 	// }
// }

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
    //
    // addValidator() valAssignments
    // removeValidator() valAssignments
}

// consistentHashingPolicy uses consistent hashing to assign validators to proxies
type consistentHashingPolicy struct {
    hashRing *hashring.HashRing
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
    return ch.valAssignments
}

// addProxy adds a proxy to the consistent hasher and recalculates all validator assignments
func (ch *consistentHashingPolicy) addProxy(proxy *proxy) {
    ch.hashRing = ch.hashRing.AddNode(proxy.ID().String())
    ch.reassignValidators()
}

// removeProxy removes a proxy to the consistent hasher and recalculates all validator assignments
func (ch *consistentHashingPolicy) removeProxy(proxy *proxy) {
    ch.hashRing = ch.hashRing.RemoveNode(proxy.ID().String())
    ch.reassignValidators()
}

func (ch *consistentHashingPolicy) addValidators(vals map[common.Address]bool) {
	log.Warn("dood")
	ch.valAssignments.addValidators(vals)
	ch.reassignValidators()
}

func (ch *consistentHashingPolicy) removeValidators(vals map[common.Address]bool) {
	ch.valAssignments.removeValidators(vals)
	ch.reassignValidators()
}

// reassignValidators recalculates all validator <-> proxy pairings
func (ch *consistentHashingPolicy) reassignValidators() {
    for val, proxyID := range ch.valAssignments.valToProxy {
        newProxyID, ok := ch.hashRing.GetNode(val.Hex())
        if ok && (proxyID == nil || newProxyID != proxyID.String()) {
            ch.valAssignments.disassociateValidator(val)
            ch.valAssignments.assignValidator(val, enode.HexID(newProxyID))
        }
    }
}



// This struct maintains the validators assignments to proxies
type valAssignments struct {
	valToProxy  map[common.Address]*enode.ID         // map of validator address -> proxy ID.  If the proxy ID is nil, then the validator is unassigned
	proxyToVals map[enode.ID]map[common.Address]bool // map of proxy ID to array of validator addresses
}

func newValAssignments() *valAssignments {
	return &valAssignments{
		valToProxy: make(map[common.Address]*enode.ID),
		proxyToVals: make(map[enode.ID]map[common.Address]bool),
	}
}

func (va *valAssignments) addValidators(vals map[common.Address]bool) {
	log.Warn("yooooooeeeeetttt", "sup", vals)
	// TODO this needs another look
	for val := range vals {
		log.Warn("valAssignments addValidator", "vals", vals)
		va.valToProxy[val] = nil
	}
}

func (va *valAssignments) removeValidators(vals map[common.Address]bool) {
	for val := range vals {
		va.disassociateValidator(val)
		delete(va.valToProxy, val)
	}
}

func (va *valAssignments) getAssignedValidatorsForProxy(proxyID enode.ID) map[common.Address]bool {
	return va.proxyToVals[proxyID]
}

func (va *valAssignments) assignValidator(valAddress common.Address, proxyID enode.ID) {
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
	proxyID := va.valToProxy[valAddress]

	if proxyID != nil {
		va.valToProxy = nil
		delete(va.proxyToVals[*proxyID], valAddress)

		if len(va.proxyToVals[*proxyID]) == 0 {
			delete(va.proxyToVals, *proxyID)
		}
	}
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
}

// func newProxyHandler() *proxyHandler {
//     return &proxyHandler{
//         quit: make(chan struct{}),
//
//         addProxies: make(chan []*ProxyNode),
//         removeProxies: make(chan []*enode.Node),
//
//         addProxyPeer: make(chan consensus.Peer),
//         delProxyPeer: make(chan consensus.Peer),
//
//         newBlockchainEpoch: make(chan struct{}),
//
//     }
// }

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

func (ph *proxyHandler) GetProxiesForAddresses(map[common.Address]bool) map[common.Address]proxy {
	return nil
}

func (ph *proxyHandler) run() {
	defer ph.loopWG.Done()

	var (
		// This data structure will store all the proxies.  It will need the proxyToValsAssignments map so that it can
		// sort the proxies based on how many validators are assigned to it.
		ps = newProxySet(newConsistentHashingPolicy())
	)

	phEpochTicker := time.NewTicker(ph.proxyHandlerEpochLength)
	defer phEpochTicker.Stop()

	// TODO come back to this

	newVals, rmVals, err := ph.checkForActiveRegValChanges(nil)
	log.Warn("suhhh???????", "newVals", newVals, "rmVals", rmVals, "err", err)
	if err == nil {
		log.Warn("okay like what")
		ps.addValidators(newVals)
		ps.removeValidators(rmVals)
	}

running:
	for {
		select {
		case <-ph.quit:
			// The proxyHandler was stopped. Run the cleanup logic.
			break running

		case addProxyNodes := <-ph.addProxies:
			// Got command to add proxy nodes.  Add any unseen proxies to the proxies set and add p2p static connections to them.
			for _, proxyNode := range addProxyNodes {
				proxyID := proxyNode.InternalFacingNode.ID()
				if ps.getProxy(proxyID) == nil && ph.p2pserver != nil {
					log.Warn("Adding proxy node", "proxyNode", proxyNode)

					ps.addProxy(proxyNode)
					// TODO: check on this
					ph.p2pserver.AddPeer(proxyNode.InternalFacingNode, p2p.ProxyPurpose)
				}
			}

		case rmProxyNodes := <-ph.removeProxies:
			// Got command to remove proxy nodes.  Disassociate the validators that were assigned to them, remove the proxies from the proxies set, and remove the p2p static connection
			for _, proxyNode := range rmProxyNodes {
				proxyID := proxyNode.ID()
				proxy := ps.getProxy(proxyID)

				if proxy != nil {
					log.Warn("Removing proxy node", "proxyNode", proxyNode)

					// Disassociate the assigned validators

					// TODO: check on this
					// ph.p2pserver.removeStatic(proxy.InternalFacingNode, p2p.ProxyPurpose)

					ps.removeProxy(proxyID)
				}
			}

        // When any peer on the p2p level is connected
		case connectedPeer := <-ph.addProxyPeer:
			// Proxied peer just connected.  Set the corresponding proxyInfo's peer info and add this peer to the recentlyPeeredProxies set
			peerNode := connectedPeer.Node()
			peerID := peerNode.ID()

            if ps.getProxy(peerID) != nil {
				log.Info("Setting proxy peer", "peerID", peerID)
                ps.setProxyPeer(peerID, connectedPeer)
            }

        // When any peer on the p2p level is disconnected
		case disconnectedPeer := <-ph.delProxyPeer:
			// Proxied peer just disconnected. Remove the proxy
			proxy := ps.getProxy(disconnectedPeer.Node().ID())
			if proxy != nil {
				ph.removeProxies <- []*enode.Node{proxy.internalNode}
			}

		case <-ph.newBlockchainEpoch:
			// New epoch.  Need to see if any of the validators changed.
			// ph.checkForActiveRegValChanges(validators)

		case <-phEpochTicker.C:
			log.Info("PH Epoch ticker", "valAssignments", ps.valAssigner.getValAssignments(), "proxiesByID", ps.proxiesByID)
			// At every proxy handler epoch, do the following
			// 1) Check for validator changes
			// 2) Kick out proxy nodes that haven't connected within 60 seconds of when it was added.
			// 3) Redistribute unassigned validators to ready peers or existing peers
			// 4) Send out val_enode_share messages to the proxies
			// 5) Do consistency checks with the proxy peers in proxy handler and proxy peers in the p2p server

			// 1) Check for validator changes
			// ph.checkForActiveRegValChanges(validators, ps)

			// 2) Kick out proxy nodes that haven't connected within 60 seconds of when it was added.
			// disconnectedProxies := ps.GetDisconnectedProxies(time.Minute)
			// if disconnectedProxies != nil && len(disconnectedProxies) > 0 {
			// 	log.Info("Disconnecting proxies", "proxies", disconnectedProxies)
			// 	ph.removeProxies <- disconnectedProxies
			// }

			// 3) Redistribute unassigned validators to ready peers and existing peers.
			// if ps.hasConnectedProxies() {
			// 	for val, proxyID := range validators {
			// 		if proxyID == nil {
			// 			validators[val] = ps.assignValidator(val)
			// 		}
			// 	}
			// }

			// 4) Send out val_enode_share messages to the proxies
			// for _, proxy := range ps.getConnectedProxied() {
			// 	go sb.sendValEnodesShareMsg(proxy.peer, proxy.externalNode, proxy.assignedValidators)
			// }

			// 5) Do consistency checks with the proxy peers in proxy handler and proxy peers in the p2p server
		}
	}
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
