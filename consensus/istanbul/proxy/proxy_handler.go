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
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// This struct defines the handler that will manage all of the proxies and
// validator assignments to them
type proxyHandler struct {
	lock        sync.RWMutex // protects "runningFlag"
	runningFlag bool         // indicates if `run` is currently being run in a goroutine

	loopWG sync.WaitGroup
	quit   chan struct{}

	addProxies    chan []*istanbul.ProxyConfig // This channel is for adding new proxies specified via command line or rpc api
	removeProxies chan []*enode.Node           // This channel is for removing proxies specified via rpc api

	addProxyPeer    chan consensus.Peer // This channel is for newly peered proxies
	removeProxyPeer chan consensus.Peer // This channel is for newly disconnected peers

	proxyHandlerOpCh     chan proxyHandlerOpFunc
	proxyHandlerOpDoneCh chan struct{}

	sendValEnodeShareMsgsCh chan struct{} // This channel is to tell the proxy_handler to send a val_enode_share message to all the proxies

	newBlockchainEpoch chan struct{} // This channel is when a new blockchain epoch has started and we need to check if any validators are removed or added

	proxyHandlerEpochLength time.Duration // The duration of time between proxy handler epochs, which are occasional check-ins to ensure proxy/validator assignments are as intended

	sb     istanbul.BackendForProxy
	pe     ProxyEngine
	ps     *proxySet // Used to keep track of proxies & validators the proxies are associated with
	logger log.Logger
}

type proxyHandlerOpFunc func(ps *proxySet)

func newProxyHandler(sb istanbul.BackendForProxy, pe ProxyEngine) *proxyHandler {
	ph := &proxyHandler{
		sb:          sb,
		pe:          pe,
		runningFlag: false,

		quit:            make(chan struct{}),
		addProxies:      make(chan []*istanbul.ProxyConfig),
		removeProxies:   make(chan []*enode.Node),
		addProxyPeer:    make(chan consensus.Peer),
		removeProxyPeer: make(chan consensus.Peer),

		proxyHandlerOpCh:     make(chan proxyHandlerOpFunc),
		proxyHandlerOpDoneCh: make(chan struct{}),

		sendValEnodeShareMsgsCh: make(chan struct{}),
		newBlockchainEpoch:      make(chan struct{}),

		proxyHandlerEpochLength: time.Minute,

		ps: newProxySet(newConsistentHashingPolicy()),

		logger: log.New(),
	}

	return ph
}

// Start begins the proxyHandler event loop
func (ph *proxyHandler) Start() error {
	ph.lock.Lock()
	defer ph.lock.Unlock()

	if ph.runningFlag {
		return ErrStartedProxyHandler
	}
	ph.runningFlag = true

	ph.loopWG.Add(1)
	go ph.run()

	log.Info("Proxy handler started")
	return nil
}

// Stop stops the goroutine `run` if it is currently running
func (ph *proxyHandler) Stop() error {
	ph.lock.Lock()
	defer ph.lock.Unlock()

	if !ph.runningFlag {
		return ErrStoppedProxyHandler
	}
	ph.runningFlag = false
	ph.quit <- struct{}{}
	ph.loopWG.Wait()

	log.Info("Proxy handler stopped")
	return nil
}

// isRunning returns if `run` is currently running in a goroutine
func (ph *proxyHandler) running() bool {
	ph.lock.RLock()
	defer ph.lock.RUnlock()

	return ph.runningFlag
}

// run handles changes to proxies, validators, and performs occasional check-ins
// that proxy/validator assignments are as expected
func (ph *proxyHandler) run() {
	defer ph.loopWG.Done()

	phEpochTicker := time.NewTicker(ph.proxyHandlerEpochLength)
	defer phEpochTicker.Stop()

	logger := ph.logger.New("func", "run")

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
				log.Info("Adding proxy node", "proxyNode", proxyNode, "proxyID", proxyID)
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

				logger.Info("Removing proxy node", "proxy", proxy.String(), "chan", "removeProxies")

				ph.pe.SendValEnodesShareMsg(proxy.peer, []common.Address{})

				if valsReassigned := ph.ps.removeProxy(proxyID); valsReassigned {
					logger.Info("Remote validator to proxy assignment has changed.  Sending val enode share messages and updating announce version")
					ph.sendValEnodeShareMsgs()
					ph.sb.UpdateAnnounceVersion()
				}
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
				if valsReassigned := ph.ps.setProxyPeer(peerID, connectedPeer); valsReassigned {
					logger.Info("Remote validator to proxy assignment has changed.  Sending val enode share messages and updating announce version")
					ph.sendValEnodeShareMsgs()
					ph.sb.UpdateAnnounceVersion()
				}

				// Share this node's enodeCertificate for the proxy to use for handshakes via a forward message
				proxyEnodeCertificateMsg, err := ph.sb.GenerateEnodeCertificateMsg(proxy.externalNode.URLv4())
				if err != nil {
					logger.Warn("Error generating enode certificate message", "err", err)
				} else {
					payload, err := proxyEnodeCertificateMsg.Payload()
					if err != nil {
						logger.Error("Error getting payload of enode certificate message", "err", err)
					} else {
						validatorAssignments := ph.ps.getValidatorAssignments(nil, []enode.ID{peerID})

						destAddresses := make([]common.Address, 0, len(validatorAssignments))
						for valAddress := range validatorAssignments {
							destAddresses = append(destAddresses, valAddress)
						}

						// The forward message is being sent within a goroutine, since SendForwardMsg will query the proxy handler thread
						// (which we are already in).  If this command is done inline, then it will result in a deadlock.
						// TODO: Figure out a way to run this command inline.
						go func() {
							if err := ph.pe.SendForwardMsg(destAddresses, istanbul.EnodeCertificateMsg, nil, map[enode.ID][]byte{peerID: payload}); err != nil {
								logger.Error("Error in forwarding a enodeCertificateMsg to proxy", "proxy", peerNode, "error", err)
							}
						}()
					}
				}
			}

		case disconnectedPeer := <-ph.removeProxyPeer:
			// Proxied peer just disconnected.
			peerID := disconnectedPeer.Node().ID()
			if ph.ps.getProxy(peerID) != nil {
				logger.Debug("Disconnected proxy peer", "peerID", peerID, "chan", "removeProxyPeer")
				ph.ps.removeProxyPeer(peerID)
			}

		case proxyHandlerOp := <-ph.proxyHandlerOpCh:
			proxyHandlerOp(ph.ps)
			ph.proxyHandlerOpDoneCh <- struct{}{}

		case <-ph.newBlockchainEpoch:
			// New blockchain epoch. Update the validators in the proxySet
			valsReassigned, error := ph.updateValidators()
			if error != nil {
				logger.Warn("Error in updating validator assignments on new epoch", "error", error)
			}
			if valsReassigned {
				ph.sendValEnodeShareMsgs()
				ph.sb.UpdateAnnounceVersion()
			}

		case <-phEpochTicker.C:
			// At every proxy handler epoch, do the following:

			// 1. Ensure that any proxy nodes that haven't been connected as peers
			//    in the duration of the epoch are not assigned to any validators.
			//    This can happen if the peer was previously connected, was assigned
			//    validators, but was later disconnected at the p2p level.
			if valsReassigned := ph.ps.unassignDisconnectedProxies(ph.proxyHandlerEpochLength); valsReassigned {
				ph.sb.UpdateAnnounceVersion()
			}

			// 2. Send out a val enode share message to the proxies. Do this regardless if validators got reassigned,
			//    just to ensure that proxies will get the val enode table
			ph.sendValEnodeShareMsgs()

			// 3. TODO - Do consistency checks with the proxy peers in proxy handler and proxy peers in the p2p server

		case <-ph.sendValEnodeShareMsgsCh:
			ph.sendValEnodeShareMsgs()
		}
	}
}

func (ph *proxyHandler) sendValEnodeShareMsgs() {
	logger := ph.logger.New("func", "sendValEnodeShareMsgs")

	for _, proxy := range ph.ps.proxiesByID {
		if proxy.peer != nil {
			assignedValidators := ph.ps.getValidatorAssignments(nil, []enode.ID{proxy.ID()})
			valAddresses := make([]common.Address, 0, len(assignedValidators))
			for valAddress := range assignedValidators {
				valAddresses = append(valAddresses, valAddress)
			}
			logger.Info("Sending val enode share msg to proxy", "proxy peer", proxy.peer, "valAddresses", common.ConvertToStringSlice(valAddresses))
			go ph.pe.SendValEnodesShareMsg(proxy.peer, valAddresses)
		}
	}
}

func (ph *proxyHandler) updateValidators() (bool, error) {
	newVals, rmVals, err := ph.getValidatorConnSetDiff(ph.ps.getValidators())
	log.Trace("Proxy Handler updating validators", "newVals", common.ConvertToStringSlice(newVals), "rmVals", common.ConvertToStringSlice(rmVals), "err", err, "func", "updateValiadtors")
	if err != nil {
		return false, err
	}

	valsReassigned := false
	if len(newVals) > 0 {
		valsReassigned = ph.ps.addRemoteValidators(newVals)
	}

	if len(rmVals) > 0 {
		valsReassigned = ph.ps.removeRemoteValidators(rmVals)
	}

	return valsReassigned, nil
}

// This function will return a diff between the current Validator Connection set and the `validators` parameter.
func (ph *proxyHandler) getValidatorConnSetDiff(validators []common.Address) (newVals []common.Address, rmVals []common.Address, err error) {
	logger := ph.logger.New("func", "getValidatorConnSetDiff")

	logger.Trace("Proxy handler retrieving validator connection set diff", "validators", common.ConvertToStringSlice(validators))

	// Get the set of active and registered validators
	newValConnSet, err := ph.sb.RetrieveValidatorConnSet(false)
	if err != nil {
		logger.Warn("Proxy Handler couldn't get the validator connection set", "err", err, "func", "getValidatorConnSetDiff")
		return nil, nil, err
	}

	// Don't add this validator's address to the returned new validator set
	delete(newValConnSet, ph.sb.Address())

	outputNewValConnSet := make([]common.Address, 0, len(newValConnSet))
	for newVal := range newValConnSet {
		outputNewValConnSet = append(outputNewValConnSet, newVal)
	}
	logger.Trace("retrieved validator connset", "valConnSet", common.ConvertToStringSlice(outputNewValConnSet))

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
	newVals = make([]common.Address, 0, len(newValConnSet))
	for newVal := range newValConnSet {
		newVals = append(newVals, newVal)
	}

	logger.Trace("returned diff", "newVals", common.ConvertToStringSlice(newVals), "rmVals", common.ConvertToStringSlice(rmVals))

	return newVals, rmVals, nil
}

// This function will return the assigned proxies for the given remote validators.
// If the "validators" parameter is nil, then this function will return all of the validator assignments.
func (ph *proxyHandler) getValidatorAssignments(validators []common.Address) (map[common.Address]*proxy, error) {
	assignedProxies := make(map[common.Address]*proxy)

	select {
	case ph.proxyHandlerOpCh <- func(ps *proxySet) {
		valAssignments := ps.getValidatorAssignments(validators, nil)

		for address, proxy := range valAssignments {
			assignedProxies[address] = proxy
		}
	}:
		<-ph.proxyHandlerOpDoneCh

	case <-ph.quit:
		return nil, ErrStoppedProxyHandler

	}

	return assignedProxies, nil
}
