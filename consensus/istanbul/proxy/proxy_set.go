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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// This type defines the set of proxies that the validator is aware of and
// validator/proxy assignments.
// WARNING:  None of this object's functions are threadsafe, so it's
//           the user's responsibility to ensure that.
type proxySet struct {
	proxiesByID    map[enode.ID]*proxy // all proxies known by this node, whether or not they are peered
	valAssignments *valAssignments     // the mappings of proxy<->remote validators
	valAssigner    assignmentPolicy    // used for assigning peered proxies with remote validators
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
func (ps *proxySet) addProxy(newProxy *istanbul.ProxyConfig) {
	internalID := newProxy.InternalNode.ID()
	if ps.proxiesByID[internalID] == nil {
		ps.proxiesByID[internalID] = &proxy{
			node:         newProxy.InternalNode,
			externalNode: newProxy.ExternalNode,
			peer:         nil,
			disconnectTS: time.Now(),
		}
	} else {
		log.Warn("Cannot add proxy, since a proxy with the same internal enode ID exists already", "func", "addProxy")
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
		ps.valAssigner.removeProxy(proxy, ps.valAssignments)
		delete(ps.proxiesByID, proxyID)
	}
}

// setProxyPeer sets the peer for a proxy with enode ID proxyID.
// Since this proxy is now connected tto the proxied validator, it
// can now be assigned remote validators.
func (ps *proxySet) setProxyPeer(proxyID enode.ID, peer consensus.Peer) {
	proxy := ps.proxiesByID[proxyID]
	if proxy != nil {
		proxy.peer = peer
		ps.valAssigner.assignProxy(proxy, ps.valAssignments)
	}
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
func (ps *proxySet) addRemoteValidators(validators []common.Address) {
	ps.valAssigner.assignRemoteValidators(validators, ps.valAssignments)
}

// removeRemoteValidators removes remote validators from the validator assignments
func (ps *proxySet) removeRemoteValidators(validators []common.Address) {
	ps.valAssigner.removeRemoteValidators(validators, ps.valAssignments)
}

// getValidatorAssignments returns the validator assignments for the given set of validators.
// If the "validators" parameter is nil, then all the validator assignments are returned.
func (ps *proxySet) getValidatorAssignments(validators []common.Address) map[common.Address]*proxy {
	proxies := make(map[common.Address]*proxy)

	if validators == nil {
		for val, proxyID := range ps.valAssignments.valToProxy {
			if proxyID != nil {
				proxy := ps.getProxy(*proxyID)
				if proxy.peer != nil {
					proxies[val] = proxy
				}
			}
		}
	} else {
		for _, val := range validators {
			proxyID := ps.valAssignments.valToProxy[val]
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

// unassignDisconnectedProxies unassigns proxies that have been disconnected for
// at least minAge ago
func (ps *proxySet) unassignDisconnectedProxies(minAge time.Duration) {
	for proxyID := range ps.valAssignments.proxyToVals {
		proxy := ps.getProxy(proxyID)
		if proxy != nil && proxy.peer == nil && time.Since(proxy.disconnectTS) >= minAge {
			log.Debug("Unassigning disconnected proxy", "proxy", proxy.String(), "func", "unassignDisconnectedProxies")
			ps.valAssigner.removeProxy(proxy, ps.valAssignments)
		}
	}
}

// getValidators returns all validators that are known by the proxy set
func (ps *proxySet) getValidators() []common.Address {
	return ps.valAssignments.getValidators()
}

// getProxyInfo returns basic info on all the proxies in the proxySet
func (ps *proxySet) getProxyInfo() []ProxyInfo {
	proxies := make([]ProxyInfo, len(ps.proxiesByID))

	i := 0
	for proxyID, proxy := range ps.proxiesByID {
		proxies[i] = proxy.Info()
		assignedVals := ps.valAssignments.proxyToVals[proxyID]

		if assignedVals != nil && len(assignedVals) > 0 {
			proxies[i].Validators = make([]common.Address, len(assignedVals))

			for val := range assignedVals {
				proxies[i].Validators = append(proxies[i].Validators, val)
			}
		}

		i++
	}
	return proxies
}
