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
	"reflect"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/backendtest"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/p2p"
)

func TestAddProxy(t *testing.T) {
	numValidators := 2
	genesisCfg, nodeKeys := backendtest.GetGenesisAndKeys(numValidators)

	valBEi, _ := backendtest.NewTestBackend(false, common.Address{}, true, genesisCfg, nodeKeys[0])
	valBE := valBEi.(BackendForProxiedValidatorEngine)

	proxyBEi, _ := backendtest.NewTestBackend(true, valBE.Address(), false, genesisCfg, nil)
	proxyBE := proxyBEi.(BackendForProxyEngine)
	proxyPeer := consensustest.NewMockPeer(proxyBE.SelfNode(), p2p.ProxyPurpose)

	remoteValAddress := crypto.PubkeyToAddress(nodeKeys[1].PublicKey)

	// Get the proxied validator engine object
	pvi := valBE.GetProxiedValidatorEngine()
	pv := pvi.(*proxiedValidatorEngine)

	// Add the proxy to the proxied validator
	pv.AddProxy(proxyBE.SelfNode(), proxyBE.SelfNode())

	// Make sure the added proxy is within the proxy set but not assigned anything
	proxies, assignments, err := pv.GetProxiesAndValAssignments()

	if err != nil {
		t.Errorf("Error in retrieving proxies and val assignments. Error: %v", err)
	}

	expectedProxy := Proxy{node: proxyBE.SelfNode(),
		externalNode: proxyBE.SelfNode(),
		peer:         nil}

	if len(proxies) != 1 || proxies[0].node.URLv4() != expectedProxy.node.URLv4() || proxies[0].externalNode.URLv4() != expectedProxy.externalNode.URLv4() || proxies[0].peer != expectedProxy.peer {
		t.Errorf("Unexpected proxies value.  len(proxies): %d, proxy[0]: %v, expectedProxy: %v", len(proxies), proxies[0], expectedProxy)
	}

	if len(assignments) != 1 || assignments[proxyBE.SelfNode().ID()] == nil || len(assignments[proxyBE.SelfNode().ID()]) != 0 {
		t.Errorf("Unexpected validator assignments.  assignments: %v", assignments)
	}

	announceVersion := valBE.GetAnnounceVersion()

	// Connect the ProxyPeer
	pv.RegisterProxyPeer(proxyPeer)

	// Sleep 6s since the registration of the proxy peer is asynchronous
	time.Sleep(6 * time.Second)

	// Make sure the added proxy is within the proxy set and assigned to the remote validator
	proxies, assignments, err = pv.GetProxiesAndValAssignments()

	if err != nil {
		t.Errorf("Error in retrieving proxies and val assignments. Error: %v", err)
	}

	expectedProxy = Proxy{node: proxyBE.SelfNode(),
		externalNode: proxyBE.SelfNode(),
		peer:         proxyPeer}

	if len(proxies) != 1 || reflect.DeepEqual(proxies[0], expectedProxy.node.URLv4()) {
		t.Errorf("Unexpected proxies value.  len(proxies): %d, proxy[0]: %v, expectedProxy: %v", len(proxies), proxies[0], expectedProxy)
	}

	if len(assignments) != 1 || assignments[proxyBE.SelfNode().ID()] == nil || len(assignments[proxyBE.SelfNode().ID()]) != 1 || assignments[proxyBE.SelfNode().ID()][0] != remoteValAddress {
		t.Errorf("Unexpected validator assignments.  assignments: %v", assignments)
	}

	// Make sure that the announce version is incremented
	if valBE.GetAnnounceVersion() <= announceVersion {
		t.Errorf("Proxied validator announce version was not updated.  announceVersion: %d, valBE.GetAnnounceVersion(): %d", announceVersion, valBE.GetAnnounceVersion())
	}

	// Make sure that the enode certificates are correctly created for the added proxy
	ecMsgMap := valBE.RetrieveEnodeCertificateMsgMap()
	if len(ecMsgMap) != 1 || ecMsgMap[proxyBE.SelfNode().ID()] == nil {
		t.Errorf("Proxied validator enode cert message map incorrect.  ecMsgMap: %v", ecMsgMap)
	}

	announceVersion = valBE.GetAnnounceVersion()

	// Remove the proxy from the proxied validator
	pv.RemoveProxy(proxyBE.SelfNode())

	// Sleep for a sec since the proxy removal is asynchronous
	time.Sleep(1 * time.Second)

	// Make sure the removed proxy's is not within the proxy set
	proxies, assignments, err = pv.GetProxiesAndValAssignments()

	// Make sure that this proxy's enode certificate is removed from the enode certificate maps
	if err != nil {
		t.Errorf("Error in retrieving proxies and val assignments. Error: %v", err)
	}

	if len(proxies) != 0 {
		t.Errorf("Unexpected proxies.  Proxies: %v", proxies)
	}

	if len(assignments) != 0 {
		t.Errorf("Unexpected validator assignments.  assignments: %v", assignments)
	}

	// Make sure that the announce version is incremented
	if valBE.GetAnnounceVersion() <= announceVersion {
		t.Errorf("Proxied validator announce version was not updated.  announceVersion: %d, valBE.GetAnnounceVersion(): %d", announceVersion, valBE.GetAnnounceVersion())
	}
}
