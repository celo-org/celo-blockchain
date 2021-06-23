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
	"reflect"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/backendtest"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/davecgh/go-spew/spew"
)

func TestHandleValEnodeShare(t *testing.T) {
	// Create two validators and one proxy backend.
	// 1) Proxied validator (val0)
	// 2) Non proxied validator (val1)
	numValidators := 2
	genesisCfg, nodeKeys := backendtest.GetGenesisAndKeys(numValidators, true)

	val0BEi, _ := backendtest.NewTestBackend(false, common.Address{}, true, genesisCfg, nodeKeys[0])
	val0BE := val0BEi.(BackendForProxiedValidatorEngine)
	val0Peer := consensustest.NewMockPeer(val0BE.SelfNode(), p2p.AnyPurpose)

	proxyBEi, _ := backendtest.NewTestBackend(true, val0BE.Address(), false, genesisCfg, nil)
	proxyBE := proxyBEi.(BackendForProxyEngine)

	val1BEi, _ := backendtest.NewTestBackend(false, common.Address{}, false, genesisCfg, nodeKeys[1])
	val1BE := val1BEi.(BackendForProxiedValidatorEngine)
	val1Peer := consensustest.NewMockPeer(val1BE.SelfNode(), p2p.AnyPurpose)

	// Get the proxied validator engine object
	pvi := val0BE.GetProxiedValidatorEngine()
	pv := pvi.(*proxiedValidatorEngine)

	// Get the proxy engine object
	pi := proxyBE.GetProxyEngine()
	p := pi.(*proxyEngine)

	// Sleep for 6 seconds so that val1BE will generate it's enode certificate.
	time.Sleep(6 * time.Second)

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
	spew.Dump(expectedVetEntry)
	spew.Dump(vetEntries)
	spew.Dump(val1BE.Address())
	if vetEntries[val1BE.Address()].Address != expectedVetEntry.Address || vetEntries[val1BE.Address()].Node.URLv4() != expectedVetEntry.Node.URLv4() {
		t.Errorf("Proxy has incorrect val enode table entry.  Want: %v, Have: %v", expectedVetEntry, vetEntries[val1BE.Address()])
	}
}

func TestHandleEnodeCertificateMessage(t *testing.T) {
	// Create two validators and one proxy backend.
	// 1) Proxied validator (val0)
	// 2) Non proxied validator (val1)
	numValidators := 2
	genesisCfg, nodeKeys := backendtest.GetGenesisAndKeys(numValidators, true)

	val0BEi, _ := backendtest.NewTestBackend(false, common.Address{}, true, genesisCfg, nodeKeys[0])
	val0BE := val0BEi.(BackendForProxiedValidatorEngine)
	val0Peer := consensustest.NewMockPeer(val0BE.SelfNode(), p2p.AnyPurpose)

	proxyBEi, _ := backendtest.NewTestBackend(true, val0BE.Address(), false, genesisCfg, nil)
	proxyBE := proxyBEi.(BackendForProxyEngine)
	proxyPeer := consensustest.NewMockPeer(proxyBE.SelfNode(), p2p.ProxyPurpose)

	// Get the proxied validator engine object
	pvi := val0BE.GetProxiedValidatorEngine()
	pv := pvi.(*proxiedValidatorEngine)

	// Add the proxy to the proxied validator
	pv.AddProxy(proxyBE.SelfNode(), proxyBE.SelfNode())
	pv.RegisterProxyPeer(proxyPeer)

	// Register the proxied validator with the proxy object
	pi := proxyBE.GetProxyEngine()
	p := pi.(*proxyEngine)
	p.RegisterProxiedValidatorPeer(val0Peer)

	val1BEi, _ := backendtest.NewTestBackend(false, common.Address{}, false, genesisCfg, nodeKeys[1])
	val1BE := val1BEi.(BackendForProxiedValidatorEngine)
	val1Peer := consensustest.NewMockPeer(val1BE.SelfNode(), p2p.ValidatorPurpose)

	// Create a non elected validator
	unelectedValKey, _ := crypto.GenerateKey()
	unelectedValBEi, _ := backendtest.NewTestBackend(false, common.Address{}, false, genesisCfg, unelectedValKey)
	unelectedValBE := unelectedValBEi.(BackendForProxiedValidatorEngine)
	unelectedValPeer := consensustest.NewMockPeer(unelectedValBE.SelfNode(), p2p.AnyPurpose)

	// Sleep for 6 seconds so that val1BE will generate it's enode certificate.
	time.Sleep(6 * time.Second)

	// Test that the node will forward a message from val within val connection set
	testEnodeCertFromRemoteVal(t, val1BE, val1Peer, proxyBEi)

	// Test that the node will not forward a message not within val connetion set
	testEnodeCertFromUnelectedRemoteVal(t, unelectedValBE, unelectedValKey, unelectedValPeer, proxyBEi)

	// Test that the proxy will save the enode certificate into it's enode certificate msg map
	// when it's proxy sends it.
	testEnodeCertFromProxiedVal(t, val0BE, val0Peer, proxyBEi, proxyBE)
}

func testEnodeCertFromRemoteVal(t *testing.T, valBE BackendForProxiedValidatorEngine, valPeer consensus.Peer, proxyBEi backendtest.TestBackendInterface) {
	valEnodeCertMap := valBE.RetrieveEnodeCertificateMsgMap()
	valEnodeCert := valEnodeCertMap[valBE.SelfNode().ID()].Msg
	valEnodeCertPayload, _ := valEnodeCert.Payload()

	p2pMsg, err := backendtest.CreateP2PMsg(istanbul.EnodeCertificateMsg, valEnodeCertPayload)
	if err != nil {
		t.Errorf("Error in creating p2p message.  Error: %v", err)
	}
	handled, err := proxyBEi.HandleMsg(valBE.Address(),
		p2pMsg,
		valPeer)
	if !handled || err != nil {
		t.Errorf("Error in handling enode certificate msg.  Handled: %v, Error: %v", handled, err)
	}
}

func testEnodeCertFromUnelectedRemoteVal(t *testing.T, unelectedValBE BackendForProxiedValidatorEngine, unelectedValKey *ecdsa.PrivateKey, unelectedValPeer consensus.Peer, proxyBEi backendtest.TestBackendInterface) {
	ecMsg := istanbul.NewEnodeCeritifcateMessage(&istanbul.EnodeCertificate{
		EnodeURL: unelectedValBE.SelfNode().URLv4(),
		Version:  1,
	}, unelectedValBE.Address())
	// Sign the message
	if err := ecMsg.Sign(func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), unelectedValKey)
	}); err != nil {
		t.Errorf("Error in signing enode certificate message.  Error: %v", err)
	}

	ecMsgPayload, _ := ecMsg.Payload()

	p2pMsg, err := backendtest.CreateP2PMsg(istanbul.EnodeCertificateMsg, ecMsgPayload)
	if err != nil {
		t.Errorf("Error in creating p2p message.  Error: %v", err)
	}
	handled, err := proxyBEi.HandleMsg(unelectedValBE.Address(),
		p2pMsg,
		unelectedValPeer)
	if !handled || err != istanbul.ErrUnauthorizedAddress {
		t.Errorf("Error in handling enode certificate msg.  Handled: %v, Error: %v", handled, err)
	}
}

func testEnodeCertFromProxiedVal(t *testing.T, proxiedValBE BackendForProxiedValidatorEngine, proxiedValPeer consensus.Peer, proxyBEi backendtest.TestBackendInterface, proxyBE BackendForProxyEngine) {
	valEnodeCertMap := proxiedValBE.RetrieveEnodeCertificateMsgMap()
	valEnodeCert := valEnodeCertMap[proxyBE.SelfNode().ID()].Msg
	valEnodeCertPayload, _ := valEnodeCert.Payload()

	p2pMsg, err := backendtest.CreateP2PMsg(istanbul.EnodeCertificateMsg, valEnodeCertPayload)
	if err != nil {
		t.Errorf("Error in creating p2p message.  Error: %v", err)
	}
	handled, err := proxyBEi.HandleMsg(proxiedValBE.Address(),
		p2pMsg,
		proxiedValPeer)
	if !handled || err != nil {
		t.Errorf("Error in handling enode certificate msg.  Handled: %v, Error: %v", handled, err)
	}

	// Sleep for a bit since the handling of the val enode share message is done in a separate thread
	time.Sleep(1 * time.Second)

	// Verify that the proxy has val1's enode in it's enode certificate msg map
	msgMap := proxyBE.RetrieveEnodeCertificateMsgMap()

	if len(msgMap) != 1 {
		t.Errorf("Incorrect number of val enode table entries.  Have %d, Want: 1", len(msgMap))
	}

	if msgMap[proxyBE.SelfNode().ID()] == nil {
		t.Errorf("Proxy enode cert message map has incorrect entry.  Want: %v, Have: %v", proxyBE.SelfNode().ID(), reflect.ValueOf(msgMap).MapKeys()[0])
	}
}

func TestHandleConsensusMsg(t *testing.T) {
	// Create two validators and one proxy backend.
	// 1) Proxied validator (val0)
	// 2) Non proxied validator (val1)
	numValidators := 2
	genesisCfg, nodeKeys := backendtest.GetGenesisAndKeys(numValidators, true)

	val0BEi, _ := backendtest.NewTestBackend(false, common.Address{}, true, genesisCfg, nodeKeys[0])
	val0BE := val0BEi.(BackendForProxiedValidatorEngine)
	val0Peer := consensustest.NewMockPeer(val0BE.SelfNode(), p2p.AnyPurpose)

	proxyBEi, _ := backendtest.NewTestBackend(true, val0BE.Address(), false, genesisCfg, nil)
	proxyBE := proxyBEi.(BackendForProxyEngine)
	proxyPeer := consensustest.NewMockPeer(proxyBE.SelfNode(), p2p.ProxyPurpose)

	// Get the proxied validator engine object
	pvi := val0BE.GetProxiedValidatorEngine()
	pv := pvi.(*proxiedValidatorEngine)

	// Add the proxy to the proxied validator
	pv.AddProxy(proxyBE.SelfNode(), proxyBE.SelfNode())
	pv.RegisterProxyPeer(proxyPeer)

	// Register the proxied validator with the proxy object
	pi := proxyBE.GetProxyEngine()
	p := pi.(*proxyEngine)
	p.RegisterProxiedValidatorPeer(val0Peer)

	val1BEi, _ := backendtest.NewTestBackend(false, common.Address{}, false, genesisCfg, nodeKeys[1])
	val1BE := val1BEi.(BackendForProxiedValidatorEngine)
	val1Peer := consensustest.NewMockPeer(val1BE.SelfNode(), p2p.ValidatorPurpose)

	// Create a non elected validator
	unelectedValKey, _ := crypto.GenerateKey()
	unelectedValBEi, _ := backendtest.NewTestBackend(false, common.Address{}, false, genesisCfg, unelectedValKey)
	unelectedValBE := unelectedValBEi.(BackendForProxiedValidatorEngine)
	unelectedValPeer := consensustest.NewMockPeer(unelectedValBE.SelfNode(), p2p.AnyPurpose)

	// Sleep for 5 seconds so that val1BE will generate it's enode certificate.
	time.Sleep(5 * time.Second)

	subject := &istanbul.Subject{
		View: &istanbul.View{
			Round:    common.Big0,
			Sequence: common.Big0,
		},
		Digest: common.Hash{},
	}

	// Test that the node will forward a consensus message from val within val connection set
	testConsensusMsgFromRemoteVal(t, istanbul.NewPrepareMessage(subject, val1BE.Address()), val1BE, nodeKeys[1], val1Peer, proxyBEi)

	// Test that the node will not forward a consensus message not within val connetion set
	testConsensusMsgFromUnelectedRemoteVal(t, istanbul.NewPrepareMessage(subject, unelectedValBE.Address()), unelectedValBE, unelectedValKey, unelectedValPeer, proxyBEi)

	// Test that the proxy will not handle a consensus message from the proxied validator
	testConsensusMsgFromProxiedVal(t, istanbul.NewPrepareMessage(subject, val0BE.Address()), val0BE, nodeKeys[0], val0Peer, proxyBEi)
}

func testConsensusMsgFromRemoteVal(t *testing.T, consMsg *istanbul.Message, valBE BackendForProxiedValidatorEngine, valKey *ecdsa.PrivateKey, valPeer consensus.Peer, proxyBEi backendtest.TestBackendInterface) {
	if err := consMsg.Sign(func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), valKey)
	}); err != nil {
		t.Errorf("Error in signing consensus message.  Error: %v", err)
	}

	consMsgPayload, _ := consMsg.Payload()

	p2pMsg, err := backendtest.CreateP2PMsg(istanbul.ConsensusMsg, consMsgPayload)
	if err != nil {
		t.Errorf("Error in creating p2p message.  Error: %v", err)
	}
	handled, err := proxyBEi.HandleMsg(valBE.Address(),
		p2pMsg,
		valPeer)
	if !handled || err != nil {
		t.Errorf("Error in handling consensus msg.  Handled: %v, Error: %v", handled, err)
	}
}

func testConsensusMsgFromUnelectedRemoteVal(t *testing.T, consMsg *istanbul.Message, unelectedValBE BackendForProxiedValidatorEngine, unelectedValKey *ecdsa.PrivateKey, unelectedValPeer consensus.Peer, proxyBEi backendtest.TestBackendInterface) {
	// Sign the message
	if err := consMsg.Sign(func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), unelectedValKey)
	}); err != nil {
		t.Errorf("Error in signing consensus message.  Error: %v", err)
	}

	consMsgPayload, _ := consMsg.Payload()

	p2pMsg, err := backendtest.CreateP2PMsg(istanbul.ConsensusMsg, consMsgPayload)
	if err != nil {
		t.Errorf("Error in creating p2p message.  Error: %v", err)
	}
	handled, err := proxyBEi.HandleMsg(unelectedValBE.Address(),
		p2pMsg,
		unelectedValPeer)
	if !handled || err != istanbul.ErrUnauthorizedAddress {
		t.Errorf("Error in handling consensus msg.  Handled: %v, Error: %v", handled, err)
	}
}

func testConsensusMsgFromProxiedVal(t *testing.T, consMsg *istanbul.Message, proxiedValBE BackendForProxiedValidatorEngine, proxiedValKey *ecdsa.PrivateKey, proxiedValPeer consensus.Peer, proxyBEi backendtest.TestBackendInterface) {
	// Sign the message
	if err := consMsg.Sign(func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), proxiedValKey)
	}); err != nil {
		t.Errorf("Error in signing consensus message.  Error: %v", err)
	}

	consMsgPayload, _ := consMsg.Payload()

	p2pMsg, err := backendtest.CreateP2PMsg(istanbul.ConsensusMsg, consMsgPayload)
	if err != nil {
		t.Errorf("Error in creating p2p message.  Error: %v", err)
	}
	handled, _ := proxyBEi.HandleMsg(proxiedValBE.Address(),
		p2pMsg,
		proxiedValPeer)
	if handled {
		t.Errorf("Unexpectedly handled a consensus message from the proxied validator")
	}
}
