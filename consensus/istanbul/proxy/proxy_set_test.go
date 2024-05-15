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
	"crypto/ecdsa"
	"math/rand"
	"net"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

func createProxyConfig(randomSeed int64) *istanbul.ProxyConfig {
	rng := rand.New(rand.NewSource(randomSeed))

	key, err := ecdsa.GenerateKey(crypto.S256(), rng)
	if err != nil {
		panic("couldn't generate key: " + err.Error())
	}

	internalPort := rng.Int()
	externalPort := internalPort + 1

	return &istanbul.ProxyConfig{
		InternalNode: enode.NewV4(&key.PublicKey, net.IPv6loopback, internalPort, 0),
		ExternalNode: enode.NewV4(&key.PublicKey, net.IPv6loopback, externalPort, 0),
	}
}

func TestProxySet(t *testing.T) {
	proxy0Config := createProxyConfig(0)
	proxy1Config := createProxyConfig(1)
	proxy2Config := createProxyConfig(2)

	proxy0ID := proxy0Config.InternalNode.ID()
	proxy1ID := proxy1Config.InternalNode.ID()
	proxy2ID := proxy2Config.InternalNode.ID()

	proxy0Peer := consensustest.NewMockPeer(proxy0Config.InternalNode, p2p.ProxyPurpose)
	proxy1Peer := consensustest.NewMockPeer(proxy1Config.InternalNode, p2p.ProxyPurpose)
	proxy2Peer := consensustest.NewMockPeer(proxy2Config.InternalNode, p2p.ProxyPurpose)

	proxy0 := &Proxy{node: proxy0Config.InternalNode,
		externalNode: proxy0Config.ExternalNode,
		peer:         proxy0Peer}

	proxy1 := &Proxy{node: proxy1Config.InternalNode,
		externalNode: proxy1Config.ExternalNode,
		peer:         proxy1Peer}

	proxy2 := &Proxy{node: proxy2Config.InternalNode,
		externalNode: proxy2Config.ExternalNode,
		peer:         proxy2Peer}

	remoteVal0Address := common.BytesToAddress([]byte("32526362351"))
	remoteVal1Address := common.BytesToAddress([]byte("64362643436"))
	remoteVal2Address := common.BytesToAddress([]byte("72436452463"))
	remoteVal3Address := common.BytesToAddress([]byte("46346373463"))
	remoteVal4Address := common.BytesToAddress([]byte("25364624352"))
	remoteVal5Address := common.BytesToAddress([]byte("73576426242"))
	remoteVal6Address := common.BytesToAddress([]byte("75375374457"))
	remoteVal7Address := common.BytesToAddress([]byte("64262735373"))
	remoteVal8Address := common.BytesToAddress([]byte("23575764262"))
	remoteVal9Address := common.BytesToAddress([]byte("74364626427"))

	proxySetOps := []struct {
		addProxies                  []*istanbul.ProxyConfig     // Proxies to add to the proxy set
		removeProxies               []enode.ID                  // Proxies to remove from the proxy set
		setProxyPeer                map[enode.ID]consensus.Peer // Map of added proxy that gets peered
		removeProxyPeer             *enode.ID                   // Proxy that becomes unpeered
		addRemoteValidators         []common.Address            // Remote validator addresses to add to the proxy set
		removeRemoteValidators      []common.Address            // Remote validator addresses to remove from the proxy set
		expectedValProxyAssignments map[common.Address]*Proxy   // Expected validator to proxy assignments after all proxy set operations are applied
	}{
		// Test with no proxies and no validators
		{
			expectedValProxyAssignments: map[common.Address]*Proxy{},
		},

		// Test with 3 remote validator addresses. Emulate a proxied validator starting with initial valset at genesis block.
		{
			addRemoteValidators:         []common.Address{remoteVal0Address, remoteVal1Address, remoteVal2Address},
			expectedValProxyAssignments: map[common.Address]*Proxy{remoteVal0Address: nil, remoteVal1Address: nil, remoteVal2Address: nil},
		},

		// Test with adding 1 proxy.  Emulate when a proxied validator adds a proxy but that proxy hasn't peered yet.
		{
			addProxies:                  []*istanbul.ProxyConfig{proxy0Config},
			expectedValProxyAssignments: map[common.Address]*Proxy{remoteVal0Address: nil, remoteVal1Address: nil, remoteVal2Address: nil},
		},

		// Test with the proxy being peered.
		{
			setProxyPeer:                map[enode.ID]consensus.Peer{proxy0ID: proxy0Peer},
			expectedValProxyAssignments: map[common.Address]*Proxy{remoteVal0Address: proxy0, remoteVal1Address: proxy0, remoteVal2Address: proxy0},
		},

		// Test with adding an additional proxy
		{
			addProxies:                  []*istanbul.ProxyConfig{proxy1Config},
			expectedValProxyAssignments: map[common.Address]*Proxy{remoteVal0Address: proxy0, remoteVal1Address: proxy0, remoteVal2Address: proxy0},
		},

		// Test with additional proxy peered
		{
			setProxyPeer:                map[enode.ID]consensus.Peer{proxy1ID: proxy1Peer},
			expectedValProxyAssignments: map[common.Address]*Proxy{remoteVal0Address: proxy0, remoteVal1Address: proxy1, remoteVal2Address: proxy0},
		},

		// Test with one of the proxies getting unpeered.  Emulate when a proxy gets disconnected.
		{
			removeProxyPeer:             &proxy0ID,
			expectedValProxyAssignments: map[common.Address]*Proxy{remoteVal0Address: proxy1, remoteVal1Address: proxy1, remoteVal2Address: proxy1},
		},

		// Test the unpeered proxy getting repeered
		{
			setProxyPeer:                map[enode.ID]consensus.Peer{proxy0ID: proxy0Peer},
			expectedValProxyAssignments: map[common.Address]*Proxy{remoteVal0Address: proxy0, remoteVal1Address: proxy1, remoteVal2Address: proxy0},
		},

		// Test with adding 5 more valiators
		{
			addRemoteValidators: []common.Address{remoteVal3Address, remoteVal4Address, remoteVal5Address, remoteVal6Address, remoteVal7Address},
			expectedValProxyAssignments: map[common.Address]*Proxy{remoteVal0Address: proxy0, remoteVal1Address: proxy1, remoteVal2Address: proxy0,
				remoteVal3Address: proxy0, remoteVal4Address: proxy0, remoteVal5Address: proxy1, remoteVal6Address: proxy1, remoteVal7Address: proxy0},
		},

		// Test with two remote validators removed and two added.  Emulate a change in valset at epoch transition
		{
			addRemoteValidators:    []common.Address{remoteVal8Address, remoteVal9Address},
			removeRemoteValidators: []common.Address{remoteVal3Address, remoteVal5Address},
			expectedValProxyAssignments: map[common.Address]*Proxy{remoteVal0Address: proxy0, remoteVal1Address: proxy1, remoteVal2Address: proxy0,
				remoteVal4Address: proxy0, remoteVal6Address: proxy1, remoteVal7Address: proxy0, remoteVal8Address: proxy1, remoteVal9Address: proxy0},
		},

		// Test with adding a new peered proxy
		{
			addProxies:   []*istanbul.ProxyConfig{proxy2Config},
			setProxyPeer: map[enode.ID]consensus.Peer{proxy2ID: proxy2Peer},
			expectedValProxyAssignments: map[common.Address]*Proxy{remoteVal0Address: proxy0, remoteVal1Address: proxy1, remoteVal2Address: proxy2,
				remoteVal4Address: proxy0, remoteVal6Address: proxy1, remoteVal7Address: proxy0, remoteVal8Address: proxy1, remoteVal9Address: proxy0},
		},
	}

	proxiesAdded := make(map[enode.ID]*istanbul.ProxyConfig)
	valsAdded := make(map[common.Address]struct{})
	proxiesPeered := make(map[enode.ID]struct{})
	proxiesNotPeered := make(map[enode.ID]struct{})

	// Testing with consistentHashingPolicy but keeping all tests as
	// implementation-agnostic as possible
	ps := newProxySet(newConsistentHashingPolicy())

	for i, proxySetOp := range proxySetOps {
		// Add proxies
		if proxySetOp.addProxies != nil {
			for _, proxyConfig := range proxySetOp.addProxies {
				ps.addProxy(proxyConfig)
				proxiesAdded[proxyConfig.InternalNode.ID()] = proxyConfig
				proxiesNotPeered[proxyConfig.InternalNode.ID()] = struct{}{}
			}
		}

		// Remove proxies
		if proxySetOp.removeProxies != nil {
			for _, proxyID := range proxySetOp.removeProxies {
				ps.removeProxy(proxyID)
				delete(proxiesAdded, proxyID)
				delete(proxiesPeered, proxyID)
				delete(proxiesNotPeered, proxyID)
			}
		}

		verifyAddedProxies(t, i, ps, proxiesAdded)

		// Set proxy peer
		if proxySetOp.setProxyPeer != nil {
			for proxyID, proxyPeer := range proxySetOp.setProxyPeer {
				ps.setProxyPeer(proxyID, proxyPeer)
				proxiesPeered[proxyID] = struct{}{}
				delete(proxiesNotPeered, proxyID)
			}
		}

		// Remove proxy peer
		if proxySetOp.removeProxyPeer != nil {
			ps.removeProxyPeer(*proxySetOp.removeProxyPeer)
			ps.unassignDisconnectedProxies(0)
			delete(proxiesPeered, *proxySetOp.removeProxyPeer)
			proxiesNotPeered[*proxySetOp.removeProxyPeer] = struct{}{}
		}

		verifyProxyPeers(t, i, ps, proxiesPeered, proxiesNotPeered)

		// Add validator(s)
		if proxySetOp.addRemoteValidators != nil {
			ps.addRemoteValidators(proxySetOp.addRemoteValidators)
			for _, remoteVal := range proxySetOp.addRemoteValidators {
				valsAdded[remoteVal] = struct{}{}
			}
		}

		// Remove validator(s)
		if proxySetOp.removeRemoteValidators != nil {
			ps.removeRemoteValidators(proxySetOp.removeRemoteValidators)
			for _, remoteVal := range proxySetOp.removeRemoteValidators {
				delete(valsAdded, remoteVal)
			}
		}

		verifyAddedValidators(t, i, ps, valsAdded)

		// Verifying that the validator assignments are correct
		verifyValidatorAssignments(t, i, ps, proxySetOp.expectedValProxyAssignments)
	}
}

func verifyAddedProxies(t *testing.T, opID int, ps *proxySet, addedProxies map[enode.ID]*istanbul.ProxyConfig) {
	// Test that adding the proxies results in the correct number of proxies within the proxyset
	if len(ps.proxiesByID) != len(addedProxies) {
		t.Errorf("opID: %d - ps.proxiedByID length is incorrect.  Want: %d, Have: %d", opID, len(addedProxies), len(ps.proxiesByID))
	}

	// Testing that getting the proxies from the proxyset works as intended
	for proxyID, proxyConfig := range addedProxies {
		proxyFromPS := ps.getProxy(proxyID)

		if proxyFromPS == nil || proxyFromPS.node != proxyConfig.InternalNode || proxyFromPS.externalNode != proxyConfig.ExternalNode {
			t.Errorf("opID: %d - ps.getProxy() did not get the correct proxy.  Internal Node (Want: %v, Have: %v)    External Node(Want: %v, Have: %v)", opID,
				proxyConfig.InternalNode, proxyFromPS.node,
				proxyConfig.ExternalNode, proxyFromPS.externalNode)
		}
	}
}

func verifyProxyPeers(t *testing.T, opID int, ps *proxySet, proxiesPeered map[enode.ID]struct{}, proxiesNotPeered map[enode.ID]struct{}) {
	// Verify that the sum of proxiesPeered length and proxiesNotPeered length is
	// equal to the total number of proxies in the proxySet
	if len(proxiesPeered)+len(proxiesNotPeered) != len(ps.proxiesByID) {
		t.Errorf("opID: %d - len(proxiesPeered) + len(proxiesNotPeered) != len(ps.proxiesByID). Want: %d, Have %d", opID, len(proxiesPeered)+len(proxiesNotPeered), len(ps.proxiesByID))
	}

	// iteration through proxiedPeered and verify that those proxies are peered
	for proxyID := range proxiesPeered {
		proxyFromPS := ps.getProxy(proxyID)

		if proxyFromPS == nil {
			t.Errorf("opID: %d - expected proxy not in proxy set. Want: %s, Have: nil", opID, proxyID)
		}

		if proxyFromPS.peer == nil {
			t.Errorf("opID: %d - expected proxy to be peered.  ProxyID: %s", opID, proxyID)
		}
	}

	for proxyID := range proxiesNotPeered {
		proxyFromPS := ps.getProxy(proxyID)

		if proxyFromPS == nil {
			t.Errorf("opID: %d - expected proxy not in proxy set. Want: %s, Have: nil", opID, proxyID)
		}

		if proxyFromPS.peer != nil {
			t.Errorf("opID: %d - expected proxy to be not peered.  ProxyID: %s", opID, proxyID)
		}
	}
}

func verifyAddedValidators(t *testing.T, opID int, ps *proxySet, addedValidators map[common.Address]struct{}) {
	// Test that ps.GetValidators() is the right length and returns the correct set
	valsFromPS := ps.getValidators()

	// Test that the lengths are the same
	if len(valsFromPS) != len(addedValidators) {
		t.Errorf("opID: %d - ps.getValidators() returned incorrect lengthed set.  Want :%d, Have: %d", opID, len(addedValidators), len(valsFromPS))
	}

	valsFromPSMap := make(map[common.Address]struct{})
	for _, valAddress := range valsFromPS {
		valsFromPSMap[valAddress] = struct{}{}
	}

	for expectedValAddress := range addedValidators {
		if _, ok := valsFromPSMap[expectedValAddress]; !ok {
			t.Errorf("opID: %d - expected val address %s not in the proxy set's validators", opID, expectedValAddress.Hex())
		}
	}
}

func verifyValidatorAssignments(t *testing.T, opID int, ps *proxySet, expectedValAssignments map[common.Address]*Proxy) {
	valAssignmentsFromPS := ps.getValidatorAssignments(nil, nil)

	// Test that the lengths of the proxy set's assignments and expected assignments are the same
	if len(expectedValAssignments) != len(valAssignmentsFromPS) {
		t.Errorf("opID: %d - val assignment lengths don't match. Want: %d, Have: %d", opID, len(expectedValAssignments), len(valAssignmentsFromPS))
	}

	// Test that all entries in the expected assignments is equal to the proxy set's assignments
	for val, expectedProxy := range expectedValAssignments {
		proxyFromPS := valAssignmentsFromPS[val]
		if !proxyCompare(expectedProxy, proxyFromPS) {
			t.Errorf("opID: %d - unexpected val assignement.  Want: %s -> %s, Have: %s -> %s", opID, val, proxyOutput(expectedProxy), val, proxyOutput(proxyFromPS))
		}
	}
}

func proxyCompare(p1 *Proxy, p2 *Proxy) bool {
	if p1 == nil && p2 == nil {
		return true
	} else if p1 == nil && p2 != nil {
		return false
	} else if p1 != nil && p2 == nil {
		return false
	} else if p1.node != p2.node || p1.externalNode != p2.externalNode || p1.peer != p2.peer {
		return false
	} else {
		return true
	}
}

func proxyOutput(p *Proxy) string {
	if p == nil {
		return "nil"
	} else {
		return p.node.ID().String()
	}
}
