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
       "sync"
       "time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
 	"github.com/ethereum/go-ethereum/p2p/enode"
)


// This type defines a proxy
type proxy struct {
	internalNode *enode.Node    // Enode for the proxy's internal network interface
	externalNode *enode.Node    // Enode for the proxy's external network interface
	peer         consensus.Peer // Connected proxy peer.  Is nil if this node is not connected to the proxy
	addTimestamp        time.Time      // Timestamp when this proxy was added to the proxy set.  If it hasn't been connected within 60 seconds, then it's removed from the proxy set.
	index          int       // Index of proxy within proxyHeap.  This is used to help maintain the proxy heap's ordering.  This is -1 if it's not in the heap
}



// This type defines the set of proxies that the validator is aware of (communicated via the command line and/or the rpc api).
type proxySet struct {
     proxiesByID map[enode.ID]*proxy
     proxyHeap []*proxy          // list of the proxies.  This will be sorted on number of assigned validator per proxy
     getAssignedValidators func(enode.ID)[]common.Address     // interface to the validator assignments.  Used to retrieve the list of validators assigned for a proxy
}

func newProxySet(getAssignedValidators func(enode.ID)[]common.Address) *proxySet {
     newPS := &proxySet{proxiedByID: make(map[enode.ID]*proxyInfo), proxyPeersHeap: make(proxyHeap, 0), assignments: assignments}

     return newPS
}

// Heap related functions for the proxyHeap field.
func (ps *proxySet) Len() int  { return len(ps.proxyHeap) }

func (ps *proxySet) Less(i, j int) bool {
     // We want to sort by number of assigned validators
     numAssignedValsI := len(ps.getAssignedValidators(ps.proxyHeap[i].internalNode.ID()))
     numAssignedValsJ := len(ps.getAssignedValidators(ps.proxyHeap[j].internalNode.ID()))

     return numAssignedValsI < numAssignedValsJ
}

func (ps *proxySet) Swap(i, j int) {
     ps.proxyHeap[i], ps.proxyHeap[j] = ps.proxyHeap[j], ps.proxyHeap[i]
     ps.proxyHeap[i].index = i
     ps.proxyHeap[j].index = j
}

func (ps *proxySet) Push(x interface{}) {
     // Put the entry into the end of the heap
     n := len(ps.proxyHeap)
     proxy := x.(*Proxy)
     proxy.index = n
     ps.proxies = append(ps.proxies, proxy)
}

func (ps *proxySet) Pop() interface{} {
     // Pop the last entry of the heap
     heapLen := len(ps.proxyHeap)
     proxy := origHeap[heapLen-1]
     proxy.index = -1
     ps.proxyHeap = origHeap[0 : heapLen-1]
     return proxy
}
// End of heap related functions

func (ps *proxySet) addProxy(proxyInternalNode *enode.Node, proxyExternalNode *enode.Node) {
     if !ps.proxiedByID[proxyInternalNode.ID()] {
          ps.proxiedByID[proxyInternalNode.ID()] = &proxyInfo{node: proxyInternalNode, externalNode: proxyExternalNode, assignedValidators: make(map[common.Address]bool), addTime: time.Now(), index: -1}
     }
}

func (ps *proxySet) getProxy(proxyID enode.ID) *proxy {
     return ps.proxiedByID[proxyID]
}

func (ps *proxySet) removeProxy(proxyID enode.ID) {
     if ps.proxiesByID[proxyID] {
     	if ps.proxiesByID[proxyID].index != -1 {
	   heap.Remove(&ps.proxyPeersHeap, ps.proxiesByID[proxyID].index)
     	}

	delete(ps.proxiesByID[proxyID].assignedValidators)
	delete(ps.proxiesByID, proxyID)
     }
}

func (ps *proxySet) setPeer(proxyID enode.ID, peer consensus.Peer) {
     if ps.proxiesByID[proxyID] && ps.proxiedByID[proxyID].peer == nil {
     	ps.proxiesByID[proxyID].peer = connectedPeer

	heap.Push(&ps.proxyHeap, ps.proxiesByID[proxyID])
     }
}

func (ps *proxySet) proxyAssignmentChanged(proxyID enode.ID) {
     if ps.proxiesByID[ID].index != -1 {
     	heap.Fix(&ps, ps.proxiesByID[ID].index)
     }
}

func (ps *proxySet) hasConnectedProxies() bool {
     return len(ps.proxyPeersHeap) > 0
}



// This struct maintains the validators assignments to proxies
type valAssignments struct {
     valToProxy map[common.Address]*enode.ID             // map of validator address -> proxy ID.  If the proxy ID is nil, then the validator is unassigned
     proxyToVals map[enode.ID]map[common.Address]bool    // map of proxy ID to array of validator addresses
}

func (va *valAssignments) addValidators(vals []common.Addres) {
     for _, val := range(vals) {
     	 valToProxy[val] = nil
     }
}

func (va *valAssignemnts) removeValidator(valAddress common.Address) {
     va.disassociateValidator(valAddress)
     delete(va.valToProxy, valAddress)
}

func (va *valAssignments) getAssignedValidatorsForProxy(proxyID enode.ID) {
     return va.valToProxy[proxy.ID]
}

func (va *valAssignments) assignValidator(valAddress common.Address, proxyID enode.ID) {
     va.valToProxy[valAddress] = proxyID

     if _, ok := va.proxyToVals[proxyID]; !ok {
     	va.proxyToVals[proxyID] = make(map[common.Address]bool)
     }

     va.proxyToVals[proxyID][valAddress] = true
}

func (va *valAssignemnts) disassociateValidator(valAddress common.Address) {
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

	 loopWG        sync.WaitGroup
         quit chan struct{}

	 addProxies chan []*istanbul.ProxyNodes // This channel is for adding new proxies specified via command line or rpc api
	 removeProxies chan []*enode.Node // This channel is for removing proxies specified via rpc api

	 addProxyPeer chan consensus.Peer  // This channel is for newly peered proxies
	 delProxyPeer chan consensus.Peer  // This channel is for newly disconnected peers

	 newBlockchainEpoch  chan struct{}  // This channel is when a new blockchain epoch has started and we need to check if any validators are removed or added

	 proxyHandlerEpochLength time.Time
}

func (ph *proxyHandler) Start() error {
	ph.lock.Lock()
	defer ph.lock.Unlock()
	if ph.running {
		return errors.New("proxyHandler already running")
	}

	ph.running = true

	ph.quit = make(chan struct{})
	ph.addProxies = make(chan []*ProxyNode)
	ph.removeProxies = make(chan []*Node)
	ph.addProxyPeer = make(chan consensus.Peer)
	ph.delProxyPeer = make(chan consensus.Peer)

	ph.ProxyHandlerEpochLength = time.Minute

	ph.WG.Add(1)
	go ph.run()
	return nil
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

}

func (ph *proxyHandler) run() {
     defer ph.loopWG.Done()

     var (
         va *valAssignments = valAssignments{}

     	 // This data structure will store all the proxies.  It will need the proxyToValsAssignments map so that it can
	 // sort the proxies based on how many validators are assigned to it.
     	 ps *proxySet = newProxySet(proxyToValsAssignments)
     )

     phEpochTicker := time.NewTicker(ph.ProxyHandlerEpochLength)
     defer phEpochTicker.Stop()

     newVals, rmVals := ph.checkForValidatorChanges(valAssignments.getValidators())

running:
     for {
	select {
	case <-srv.quit:
	     // The proxyHnadler was stopped. Run the cleanup logic.
	     break running

	case addProxyNodes := <-ph.addProxies:
	     // Got command to add proxy nodes.  Add any unseen proxies to the proxies set and add p2p static connections to them.
	     for _, proxyNode := range addProxyNodes {
	         proxyID := proxyNode.InternalFacingNode.ID()
		 if !proxies[proxyID] {
	     	    ph.log.Info("Adding proxy node", "proxyNode", proxyNode)

		    ps.AddProxy(proxyNode.InternalFacingNode, proxyNode.ExternalFacingNode)
		    ph.p2pserver.addStatic(proxyNode.InternalFacingNode, p2p.ProxyPurpose)
		 }
	     }

	 case rmProxyNodes := <-ph.removeProxies:
	      // Got command to remove proxy nodes.  Disassociate the validators that were assigned to them, remove the proxies from the proxies set, and remove the p2p static connection
	      for _, proxyNode := range rmProxyNodes {
     	      	  proxyID := proxyNode.ID()
	  	  proxy := ps.getProxy(proxyID)

	  	  if proxy {
	     	     ph.log.Info("Removing proxy node", "proxyNode", proxyNode)

	     	     // Disassociate the assigned validators
	     	     for val := range proxies[proxyID].assignedValidators {
	     	     	 validators[val] = nil
	     	     }

	     	     ph.p2pserver.removeStatic(proxy.InternalFacingNode, p2p.ProxyPurpose)

		     ps.removePoxy(proxyID)
	      	  }
	      }

	 case connectedPeer := <-ph.addProxyPeer:
	      // Proxied peer just connected.  Set the corresponding proxyInfo's peer info and add this peer to the recentlyPeeredProxies set
	      peerNode := connectedPeer.Node()
	      peerID := peerNode.ID()

	      ps.SetProxyPeer(peerID, connectedPeer)

	 case disconnectedPeer := <-ph.delProxyPeer:
	      // Proxied peer just disconnected.  Remove the proxy
	      proxy := ps.getProxy(disconnectedPeer.Node().ID())
	      if proxy {
	      	 ph.removeProxies <- []*enode.Node{proxy.InternalNode}
	      }

	 case <-ph.newBlockchainEpoch:
	      // New epoch.  Need to see if any of the validators changed.
	      ph.checkForActiveRegValChanges(validators, ps)

	 case <-phEpochTicker.C:
	      // At every proxy handler epoch, do the following
	      // 1) Check for validator changes
	      // 2) Kick out proxy nodes that haven't connected within 60 seconds of when it was added.
	      // 3) Redistribute unassigned validators to ready peers or existing peers
	      // 4) Send out val_enode_share messages to the proxies
	      // 5) Do consistency checks with the proxy peers in proxy handler and proxy peers in the p2p server


	      // 1) Check for validator changes
	      ph.checkForActiveRegValChanges(validators, ps)

	      // 2) Kick out proxy nodes that haven't connected within 60 seconds of when it was added.
	      disconnectedProxies := ps.GetDisconnectedProxies(time.Minute)
	      if len(disconnectedProxies) > 0 {
	      	 ph.removeProxies <- disconnectedProxies
	      }

	      // 3) Redistribute unassigned validators to ready peers and existing peers.
	      if ps.hasConnectedProxies() {
	      	 for val, proxyID := range validators {
	      	     if proxyID == nil {
		     	validators[val] = ps.assignValidator(val)
		     }
		 }
	      }

	      // 4) Send out val_enode_share messages to the proxies
	      for _, proxy := range ps.getConnectedProxied() {
	      	  go sb.sendValEnodesShareMsg(proxy.peer, proxy.externalNode, proxy.assignedValidators)
	      }

	      // 5) Do consistency checks with the proxy peers in proxy handler and proxy peers in the p2p server
          }
     }
}


// This function will see if there are any changes in the ActiveAndRegisteredValidator set,
// compared to the `validators` parameter.
func (ph *proxyHandler) checkForActiveRegValChanges(validators map[common.Address]bool) {
     // Get the set of active and registered validators
     activeAndRegVals, err := ph.sb.retrieveActiveAndRegisteredValidators()
     if err != nil {
     	ph.log.Warn("Proxy Handler couldn't get the active and registered validators", "err", err)
     } else {
        newVals := activeAndRegVals
	rmVals := make(map[common.Address]*enode.ID)

	for existingVal, _ := range validators {
	    if newVals[existingVal] {
	       delete(newVals, existingVal)
	    }

	    if !newVals[existingVal] {
	       rmVals[existingVal] = validators[existingVal]
	    }
	}

	// Add new validators to the valiadator set
	for newVal := range newVals {
	    validators[newVal] = nil
	}

	// Remove validators from the validator set
	ps.removeValidators(rmVals)
	for rmVal := range rmVals {
	    delete(rmVal, validators)
	}
     }
}
