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

package proxy

import (
	"errors"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// This struct is used for requesting proxy peers for particular validators
// from the peerHandler event loop
type validatorProxyPeersRequest struct {
	validators []common.Address
	resultCh   chan map[enode.ID]consensus.Peer
}

// This struct is used for requesting proxies for particular validators
// from the peerHandler event loop
type validatorProxiesRequest struct {
	validators map[common.Address]bool
	resultCh   chan map[common.Address]*proxy
}

// This struct is used for requesting the peer of a proxy with ID peerID
type proxyPeerRequest struct {
	peerID   enode.ID
	resultCh chan consensus.Peer
}

// This struct defines the handler that will manage all of the proxies and
// validator assignments to them
type proxyHandler struct {
	lock    sync.Mutex // protects "running" and "p2pserver"
	running bool       // indicates if `run` is currently being run in a goroutine

	loopWG sync.WaitGroup
	quit   chan struct{}

	addProxies    chan []*istanbul.ProxyConfig // This channel is for adding new proxies specified via command line or rpc api
	removeProxies chan []*enode.Node           // This channel is for removing proxies specified via rpc api

	addProxyPeer chan consensus.Peer // This channel is for newly peered proxies
	delProxyPeer chan consensus.Peer // This channel is for newly disconnected peers

	getValidatorProxyPeers chan *validatorProxyPeersRequest // This channel is for getting the peers of proxies for a set of validators
	getValidatorProxies    chan *validatorProxiesRequest    // This channel is for getting the proxies for a set of validators
	getProxyPeer           chan *proxyPeerRequest           // This channel is for getting the peers of a proxy with a specific ID

	getProxyInfo chan chan []ProxyInfo // This channel is for getting info on the proxies in proxySet

	newBlockchainEpoch chan struct{} // This channel is when a new blockchain epoch has started and we need to check if any validators are removed or added

	proxyHandlerEpochLength time.Duration // The duration of time between proxy handler epochs, which are occasional check-ins to ensure proxy/validator assignments are as intended

	sb istanbul.Backend
	pe ProxyEngine
	ps *proxySet // Used to keep track of proxies & validators the proxies are associated with
}

func newProxyHandler(sb istanbul.Backend, pe ProxyEngine) *proxyHandler {
	ph := &proxyHandler{
		sb: sb,
		pe: pe,
	}

	ph.running = false

	ph.quit = make(chan struct{})
	ph.addProxies = make(chan []*istanbul.ProxyConfig)
	ph.removeProxies = make(chan []*enode.Node)
	ph.addProxyPeer = make(chan consensus.Peer)
	ph.delProxyPeer = make(chan consensus.Peer)

	ph.getValidatorProxyPeers = make(chan *validatorProxyPeersRequest)
	ph.getValidatorProxies = make(chan *validatorProxiesRequest)
	ph.getProxyPeer = make(chan *proxyPeerRequest)

	ph.getProxyInfo = make(chan chan []ProxyInfo)

	ph.newBlockchainEpoch = make(chan struct{})

	ph.proxyHandlerEpochLength = time.Minute

	ph.ps = newProxySet(newConsistentHashingPolicy())

	return ph
}

// Start begins the proxyHandler event loop
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

// isRunning returns if `run` is currently running in a goroutine
func (ph *proxyHandler) isRunning() bool {
	ph.lock.Lock()
	defer ph.lock.Unlock()

	return ph.running
}

// run handles changes to proxies, validators, and performs occasional check-ins
// that proxy/validator assignments are as expected
func (ph *proxyHandler) run() {
	defer ph.loopWG.Done()

	phEpochTicker := time.NewTicker(ph.proxyHandlerEpochLength)
	defer phEpochTicker.Stop()

	logger := log.New("func", "run")

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
				proxyID := proxyNode.InternalNode.ID()
				if ph.ps.getProxy(proxyID) != nil {
					logger.Debug("Proxy is already in the proxy set", "proxyNode", proxyNode, "proxyID", proxyID, "chan", "addProxies")
					continue
				}
				log.Debug("Adding proxy node", "proxyNode", proxyNode, "proxyID", proxyID)
				ph.ps.addProxy(proxyNode)
				ph.sb.AddPeer(proxyNode.InternalNode, p2p.ProxyPurpose)
			}

		case rmProxyNodes := <-ph.removeProxies:
			// Got command to remove proxy nodes.
			// Remove the proxy and remove the p2p static connection
			for _, proxyNode := range rmProxyNodes {
				proxyID := proxyNode.ID()
				proxy := ph.ps.getProxy(proxyID)
				if proxy == nil {
					logger.Warn("Proxy is not in the proxy set", "proxy", proxyNode, "proxyID", proxyID, "chan", "removeProxies")
					continue
				}

				logger.Debug("Removing proxy node", "proxy", proxy.String(), "chan", "removeProxies")

				ph.ps.removeProxy(proxyID)
				// This will most likely result in validator reassignments, send
				// val enode share messages
				ph.sendValEnodeShareMsgs()
				ph.sb.RemovePeer(proxy.node, p2p.ProxyPurpose)
			}

		case connectedPeer := <-ph.addProxyPeer:
			// Proxied peer just connected.
			// Set the corresponding proxyInfo's peer
			peerNode := connectedPeer.Node()
			peerID := peerNode.ID()
			proxy := ph.ps.getProxy(peerID)
			if proxy != nil {
				logger.Debug("Connected proxy", "proxy", proxy.String(), "chan", "addProxyPeer")
				ph.ps.setProxyPeer(peerID, connectedPeer)
				// This may result in validator reassignments, send
				// val enode share messages regardless
				ph.sendValEnodeShareMsgs()
			}

		case disconnectedPeer := <-ph.delProxyPeer:
			// Proxied peer just disconnected.
			peerID := disconnectedPeer.Node().ID()
			if ph.ps.getProxy(peerID) != nil {
				logger.Debug("Disconnected proxy peer", "peerID", peerID, "chan", "delProxyPeer")
				ph.ps.removeProxyPeer(peerID)
			}

		case request := <-ph.getValidatorProxyPeers:
			request.resultCh <- ph.ps.getValidatorProxyPeers(request.validators)

		case request := <-ph.getValidatorProxies:
			request.resultCh <- ph.ps.getValidatorProxies(request.validators)

		case request := <-ph.getProxyPeer:
			proxy := ph.ps.getProxy(request.peerID)
			if proxy == nil {
				request.resultCh <- nil
			} else {
				request.resultCh <- proxy.peer
			}

		case resultCh := <-ph.getProxyInfo:
			resultCh <- ph.ps.getProxyInfo()

		case <-ph.newBlockchainEpoch:
			// New blockchain epoch. Update the validators in the proxySet
			ph.updateValidators()
			ph.sendValEnodeShareMsgs()

		case <-phEpochTicker.C:
			// At every proxy handler epoch, do the following:

			// 1. Ensure that any proxy nodes that haven't been connected as peers
			//    in the duration of the epoch are not assigned to any validators.
			//    This can happen if the peer was previously connected, was assigned
			//    validators, but was later disconnected at the p2p level.
			ph.ps.unassignDisconnectedProxies(ph.proxyHandlerEpochLength)

			// 2. Send out a val enode share message to the proxies
			ph.sendValEnodeShareMsgs()

			// 3. TODO - Do consistency checks with the proxy peers in proxy handler and proxy peers in the p2p server
		}
	}
}

func (ph *proxyHandler) sendValEnodeShareMsgs() {
	for _, proxy := range ph.ps.proxiesByID {
		if proxy.peer != nil {
			assignedValidators := ph.ps.getProxyValidators(proxy.ID())
			go ph.pe.SendValEnodesShareMsg(proxy.peer, proxy.externalNode, assignedValidators)
		}
	}
}

func (ph *proxyHandler) updateValidators() error {
	newVals, rmVals, err := ph.getValidatorConnSetDiff(ph.ps.getValidators())
	log.Trace("Proxy Handler updating validators", "newVals", newVals, "rmVals", rmVals, "err", err, "func", "updateValiadtors")
	if err != nil {
		return err
	}
	ph.ps.addRemoteValidators(newVals)
	ph.ps.removeRemoteValidators(rmVals)
	return nil
}

// This function will return a diff between the current Validator Connection set and the `validators` parameter.
func (ph *proxyHandler) getValidatorConnSetDiff(validators []common.Address) (newVals []common.Address, rmVals []common.Address, err error) {
	// Get the set of active and registered validators
	newValConnSet, err := ph.sb.RetrieveValidatorConnSet(false)
	if err != nil {
		log.Warn("Proxy Handler couldn't get the validator connection set", "err", err, "func", "getValidatorConnSetDiff")
		return nil, nil, err
	}

	rmVals = make([]common.Address, 0) // There is a good chance that there will be no diff, so set size to 0

	// First find all old val entries that are no in the newValConnSet (which will be the removed validator set),
	// and find all the same val entries and remove them from the newValConnSet.
	for _, oldVal := range validators {
		if !newValConnSet[oldVal] {
			rmVals = append(rmVals, oldVal)
		} else {
			delete(newValConnSet, oldVal)
		}
	}

	// Whatever is remaining in the newValConnSet is the new validator set.
	newVals = make([]common.Address, len(newValConnSet))
	for newVal := range newValConnSet {
		newVals = append(newVals, newVal)
	}

	return newVals, rmVals, nil
}
