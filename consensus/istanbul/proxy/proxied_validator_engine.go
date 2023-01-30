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
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

// BackendForProxiedValidatorEngine provides the Istanbul backend application specific functions for Istanbul proxied validator engine
type BackendForProxiedValidatorEngine interface {
	// Address returns the validator's signing address
	Address() common.Address

	// IsValidating returns true if this node is currently validating
	IsValidating() bool

	// IsProxiedValidator returns true if this node is a proxied validator
	IsProxiedValidator() bool

	// SelfNode returns the owner's node (if this is a proxy, it will return the external node)
	SelfNode() *enode.Node

	// Sign signs input data with the validator's ecdsa signing key
	Sign([]byte) ([]byte, error)

	// Multicast sends a message to it's connected nodes filtered on the 'addresses' parameter (where each address
	// is associated with those node's signing key)
	// If sendToSelf is set to true, then the function will send an event to self via a message event
	Multicast(addresses []common.Address, msg *istanbul.Message, ethMsgCode uint64, sendToSelf bool) error

	// Unicast will asynchronously send a celo message to peer
	Unicast(peer consensus.Peer, payload []byte, ethMsgCode uint64)

	// GetValEnodeTableEntries retrieves the entries in the valEnodeTable filtered on the "validators" parameter.
	// If the parameter is nil, then no filter will be applied.
	GetValEnodeTableEntries(validators []common.Address) (map[common.Address]*istanbul.AddressEntry, error)

	// UpdateAnnounceVersion will notify the announce protocol that this validator's valEnodeTable entry has been updated
	UpdateAnnounceVersion()

	// GetAnnounceVersion will retrieve the current node's announce version
	GetAnnounceVersion() uint

	// RetrieveEnodeCertificateMsgMap will retrieve this node's handshake enodeCertificate
	RetrieveEnodeCertificateMsgMap() map[enode.ID]*istanbul.EnodeCertMsg

	// RetrieveValidatorConnSet will retrieve the validator connection set.
	RetrieveValidatorConnSet() (map[common.Address]bool, error)

	// AddPeer will add a static peer
	AddPeer(node *enode.Node, purpose p2p.PurposeFlag)

	// RemovePeer will remove a static peer
	RemovePeer(node *enode.Node, purpose p2p.PurposeFlag)

	// GetProxiedValidatorEngine returns the proxied validator engine created for this Backend.  This should only be used for the unit tests.
	GetProxiedValidatorEngine() ProxiedValidatorEngine
}

type fwdMsgInfo struct {
	destAddresses []common.Address
	ethMsgCode    uint64
	payload       []byte
}

type proxiedValidatorEngine struct {
	config  *istanbul.Config
	logger  log.Logger
	backend BackendForProxiedValidatorEngine

	isRunning   bool
	isRunningMu sync.RWMutex

	loopWG sync.WaitGroup //

	quit chan struct{} // Used to notify to the thread to quit

	addProxies    chan []*istanbul.ProxyConfig // Used to notify to the thread that new proxies have been added via command line or rpc api
	removeProxies chan []*enode.Node           // Used to notify to the thread that proxies have been removed via rpc api

	addProxyPeer    chan consensus.Peer // Used to notify to the thread of newly peered proxies
	removeProxyPeer chan consensus.Peer // Used to notify to the thread of newly disconnected proxy peers

	proxiedValThreadOpCh     chan proxiedValThreadOpFunc // Used to submit operations to be executed on the thread's local state
	proxiedValThreadOpDoneCh chan struct{}

	sendValEnodeShareMsgsCh chan struct{} // Used to notify the thread to send a val enode share message to all of the proxies

	sendEnodeCertsCh chan map[enode.ID]*istanbul.EnodeCertMsg // Used to notify the thread to send the enode certs to the appropriate proxy.

	sendFwdMsgsCh chan *fwdMsgInfo // Used to send a forward message to all of the proxies

	newBlockchainEpoch chan struct{} // Used to notify to the thread that a new blockchain epoch has started
}

// proxiedValThreadOpFunc is a function type to define operations executed with run's local state as parameters.
type proxiedValThreadOpFunc func(ps *proxySet)

// New creates a new proxied validator engine.
func NewProxiedValidatorEngine(backend BackendForProxiedValidatorEngine, config *istanbul.Config) (ProxiedValidatorEngine, error) {
	if !backend.IsProxiedValidator() {
		return nil, ErrNodeNotProxiedValidator
	}

	pv := &proxiedValidatorEngine{
		config:  config,
		logger:  log.New(),
		backend: backend,

		addProxies:      make(chan []*istanbul.ProxyConfig),
		removeProxies:   make(chan []*enode.Node),
		addProxyPeer:    make(chan consensus.Peer, 10),
		removeProxyPeer: make(chan consensus.Peer, 10),

		proxiedValThreadOpCh:     make(chan proxiedValThreadOpFunc),
		proxiedValThreadOpDoneCh: make(chan struct{}),

		sendValEnodeShareMsgsCh: make(chan struct{}),
		sendEnodeCertsCh:        make(chan map[enode.ID]*istanbul.EnodeCertMsg),
		sendFwdMsgsCh:           make(chan *fwdMsgInfo),
		newBlockchainEpoch:      make(chan struct{}),
	}

	return pv, nil
}

func (pv *proxiedValidatorEngine) Start() error {
	pv.isRunningMu.Lock()
	defer pv.isRunningMu.Unlock()

	if pv.isRunning {
		return istanbul.ErrStartedProxiedValidatorEngine
	}

	pv.loopWG.Add(1)
	pv.quit = make(chan struct{})
	go pv.threadRun()

	if len(pv.config.ProxyConfigs) > 0 {
		select {
		case pv.addProxies <- pv.config.ProxyConfigs:
		case <-pv.quit:
			return istanbul.ErrStartedProxiedValidatorEngine
		}
	}

	pv.isRunning = true
	pv.logger.Info("Proxied validator engine started")
	return nil
}

func (pv *proxiedValidatorEngine) Stop() error {
	pv.isRunningMu.Lock()
	defer pv.isRunningMu.Unlock()

	if !pv.isRunning {
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	close(pv.quit)
	pv.loopWG.Wait()

	pv.isRunning = false
	pv.logger.Info("Proxy engine stopped")
	return nil
}

// Running will return true if the proxied validator engine in runnning, and false otherwise.
func (pv *proxiedValidatorEngine) Running() bool {
	pv.isRunningMu.RLock()
	defer pv.isRunningMu.RUnlock()

	return pv.isRunning
}

// AddProxy will add a proxy config, connect to it's internal enodeURL, and assign it remote validators.
func (pv *proxiedValidatorEngine) AddProxy(node, externalNode *enode.Node) error {
	if !pv.Running() {
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	select {
	case pv.addProxies <- []*istanbul.ProxyConfig{{InternalNode: node, ExternalNode: externalNode}}:
		return nil
	case <-pv.quit:
		return istanbul.ErrStoppedProxiedValidatorEngine
	}
}

// RemoveProxy will remove a proxy, disconnect from it, and reassign remote validators that were originally assigned to them.
func (pv *proxiedValidatorEngine) RemoveProxy(node *enode.Node) error {
	if !pv.Running() {
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	select {
	case pv.removeProxies <- []*enode.Node{node}:
		return nil
	case <-pv.quit:
		return istanbul.ErrStoppedProxiedValidatorEngine
	}
}

func (pv *proxiedValidatorEngine) RegisterProxyPeer(proxyPeer consensus.Peer) error {
	if !pv.Running() {
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	logger := pv.logger.New("func", "RegisterProxyPeer")
	if proxyPeer.PurposeIsSet(p2p.ProxyPurpose) {
		logger.Info("Got new proxy peer", "proxyPeer", proxyPeer)
		select {
		case pv.addProxyPeer <- proxyPeer:
		case <-pv.quit:
			return istanbul.ErrStoppedProxiedValidatorEngine
		}
	} else {
		logger.Error("Unauthorized connected peer to the proxied validator", "peerID", proxyPeer.Node().ID())
		return errUnauthorizedProxiedValidator
	}

	return nil
}

func (pv *proxiedValidatorEngine) UnregisterProxyPeer(proxyPeer consensus.Peer) error {
	if !pv.Running() {
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	if proxyPeer.PurposeIsSet(p2p.ProxyPurpose) {
		select {
		case pv.removeProxyPeer <- proxyPeer:
		case <-pv.quit:
			return istanbul.ErrStoppedProxiedValidatorEngine
		}
	}

	return nil
}

// This function will return the remote validator to proxy assignments for the given remote validators.
// If the "validators" parameter is nil, then this function will return all of the validator assignments.
func (pv *proxiedValidatorEngine) GetValidatorProxyAssignments(validators []common.Address) (map[common.Address]*Proxy, error) {
	if !pv.Running() {
		return nil, istanbul.ErrStoppedProxiedValidatorEngine
	}

	valAssignments := make(map[common.Address]*Proxy)

	select {
	case pv.proxiedValThreadOpCh <- func(ps *proxySet) {
		for address, proxy := range ps.getValidatorAssignments(validators, nil) {
			valAssignments[address] = proxy
		}
	}:
		<-pv.proxiedValThreadOpDoneCh

	case <-pv.quit:
		return nil, istanbul.ErrStoppedProxiedValidatorEngine

	}

	return valAssignments, nil
}

// This function will return all of the proxies and all the proxy to validator assignments.
func (pv *proxiedValidatorEngine) GetProxiesAndValAssignments() ([]*Proxy, map[enode.ID][]common.Address, error) {
	var proxies []*Proxy
	var valAssignments map[enode.ID][]common.Address

	if !pv.Running() {
		return nil, nil, istanbul.ErrStoppedProxiedValidatorEngine
	}

	select {
	case pv.proxiedValThreadOpCh <- func(ps *proxySet) {
		proxies, valAssignments = ps.getProxyAndValAssignments()
	}:
		<-pv.proxiedValThreadOpDoneCh

	case <-pv.quit:
		return nil, nil, istanbul.ErrStoppedProxiedValidatorEngine

	}

	return proxies, valAssignments, nil
}

// SendValEnodeShareMsgs will signal to the running thread to send a val enode share message to all of the proxies
func (pv *proxiedValidatorEngine) SendValEnodesShareMsgToAllProxies() error {
	if !pv.Running() {
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	select {
	case pv.sendValEnodeShareMsgsCh <- struct{}{}:

	case <-pv.quit:
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	return nil
}

// SendEnodeCertsToAllProxies will signal to the running thread to share the given enode certs to the appropriate proxy
func (pv *proxiedValidatorEngine) SendEnodeCertsToAllProxies(enodeCerts map[enode.ID]*istanbul.EnodeCertMsg) error {
	if !pv.Running() {
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	select {
	case pv.sendEnodeCertsCh <- enodeCerts:

	case <-pv.quit:
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	return nil
}

// SendForwardMsgToAllProxies will signal to the running thread to send a forward message to all proxies.
func (pv *proxiedValidatorEngine) SendForwardMsgToAllProxies(finalDestAddresses []common.Address, ethMsgCode uint64, payload []byte) error {
	if !pv.Running() {
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	select {
	case pv.sendFwdMsgsCh <- &fwdMsgInfo{destAddresses: finalDestAddresses, ethMsgCode: ethMsgCode, payload: payload}:

	case <-pv.quit:
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	return nil
}

// NewEpoch will notify the proxied validator's thread that a new epoch started
func (pv *proxiedValidatorEngine) NewEpoch() error {
	if !pv.Running() {
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	select {
	case pv.newBlockchainEpoch <- struct{}{}:

	case <-pv.quit:
		return istanbul.ErrStoppedProxiedValidatorEngine
	}

	return nil
}

// run handles changes to proxies and validator assignments
func (pv *proxiedValidatorEngine) threadRun() {
	var (
		// The minimum allowable time that a proxy can be disconnected from the proxied validator
		// After this expires, the proxy handler will remove any validator assignments from the proxy.
		minProxyDisconnectTime time.Duration = 30 * time.Second

		// The duration of time between thread update, which are occasional check-ins to ensure proxy/validator assignments are as intended
		schedulerPeriod time.Duration = 30 * time.Second

		// Used to keep track of proxies & validators the proxies are associated with
		ps *proxySet = newProxySet(newConsistentHashingPolicy())
	)

	logger := pv.logger.New("func", "threadRun")

	defer pv.loopWG.Done()

	schedulerTicker := time.NewTicker(schedulerPeriod)
	defer schedulerTicker.Stop()

	pv.updateValidatorAssignments(ps)

loop:
	for {
		select {
		case <-pv.quit:
			// The proxied validator engine was stopped
			break loop

		case addProxyNodes := <-pv.addProxies:
			// Got command to add proxy nodes.
			// Add any unseen proxies to the proxy set and add p2p static connections to them.
			for _, proxyNode := range addProxyNodes {
				proxyID := proxyNode.InternalNode.ID()
				if ps.getProxy(proxyID) != nil {
					logger.Debug("Proxy is already in the proxy set", "proxyNode", proxyNode, "proxyID", proxyID, "chan", "addProxies")
					continue
				}
				log.Info("Adding proxy node", "proxyNode", proxyNode, "proxyID", proxyID)
				ps.addProxy(proxyNode)
				pv.backend.AddPeer(proxyNode.InternalNode, p2p.ProxyPurpose)
			}

		case rmProxyNodes := <-pv.removeProxies:
			// Got command to remove proxy nodes.
			// Remove the proxy and remove the p2p static connection
			for _, proxyNode := range rmProxyNodes {
				proxyID := proxyNode.ID()
				proxy := ps.getProxy(proxyID)
				if proxy == nil {
					logger.Warn("Proxy is not in the proxy set", "proxy", proxyNode, "proxyID", proxyID, "chan", "removeProxies")
					continue
				}

				logger.Info("Removing proxy node", "proxy", proxy.String(), "chan", "removeProxies")

				// If the removed proxy is connected, instruct it to disconnect any of it's validator connections
				// by sending a val enode share message with an empty set.
				if proxy.peer != nil {
					pv.sendValEnodesShareMsg(proxy.peer, []common.Address{})
				}

				if valsReassigned := ps.removeProxy(proxyID); valsReassigned {
					logger.Info("Remote validator to proxy assignment has changed.  Sending val enode share messages and updating announce version")
					pv.backend.UpdateAnnounceVersion()
					pv.sendValEnodeShareMsgs(ps)
				}
				pv.backend.RemovePeer(proxy.node, p2p.ProxyPurpose)
			}

		case connectedPeer := <-pv.addProxyPeer:
			// Proxied peer just connected.
			// Set the corresponding proxyInfo's peer
			peerNode := connectedPeer.Node()
			peerID := peerNode.ID()
			proxy := ps.getProxy(peerID)
			if proxy != nil {
				logger.Debug("Connected proxy", "proxy", proxy.String(), "chan", "addProxyPeer")
				if valsReassigned := ps.setProxyPeer(peerID, connectedPeer); valsReassigned {
					logger.Info("Remote validator to proxy assignment has changed.  Sending val enode share messages and updating announce version")
					pv.backend.UpdateAnnounceVersion()
					pv.sendValEnodeShareMsgs(ps)
				}
			}

		case disconnectedPeer := <-pv.removeProxyPeer:
			// Proxied peer just disconnected.
			peerID := disconnectedPeer.Node().ID()
			if ps.getProxy(peerID) != nil {
				logger.Debug("Disconnected proxy peer", "peerID", peerID, "chan", "removeProxyPeer")
				ps.removeProxyPeer(peerID)
			}

		case proxyHandlerOp := <-pv.proxiedValThreadOpCh:
			proxyHandlerOp(ps)
			pv.proxiedValThreadOpDoneCh <- struct{}{}

		case <-pv.newBlockchainEpoch:
			// New blockchain epoch. Update the validators in the proxySet
			valsReassigned, error := pv.updateValidatorAssignments(ps)
			if error != nil {
				logger.Warn("Error in updating validator assignments on new epoch", "error", error)
			}
			if valsReassigned {
				pv.backend.UpdateAnnounceVersion()
				pv.sendValEnodeShareMsgs(ps)
			}

		case <-pv.sendValEnodeShareMsgsCh:
			pv.sendValEnodeShareMsgs(ps)

		case enodeCerts := <-pv.sendEnodeCertsCh:
			pv.sendEnodeCerts(ps, enodeCerts)

		case fwdMsg := <-pv.sendFwdMsgsCh:
			pv.sendForwardMsg(ps, fwdMsg.destAddresses, fwdMsg.ethMsgCode, fwdMsg.payload)

		case <-schedulerTicker.C:
			logger.Trace("schedulerTicker ticked")

			// Remove validator assignement for proxies that are disconnected for a minimum of `minProxyDisconnectTime` seconds.
			// The reason for not immediately removing the validator asssignments is so that if there is a
			// network disconnect then a quick reconnect, the validator assignments wouldn't be changed.
			// If no reassignments were made, then resend all enode certificates and val enode share messages to the
			// proxies, in case previous attempts failed.
			if valsReassigned := ps.unassignDisconnectedProxies(minProxyDisconnectTime); valsReassigned {
				pv.backend.UpdateAnnounceVersion()
				pv.sendValEnodeShareMsgs(ps)
			} else {
				// Send out the val enode share message.  We will resend the valenodeshare message here in case it was
				// never successfully sent before.
				pv.sendValEnodeShareMsgs(ps)

				// Also resend the enode certificates to the proxies (via a forward message), in case it was
				// never successfully sent before.

				// Get all the enode certificate messages
				proxyEnodeCertMsgs := pv.backend.RetrieveEnodeCertificateMsgMap()

				// Share the enode certs with the proxies
				pv.sendEnodeCerts(ps, proxyEnodeCertMsgs)
			}
		}
	}
}

// sendValEnodeShareMsgs sends a ValEnodeShare Message to each proxy to update the proxie's validator enode table.
// This is a no-op for replica validators.
func (pv *proxiedValidatorEngine) sendValEnodeShareMsgs(ps *proxySet) {
	logger := pv.logger.New("func", "sendValEnodeShareMsgs")

	for _, proxy := range ps.proxiesByID {
		if proxy.peer != nil {
			assignedValidators := ps.getValidatorAssignments(nil, []enode.ID{proxy.ID()})
			valAddresses := make([]common.Address, 0, len(assignedValidators))
			for valAddress := range assignedValidators {
				valAddresses = append(valAddresses, valAddress)
			}
			logger.Info("Sending val enode share msg to proxy", "proxy peer", proxy.peer, "valAddresses length", len(valAddresses))
			logger.Trace("Sending val enode share msg to proxy with validator addresses", "valAddresses", common.ConvertToStringSlice(valAddresses))
			pv.sendValEnodesShareMsg(proxy.peer, valAddresses)
		}
	}
}

// sendEnodeCerts will send the appropriate enode certificate to the proxies.
// This is a no-op for replica validators.
func (pv *proxiedValidatorEngine) sendEnodeCerts(ps *proxySet, enodeCerts map[enode.ID]*istanbul.EnodeCertMsg) {
	logger := pv.logger.New("func", "sendEnodeCerts")
	if !pv.backend.IsValidating() {
		logger.Trace("Skipping sending EnodeCerts to proxies b/c not validating")
		return
	}

	for proxyID, proxy := range ps.proxiesByID {
		if proxy.peer != nil && enodeCerts[proxyID] != nil {
			// Generate message payload.  Note that these enode certs are already signed by the validator
			payload, err := enodeCerts[proxyID].Msg.Payload()
			if err != nil {
				logger.Error("Error getting payload of enode certificate message", "err", err, "proxyID", proxyID)
			}

			logger.Info("Sharing enode certificate to proxy", "proxy peer", proxy.peer, "proxyID", proxyID)
			pv.backend.Unicast(proxy.peer, payload, istanbul.EnodeCertificateMsg)
		}
	}

}

// updateValidatorAssignments will retrieve find the validator conn set diff between the current validator
// conn set and the proxy set's validator conn set, and apply any diff to the proxy set.
func (pv *proxiedValidatorEngine) updateValidatorAssignments(ps *proxySet) (bool, error) {
	newVals, rmVals, err := pv.getValidatorConnSetDiff(ps.getValidators())
	log.Trace("Proxy Handler updating validators", "newVals", common.ConvertToStringSlice(newVals), "rmVals", common.ConvertToStringSlice(rmVals), "err", err, "func", "updateValiadtors")
	if err != nil {
		return false, err
	}

	valsReassigned := false
	if len(newVals) > 0 {
		valsReassigned = valsReassigned || ps.addRemoteValidators(newVals)
	}

	if len(rmVals) > 0 {
		valsReassigned = valsReassigned || ps.removeRemoteValidators(rmVals)
	}

	return valsReassigned, nil
}

// This function will return a diff between the current Validator Connection set and the `validators` parameter.
func (pv *proxiedValidatorEngine) getValidatorConnSetDiff(validators []common.Address) (newVals []common.Address, rmVals []common.Address, err error) {
	logger := pv.logger.New("func", "getValidatorConnSetDiff")

	logger.Trace("Proxied validator engine retrieving validator connection set diff", "validators", common.ConvertToStringSlice(validators))

	// Get the set of active and registered validators
	newValConnSet, err := pv.backend.RetrieveValidatorConnSet()
	if err != nil {
		logger.Warn("Proxy Handler couldn't get the validator connection set", "err", err)
		return nil, nil, err
	}

	// Don't add this validator's address to the returned new validator set
	delete(newValConnSet, pv.backend.Address())

	outputNewValConnSet := make([]common.Address, 0, len(newValConnSet))
	for newVal := range newValConnSet {
		outputNewValConnSet = append(outputNewValConnSet, newVal)
	}
	logger.Trace("retrieved validator connset", "valConnSet", common.ConvertToStringSlice(outputNewValConnSet))

	rmVals = make([]common.Address, 0) // There is a good chance that there will be no diff, so set size to 0

	// First find all old val entries that are not in the newValConnSet (which will be the removed validator set),
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

func (pv *proxiedValidatorEngine) IsProxyPeer(peerID enode.ID) (bool, error) {
	proxy, err := pv.getProxy(peerID)
	if err != nil {
		return false, err
	}

	return proxy != nil, nil
}

func (pv *proxiedValidatorEngine) getProxy(peerID enode.ID) (*Proxy, error) {
	proxies, _, err := pv.GetProxiesAndValAssignments()
	if err != nil {
		return nil, err
	}

	for _, proxy := range proxies {
		if proxy.peer != nil && proxy.peer.Node().ID() == peerID {
			return proxy, nil
		}
	}

	return nil, nil
}
