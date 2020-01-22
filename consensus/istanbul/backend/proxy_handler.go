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
	"fmt"
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
	peer         consensus.Peer // Connected proxy peer. Is nil if this node is not connected to the proxy
	dcTimestamp  time.Time      // Timestamp when this proxy was added to the proxy set. Maybe? -> If it hasn't been connected within 60 seconds, then it's removed from the proxy set.
}

type ProxyInfo struct {
	InternalNode *enode.Node `json:"internalEnodeUrl"`
	ExternalNode *enode.Node `json:"externalEnodeUrl"`
	IsPeered     bool `json:"isPeered"`
	DcTimestamp  int64 `json:"disconnectedTimestamp"` // Unix time of the last disconnect of the peer
}

func (p proxy) ID() enode.ID {
    return p.internalNode.ID()
}

func (p proxy) Info() ProxyInfo {
	return ProxyInfo{
		InternalNode: p.internalNode,
		ExternalNode: p.externalNode,
		IsPeered: p.peer != nil,
		DcTimestamp: p.dcTimestamp.Unix(),
	}
}

func (p proxy) String() string {
	return fmt.Sprintf("{internalNode: %v, externalNode %v, dcTimestamp: %v, ID: %v}", p.internalNode, p.externalNode, p.dcTimestamp, p.ID())
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

// addProxy adds a proxy to the proxySet if it does not exist.
// The valAssigner is not made aware of the proxy until after the proxy
// is peered with.
func (ps *proxySet) addProxy(proxyNodes *istanbul.ProxyNodes) {
    internalID := proxyNodes.InternalFacingNode.ID()
	if _, ok := ps.proxiesByID[internalID]; !ok {
        p := &proxy{
            internalNode: proxyNodes.InternalFacingNode,
            externalNode: proxyNodes.ExternalFacingNode,
            peer: nil,
            dcTimestamp: time.Now(),
        }
		ps.proxiesByID[internalID] = p
	}
}

// getProxy returns the proxy in the proxySet with ID proxyID
func (ps *proxySet) getProxy(proxyID enode.ID) *proxy {
	return ps.proxiesByID[proxyID]
}

// addProxy removes a proxy with ID proxyID from the proxySet and valAssigner
func (ps *proxySet) removeProxy(proxyID enode.ID) {
	proxy := ps.getProxy(proxyID)
	if proxy != nil {
		ps.valAssigner.removeProxy(proxy)
		delete(ps.proxiesByID, proxyID)
	}
}

// setProxyPeer sets the peer for a proxy with ID proxyID.
// The valAssigner is then made aware of the proxy, which is now eligible
// to be assigned to validators
func (ps *proxySet) addProxyPeer(proxyID enode.ID, peer consensus.Peer) {
	proxy := ps.proxiesByID[proxyID]
	if proxy != nil {
		proxy.peer = peer
		ps.valAssigner.addProxy(proxy)
	}
}

// removeProxyPeer sets the peer for a proxy with ID proxyID to nil
func (ps *proxySet) removeProxyPeer(proxyID enode.ID) {
	proxy := ps.proxiesByID[proxyID]
	if proxy != nil {
		proxy.peer = nil
		proxy.dcTimestamp = time.Now()
	}
}

// addValidators adds validators to be assigned by the valAssigner
func (ps *proxySet) addValidators(validators map[common.Address]bool) {
	ps.valAssigner.addValidators(validators)
}

// addValidators removes validators to be from the valAssigner
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

// getValidatorProxyPeers returns the non-nil peers of the proxies that are assigned to
// the validators specified in `validators`. If validators is nil, all non-nil peers are
// returned
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
			if proxyID == nil {
				log.Warn("No proxy assigned", "val", val)
				continue
			}

			proxy := ps.getProxy(*proxyID)
			if proxy != nil && proxy.peer != nil {
				peers[proxy.ID()] = proxy.peer
			}
		}
	}

	return peers
}

// unassignDisconnectedProxies unassigns proxies that are not peered with
// whose dcTimestamp is at least minAge ago
func (ps *proxySet) unassignDisconnectedProxies(minAge time.Duration) {
	for proxyID := range ps.valAssigner.getValAssignments().proxyToVals {
		proxy := ps.getProxy(proxyID)
		if proxy == nil && proxy.peer == nil && time.Now().Sub(proxy.dcTimestamp) >= minAge {
			log.Error("Unassigning disconnected proxy", "proxy", proxy.String())
			ps.valAssigner.removeProxy(proxy)
		}
	}
}

// getProxyValidators returns the validators that a proxy is assigned to
func (ps *proxySet) getProxyValidators(proxyID enode.ID) map[common.Address]bool {
	return ps.valAssigner.getValAssignments().getAssignedValidatorsForProxy(proxyID)
}

// getValidators returns all validators that are known by the valAssigner
func (ps *proxySet) getValidators() map[common.Address]bool {
	return ps.valAssigner.getValAssignments().getValidators()
}

// TODO come back to this- not sure why this was here to begin with
func (ps *proxySet) hasConnectedProxies() bool {
	return true
}

// getProxyInfo returns basic info on all the proxies in the proxySet
func (ps *proxySet) getProxyInfo() []ProxyInfo {
	proxiesByID := ps.proxiesByID
	proxies := make([]ProxyInfo, len(proxiesByID))

	i := 0
	for _, proxy := range proxiesByID {
		proxies[i] = proxy.Info()
		i++
	}
	return proxies
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

// consistentHashingPolicy uses consistent hashing to assign validators to proxies.
// Validator <-> proxy pairings are recalculated every time a proxy or validator
// is added/removed
type consistentHashingPolicy struct {
    hashRing *hashring.HashRing // used for consistent hashing
    valAssignments *valAssignments
}

func newConsistentHashingPolicy() *consistentHashingPolicy {
    return &consistentHashingPolicy{
        // TODO add initial proxies?
        hashRing: hashring.New(nil),
		valAssignments: newValAssignments(),
    }
}

// getValAssignments returns a copy of the current validator assignments
// to preserve thread safety
func (ch *consistentHashingPolicy) getValAssignments() *valAssignments {
    return ch.valAssignments
}

// addProxy adds a proxy to the consistent hasher and recalculates all validator assignments
func (ch *consistentHashingPolicy) addProxy(proxy *proxy) {
    ch.hashRing = ch.hashRing.AddNode(proxy.ID().String())
    ch.reassignValidators()
}

// removeProxy removes a proxy from the consistent hasher and recalculates all validator assignments
func (ch *consistentHashingPolicy) removeProxy(proxy *proxy) {
    ch.hashRing = ch.hashRing.RemoveNode(proxy.ID().String())
    ch.reassignValidators()
}

// addValidators adds validators to the valAssignments struct and recalculates
// all validator assignments
func (ch *consistentHashingPolicy) addValidators(vals map[common.Address]bool) {
	ch.valAssignments.addValidators(vals)
	ch.reassignValidators()
}

// addValidators removes validators from the valAssignments struct and recalculates
// all validator assignments
func (ch *consistentHashingPolicy) removeValidators(vals map[common.Address]bool) {
	ch.valAssignments.removeValidators(vals)
	ch.reassignValidators()
}

// reassignValidators recalculates all validator <-> proxy pairings
func (ch *consistentHashingPolicy) reassignValidators() {
	assignments := ch.getValAssignments()
    for val, proxyID := range assignments.valToProxy {
        newProxyID, ok := ch.hashRing.GetNode(val.Hex())
		// if the proxy for a validator changes, make the change
        if ok && (proxyID == nil || newProxyID != proxyID.String()) {
            ch.valAssignments.unassignValidator(val)
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

// addValidators adds validators to valToProxy without an assigned proxy
func (va *valAssignments) addValidators(vals map[common.Address]bool) {
	for val := range vals {
		va.valToProxy[val] = nil
	}
}

// removeValidators removes validators from any proxy assignments and deletes
// them from valToProxy
func (va *valAssignments) removeValidators(vals map[common.Address]bool) {
	for val := range vals {
		va.unassignValidator(val)
		delete(va.valToProxy, val)
	}
}

// getAssignedValidatorsForProxy gives all validator addresses that are assigned
// to the proxy with ID proxyID
func (va *valAssignments) getAssignedValidatorsForProxy(proxyID enode.ID) map[common.Address]bool {
	return va.proxyToVals[proxyID]
}

// assignValidator assigns a validator with address valAddress to the proxy
// with ID proxyID
func (va *valAssignments) assignValidator(valAddress common.Address, proxyID enode.ID) {
	va.valToProxy[valAddress] = &proxyID

	if _, ok := va.proxyToVals[proxyID]; !ok {
		va.proxyToVals[proxyID] = make(map[common.Address]bool)
	}

	va.proxyToVals[proxyID][valAddress] = true
}

// TODO implement this to remove all validators from a proxy
// Maybe? look into if needed
func (va *valAssignments) disassociateProxyValidators(proxyID enode.ID) {

}

// unassignValidator unassigns a validator with address valAddress from
// its proxy. If it was never assigned, this does nothing
func (va *valAssignments) unassignValidator(valAddress common.Address) {
	proxyID := va.valToProxy[valAddress]

	if proxyID != nil {
		va.valToProxy[valAddress] = nil
		delete(va.proxyToVals[*proxyID], valAddress)

		if len(va.proxyToVals[*proxyID]) == 0 {
			delete(va.proxyToVals, *proxyID)
		}
	}
}

// getValidators returns all validator addresses that are found in valToProxy
func (va *valAssignments) getValidators() map[common.Address]bool {
	vals := make(map[common.Address]bool)

	for val := range va.valToProxy {
		vals[val] = true
	}
	return vals
}

type validatorProxyPeersRequest struct {
	validators []common.Address
	resultCh   chan map[enode.ID]consensus.Peer
}

type validatorProxiesRequest struct {
	validators map[common.Address]bool
	resultCh   chan map[common.Address]*proxy
}

// This struct defines the handler that will manage all of the proxies and
// validator assignments to them
type proxyHandler struct {
	lock    sync.Mutex // protects the "running" field
	running bool       // indicates if `run` is currently being run in a goroutine

	loopWG sync.WaitGroup
	quit   chan struct{}

	addProxies    chan []*istanbul.ProxyNodes // This channel is for adding new proxies specified via command line or rpc api
	removeProxies chan []*enode.Node          // This channel is for removing proxies specified via rpc api

	addProxyPeer chan consensus.Peer // This channel is for newly peered proxies
	delProxyPeer chan consensus.Peer // This channel is for newly disconnected peers

	getValidatorProxyPeers chan *validatorProxyPeersRequest
	getValidatorProxies    chan *validatorProxiesRequest
	getProxyInfo           chan chan []ProxyInfo

	newBlockchainEpoch chan struct{} // This channel is when a new blockchain epoch has started and we need to check if any validators are removed or added

	proxyHandlerEpochLength time.Duration // The duration of time between proxy handler epochs, which are occasional check-ins to ensure proxy/validator assignments are as intended

	sb *Backend
	p2pserver consensus.P2PServer

	ps *proxySet // Used to keep track of proxies & validators the proxies are associated with
}

func newProxyHandler(sb *Backend) *proxyHandler {
	ph := &proxyHandler{
		sb: sb,
	}

	ph.running = false

	ph.quit = make(chan struct{})
	ph.addProxies = make(chan []*istanbul.ProxyNodes)
	ph.removeProxies = make(chan []*enode.Node)
	ph.addProxyPeer = make(chan consensus.Peer)
	ph.delProxyPeer = make(chan consensus.Peer)
	ph.ps = newProxySet(newConsistentHashingPolicy())

	ph.getValidatorProxyPeers = make(chan *validatorProxyPeersRequest)
	ph.getValidatorProxies = make(chan *validatorProxiesRequest)
	ph.getProxyInfo = make(chan chan []ProxyInfo)

	// TODO change back to a minute after testing changes
	ph.proxyHandlerEpochLength = time.Minute / 6.0

	return ph
}

// Start begins the proxyHandler
func (ph *proxyHandler) Start() error {
	ph.lock.Lock()
	defer ph.lock.Unlock()
	if ph.running {
		return errors.New("proxyHandler already running")
	}
	ph.running = true

	ph.loopWG.Add(1)
	go ph.run()
	return nil
}

// isRunning returns if `run` is currently running in a goroutine
func (ph *proxyHandler) isRunning() bool {
	ph.lock.Lock()
	defer ph.lock.Unlock()

	return ph.running
}

// setP2PServer sets the p2pserver
func (ph *proxyHandler) setP2PServer(p2pserver consensus.P2PServer) {
	ph.p2pserver = p2pserver
}

// Stop stops the goroutine `run` if it is currently running
func (ph *proxyHandler) Stop() {
	ph.lock.Lock()
	defer ph.lock.Unlock()
	if !ph.running {
		return
	}
	ph.running = false
	ph.quit <- struct{}{}
	ph.loopWG.Wait()
}

// run handles changes to proxies, validators, and performs occasional check-ins
// that proxy/validator assignments are as expected
func (ph *proxyHandler) run() {
	defer ph.loopWG.Done()

	phEpochTicker := time.NewTicker(ph.proxyHandlerEpochLength)
	defer phEpochTicker.Stop()

	ph.updateValidators()

loop:
	for {
		select {
		case <-ph.quit:
			// The proxyHandler was stopped
			break loop

		case addProxyNodes := <-ph.addProxies:
			// Got command to add proxy nodes.
			// Add any unseen proxies to the proxy set and add p2p static connections to them.
			for _, proxyNode := range addProxyNodes {
				proxyID := proxyNode.InternalFacingNode.ID()
				if ph.ps.getProxy(proxyID) != nil {
					log.Warn("Proxy is already in the proxy set", "proxyNode", proxyNode, "proxyID", proxyID)
					continue
				}
				if ph.p2pserver == nil {
					log.Warn("Proxy handler p2pserver not set, cannot add proxy", "proxyNode", proxyNode, "proxyID", proxyID)
					continue
				}
				log.Warn("Adding proxy node", "proxyNode", proxyNode, "proxyID", proxyID)
				ph.ps.addProxy(proxyNode)
				ph.p2pserver.AddPeer(proxyNode.InternalFacingNode, p2p.ProxyPurpose)
			}

		case rmProxyNodes := <-ph.removeProxies:
			// Got command to remove proxy nodes.
			// Remove the proxy and remove the p2p static connection
			for _, proxyNode := range rmProxyNodes {
				proxyID := proxyNode.ID()
				proxy := ph.ps.getProxy(proxyID)
				if proxy == nil {
					log.Warn("Proxy is not in the proxy set", "proxy", proxyNode, "proxyID", proxyID)
					continue
				}

				log.Warn("Removing proxy node", "proxy", proxy.String())

				ph.ps.removeProxy(proxyID)
				ph.p2pserver.RemovePeer(proxy.internalNode, p2p.ProxyPurpose)
			}

        // When any peer on the p2p level is connected
		case connectedPeer := <-ph.addProxyPeer:
			// Proxied peer just connected.
			// Set the corresponding proxyInfo's peer
			peerNode := connectedPeer.Node()
			peerID := peerNode.ID()
			proxy := ph.ps.getProxy(peerID)
            if proxy != nil {
				log.Warn("Connected proxy", "proxy", proxy.String())
                ph.ps.addProxyPeer(peerID, connectedPeer)
            }

        // When any peer on the p2p level is disconnected
		case disconnectedPeer := <-ph.delProxyPeer:
			peerID := disconnectedPeer.Node().ID()
			if ph.ps.getProxy(peerID) != nil {
				log.Warn("Disconnected proxy peer", "peerID", peerID)
                ph.ps.removeProxyPeer(peerID)
            }

		case request := <-ph.getValidatorProxyPeers:
			request.resultCh <- ph.ps.getValidatorProxyPeers(request.validators)

		case request := <-ph.getValidatorProxies:
			request.resultCh <- ph.ps.getValidatorProxies(request.validators)

		case resultCh := <-ph.getProxyInfo:
			resultCh <- ph.ps.getProxyInfo()

		case <-ph.newBlockchainEpoch:
			// New blockchain epoch. Update the validators in the proxySet
			ph.updateValidators()

		case <-phEpochTicker.C:
			log.Warn("PH Epoch ticker", "valAssignments", ph.ps.valAssigner.getValAssignments(), "proxiesByID", ph.ps.proxiesByID)


			// At every proxy handler epoch, do the following:

			// 1. Ensure that any proxy nodes that haven't been connected as peers
			//    in the duration of the epoch are not assigned to any validators.
			//    This can happen if the peer was previously connected, was assigned
			//    validators, but was later disconnected at the p2p level.
			ph.ps.unassignDisconnectedProxies(ph.proxyHandlerEpochLength)

			// 2. Send out a val enode share message to the proxies
			for _, proxy := range ph.ps.proxiesByID {
				if proxy.peer != nil {
					assignedValidators := ph.ps.getProxyValidators(proxy.ID())
					go ph.sb.sendValEnodesShareMsg(proxy.peer, proxy.externalNode, assignedValidators)
				}
			}



			//
			// // At every proxy handler epoch, do the following
			// // 1) Check for validator changes
			// // 2) Kick out proxy nodes that haven't connected within 60 seconds of when it was added.
			// // 3) Redistribute unassigned validators to ready peers or existing peers
			// // 4) Send out val_enode_share messages to the proxies
			// // 5) Do consistency checks with the proxy peers in proxy handler and proxy peers in the p2p server
			//
			// // 1) Check for validator changes
			// ph.updateValidators()
			//
			// // 2) Kick out proxy nodes that haven't connected within 60 seconds of when it was added.
			// // I think this might be a bad idea-- no step 3 and 4 do not wait for this to happen...
			// nonPeeredProxies := ph.ps.getNonPeeredProxyNodes(time.Minute)
			// if nonPeeredProxies != nil && len(nonPeeredProxies) > 0 {
			// 	log.Info("Non peered proxies", "proxies", nonPeeredProxies)
			// 	ph.removeProxies <- nonPeeredProxies
			// }
			//
			// // 3) Redistribute unassigned validators to ready peers and existing peers.
			// ph.ps.valAssigner.reassignValidators()
			//
			// 4) Send out val_enode_share messages to the proxies
			// for _, proxy := range ph.ps.proxiesByID {
			// 	if proxy.peer != nil {
			// 		assignedValidators := ph.ps.getProxyValidators(proxy.ID())
			// 		log.Warn("Sending a val_enode_share message", "proxy", proxy.String())
			// 		go ph.sb.sendValEnodesShareMsg(proxy.peer, proxy.externalNode, assignedValidators)
			// 	}
			// }

			// 5) Do consistency checks with the proxy peers in proxy handler and proxy peers in the p2p server
			// what happens if the proxy peer was dc'd and there are still validators assigned? do something here
		}
	}
}

func (ph *proxyHandler) updateValidators() error {
	newVals, rmVals, err := ph.checkForActiveRegValChanges(ph.ps.getValidators())
	log.Trace("Proxy Handler updating validators", "newVals", newVals, "rmVals", rmVals, "err", err)
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
