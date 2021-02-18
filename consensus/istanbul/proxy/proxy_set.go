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
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

// proxySet defines the set of proxies that the validator is aware of and
// validator/proxy assignments.
// WARNING:  None of this object's functions are threadsafe, so it's
//           the user's responsibility to ensure that.
type proxySet struct {
	proxiesByID    map[enode.ID]*Proxy // all proxies known by this node, whether or not they are peered
	valAssignments *valAssignments     // the mappings of proxy<->remote validators
	valAssigner    assignmentPolicy    // used for assigning peered proxies with remote validators
	logger         log.Logger
}

func newProxySet(assignmentPolicy assignmentPolicy) *proxySet {
	return &proxySet{
		proxiesByID:    make(map[enode.ID]*Proxy),
		valAssignments: newValAssignments(),
		valAssigner:    assignmentPolicy,
		logger:         log.New(),
	}
}

// addProxy adds a proxy to the proxySet if it does not exist.
// The valAssigner is not made aware of the proxy until after the proxy
// is peered with.
func (ps *proxySet) addProxy(newProxy *istanbul.ProxyConfig) {
	logger := ps.logger.New("func", "addProxy")
	logger.Trace("Adding proxy to the proxy set", "new proxy internal ID", newProxy.InternalNode.String(), "new proxy external ID", newProxy.ExternalNode.String())
	internalID := newProxy.InternalNode.ID()
	if ps.proxiesByID[internalID] == nil {
		ps.proxiesByID[internalID] = &Proxy{
			node:         newProxy.InternalNode,
			externalNode: newProxy.ExternalNode,
			peer:         nil,
			disconnectTS: time.Now(),
		}
	} else {
		logger.Warn("Cannot add proxy, since a proxy with the same internal enode ID exists already")
	}
}

// getProxy returns the proxy in the proxySet with ID proxyID
func (ps *proxySet) getProxy(proxyID enode.ID) *Proxy {
	proxy, ok := ps.proxiesByID[proxyID]
	if ok {
		return proxy
	}
	return nil
}

// removeProxy removes a proxy with ID proxyID from the proxySet and valAssigner.
// Will return true if any of the validators got reassigned to a different proxy.
func (ps *proxySet) removeProxy(proxyID enode.ID) bool {
	proxy := ps.getProxy(proxyID)
	if proxy == nil {
		return false
	}
	valsReassigned := ps.valAssigner.removeProxy(proxy, ps.valAssignments)
	delete(ps.proxiesByID, proxyID)
	return valsReassigned
}

// setProxyPeer sets the peer for a proxy with enode ID proxyID.
// Since this proxy is now connected tto the proxied validator, it
// can now be assigned remote validators.
func (ps *proxySet) setProxyPeer(proxyID enode.ID, peer consensus.Peer) bool {
	logger := ps.logger.New("func", "setProxyPeer")
	logger.Trace("Setting proxy peer for proxy set", "proxyID", proxyID)
	proxy := ps.proxiesByID[proxyID]
	valsReassigned := false
	if proxy != nil {
		proxy.peer = peer
		logger.Trace("Assigning validators to proxy", "proxyID", proxyID)
		valsReassigned = ps.valAssigner.assignProxy(proxy, ps.valAssignments)
	}

	return valsReassigned
}

// removeProxyPeer sets the peer for a proxy with ID proxyID to nil.
func (ps *proxySet) removeProxyPeer(proxyID enode.ID) {
	proxy := ps.proxiesByID[proxyID]
	if proxy != nil {
		proxy.peer = nil
		proxy.disconnectTS = time.Now()
	}
}

// addRemoteValidators adds remote validators to be assigned by the valAssigner
func (ps *proxySet) addRemoteValidators(validators []common.Address) bool {
	ps.logger.Trace("adding remote validators to the proxy set", "validators", common.ConvertToStringSlice(validators))
	return ps.valAssigner.assignRemoteValidators(validators, ps.valAssignments)
}

// removeRemoteValidators removes remote validators from the validator assignments
func (ps *proxySet) removeRemoteValidators(validators []common.Address) bool {
	ps.logger.Trace("removing remote validators from the proxy set", "validators", common.ConvertToStringSlice(validators))
	return ps.valAssigner.removeRemoteValidators(validators, ps.valAssignments)
}

// getValidatorAssignments returns the validator assignments for the given set of validators filtered on
// the parameters `validators` AND `proxies`.  If either or both of them or nil, then that means that there is no
// filter for that respective dimension.
func (ps *proxySet) getValidatorAssignments(validators []common.Address, proxyIDs []enode.ID) map[common.Address]*Proxy {
	// First get temp set based on proxies filter
	var tempValAssignmentsFromProxies map[common.Address]*enode.ID

	if proxyIDs != nil {
		tempValAssignmentsFromProxies = make(map[common.Address]*enode.ID)
		for _, proxyID := range proxyIDs {
			if proxyValSet, ok := ps.valAssignments.proxyToVals[proxyID]; ok {
				for valAddress := range proxyValSet {
					tempValAssignmentsFromProxies[valAddress] = &proxyID
				}
			}
		}
	} else {
		tempValAssignmentsFromProxies = ps.valAssignments.valToProxy
	}

	// Now get temp set based on validators filter
	var tempValAssignmentsFromValidators map[common.Address]*enode.ID

	if validators != nil {
		tempValAssignmentsFromValidators = make(map[common.Address]*enode.ID)
		for _, valAddress := range validators {
			if enodeID, ok := ps.valAssignments.valToProxy[valAddress]; ok && enodeID != nil {
				tempValAssignmentsFromValidators[valAddress] = enodeID
			}
		}
	} else {
		tempValAssignmentsFromValidators = ps.valAssignments.valToProxy
	}

	// Now do an intersection between the two temporary maps.
	// TODO:  An optimization that can be done is to loop over the temporary map
	//        that is smaller.
	valAssignments := make(map[common.Address]*Proxy)

	for outerValAddress := range tempValAssignmentsFromProxies {
		if enodeID, ok := tempValAssignmentsFromValidators[outerValAddress]; ok {
			if enodeID != nil {
				proxy := ps.getProxy(*enodeID)
				if proxy.peer != nil {
					valAssignments[outerValAddress] = proxy
				}
			} else {
				valAssignments[outerValAddress] = nil
			}
		}
	}

	return valAssignments
}

// unassignDisconnectedProxies unassigns proxies that have been disconnected for
// at least minAge ago
func (ps *proxySet) unassignDisconnectedProxies(minAge time.Duration) bool {
	logger := ps.logger.New("func", "unassignDisconnectedProxies")
	valsReassigned := false
	for proxyID := range ps.valAssignments.proxyToVals {
		proxy := ps.getProxy(proxyID)
		if proxy != nil && proxy.peer == nil && time.Since(proxy.disconnectTS) >= minAge {
			logger.Debug("Unassigning disconnected proxy", "proxy", proxy.String())
			valsReassigned = ps.valAssigner.removeProxy(proxy, ps.valAssignments) || valsReassigned
		}
	}

	return valsReassigned
}

// getValidators returns all validators that are known by the proxy set
func (ps *proxySet) getValidators() []common.Address {
	return ps.valAssignments.getValidators()
}

// getProxyAndValAssignments returns the proxies that are added to the proxied validators
// and the remote validator assignments
func (ps *proxySet) getProxyAndValAssignments() ([]*Proxy, map[enode.ID][]common.Address) {
	proxies := make([]*Proxy, 0, len(ps.proxiesByID))
	valAssignments := make(map[enode.ID][]common.Address)

	for proxyID, proxy := range ps.proxiesByID {
		proxies = append(proxies, proxy)
		assignedVals := ps.getValidatorAssignments(nil, []enode.ID{proxyID})
		assignedValsArray := make([]common.Address, 0, len(assignedVals))

		for val := range assignedVals {
			assignedValsArray = append(assignedValsArray, val)
		}

		valAssignments[proxyID] = assignedValsArray
	}

	return proxies, valAssignments
}
