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
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/consensustest"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/backend/backendtest"
)

func TestHandleValEnodeShare(t *testing.T) {
	// Create two validators and one proxy backend.
	// 1) Proxied validator (val0)
	// 2) Non proxied validator (val1)
	numValidators := 2
	genesisCfg, nodeKeys := backendtest.GetGenesisAndKeys(numValidators, true)

	val0BEi, _ := backendtest.NewTestBackend(false, common.Address{}, true, genesisCfg, nodeKeys[0])
	val0BE := val0BEi.(BackendForProxiedValidatorEngine)
	val0Peer := consensustest.NewMockPeer(val0BE.SelfNode())

	proxyBEi, _ := backendtest.NewTestBackend(true, val0BE.Address(), false, genesisCfg, nil)
	proxyBE := proxyBEi.(BackendForProxyEngine)

	val1BEi, _ := backendtest.NewTestBackend(false, common.Address{}, false, genesisCfg, nodeKeys[1])
	val1BE := val1BEi.(BackendForProxiedValidatorEngine)
	val1Peer := consensustest.NewMockPeer(val1BE.SelfNode())

	// Get the proxied validator engine object
	pvi := val0BE.GetProxiedValidatorEngine()
	pv := pvi.(*proxiedValidatorEngine)

	// Get the proxy engine object
	pi := proxyBE.GetProxyEngine()
	p := pi.(*proxyEngine)

	// Sleep for 5 seconds so that val1BE will generate it's enode certificate.
	time.Sleep(5 * time.Second)

	// Register the proxied validator with the proxy object
	p.RegisterProxiedValidatorPeer(val0Peer)

	// "Send" an enode certificate msg from val1 to val0
	val1EnodeCertMap := val1BE.RetrieveEnodeCertificateMsgMap()
	val1EnodeCert := val1EnodeCertMap[val1BE.SelfNode().ID()].Msg
	val1EnodeCertPayload, _ := val1EnodeCert.Payload()

	p2pMsg, err := backendtest.CreateP2PMsg(istanbul.EnodeCertificateMsg, val1EnodeCertPayload)
	if err != nil {
		t.Errorf("Error in creating p2p message.  Error: %v", err)
	}
	handled, err := val0BEi.HandleMsg(val1BE.Address(),
		p2pMsg,
		val1Peer)
	if !handled || err != nil {
		t.Errorf("Error in handling enode certificate msg.  Handled: %v, Error: %v", handled, err)
	}

	// Sleep for a bit since the handling of the enode certificate message is done in a separate thread
	time.Sleep(1 * time.Second)

	// Generate val enode share message
	vesMsg, err := pv.generateValEnodesShareMsg([]common.Address{val1BE.Address()})
	if err != nil {
		t.Errorf("Error in generating a val enode share msg")
	}

	vesMsgPayload, _ := vesMsg.Payload()
	// "Send" it to the proxy
	handled, err = p.handleValEnodesShareMsg(val0Peer, vesMsgPayload)
	if !handled || err != nil {
		t.Errorf("Uncessfully handled val enode share message.  Handled: %v, Err: %v", handled, err)
	}
	// Sleep for a bit since the handling of the val enode share message is done in a separate thread
	time.Sleep(1 * time.Second)

	// Verify that the proxy has val1's enode in it's val enode table
	vetEntries, err := proxyBE.GetValEnodeTableEntries(nil)
	if err != nil {
		t.Errorf("Error in retrieving val enode table entries from proxy.  Err: %v", err)
	}

	if len(vetEntries) != 1 {
		t.Errorf("Incorrect number of val enode table entries.  Have %d, Want: 1", len(vetEntries))
	}

	expectedVetEntry := istanbul.AddressEntry{Address: val1BE.Address(), Node: val1BE.SelfNode()}
	if vetEntries[val1BE.Address()].Address != expectedVetEntry.Address || vetEntries[val1BE.Address()].Node.URLv4() != expectedVetEntry.Node.URLv4() {
		t.Errorf("Proxy has incorrect val enode table entry.  Want: %v, Have: %v", expectedVetEntry, vetEntries[val1BE.Address()])
	}
}

func TestHandleEnodeMessageFromRemoteVal(t *testing.T) {
	// Test that the node will forward a message from val within val connection set

	// Test that the node will not forward a message not within val connetion set
}

func TestHandleEnodeMessageFromProxiedVal(t *testing.T) {
	// Test that the node will save the enode certificate into it's enode certificate msg map
}

func TestHandleConsensusMsg(t *testing.T) {
	// Test that the node will forward a consensus message from a val within val set

	// Test that the ndoe will not forward a consensus message from a val not within val set
}
