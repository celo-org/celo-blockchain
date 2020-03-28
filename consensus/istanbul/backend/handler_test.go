// Copyright 2015 The go-ethereum Authors
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
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
)

type MockPeer struct {
	Messages     chan p2p.Msg
	NodeOverride *enode.Node
}

func (p *MockPeer) Send(msgcode uint64, data interface{}) error {
	return nil
}

func (p *MockPeer) Node() *enode.Node {
	if p.NodeOverride != nil {
		return p.NodeOverride
	}
	return enode.MustParse("enode://1dd9d65c4552b5eb43d5ad55a2ee3f56c6cbc1c64a5c8d659f51fcd51bace24351232b8d7821617d2b29b54b81cdefb9b3e9c37d7fd5f63270bcc9e1a6f6a439@127.0.0.1:52150")
}

func (p *MockPeer) Version() int {
	return 0
}

func (p *MockPeer) ReadMsg() (p2p.Msg, error) {
	select {
	case msg := <-p.Messages:
		return msg, nil
	default:
		return p2p.Msg{}, nil
	}
}

func (p *MockPeer) Inbound() bool {
	return false
}

func (p *MockPeer) PurposeIsSet(purpose p2p.PurposeFlag) bool {
	return false
}

func TestIstanbulMessage(t *testing.T) {
	_, backend := newBlockChain(1, true)

	// generate one msg
	data := []byte("data1")
	msg := makeMsg(istanbulQueryEnodeMsg, data)
	addr := common.BytesToAddress([]byte("address"))

	_, err := backend.HandleMsg(addr, msg, &MockPeer{})
	if err != nil {
		t.Fatalf("handle message failed: %v", err)
	}
}

func TestRecentMessageCaches(t *testing.T) {
	// Define the various voting scenarios to test
	tests := []struct {
		ethMsgCode  uint64
		shouldCache bool
	}{
		{
			ethMsgCode:  istanbulConsensusMsg,
			shouldCache: false,
		},
		{
			ethMsgCode:  istanbulQueryEnodeMsg,
			shouldCache: true,
		},
		{
			ethMsgCode:  istanbulValEnodesShareMsg,
			shouldCache: false,
		},
		{
			ethMsgCode:  istanbulFwdMsg,
			shouldCache: false,
		},
		{
			ethMsgCode:  istanbulVersionCertificatesMsg,
			shouldCache: true,
		},
		{
			ethMsgCode:  istanbulEnodeCertificateMsg,
			shouldCache: false,
		},
		{
			ethMsgCode:  istanbulValidatorHandshakeMsg,
			shouldCache: false,
		},
	}

	for _, tt := range tests {
		_, backend := newBlockChain(1, true)

		// generate a msg that is not an Announce
		data := []byte("data1")
		hash := istanbul.RLPHash(data)
		msg := makeMsg(tt.ethMsgCode, data)
		addr := common.BytesToAddress([]byte("address"))

		// 1. this message should not be in cache
		// for peers
		if _, ok := backend.peerRecentMessages.Get(addr); ok {
			t.Fatalf("the cache of messages for this peer should be nil")
		}

		// for self
		if _, ok := backend.selfRecentMessages.Get(hash); ok {
			t.Fatalf("the cache of messages should be nil")
		}

		// 2. this message should be in cache only when ethMsgCode == istanbulQueryEnodeMsg || ethMsgCode == istanbulVersionCertificatesMsg
		_, err := backend.HandleMsg(addr, msg, &MockPeer{})
		if err != nil {
			t.Fatalf("handle message failed: %v", err)
		}

		// for peers
		if ms, ok := backend.peerRecentMessages.Get(addr); tt.shouldCache != ok {
			t.Fatalf("the cache of messages for this peer should be nil")
		} else if tt.shouldCache {
			if m, ok := ms.(*lru.ARCCache); !ok {
				t.Fatalf("the cache of messages for this peer cannot be casted")
			} else if _, ok := m.Get(hash); !ok {
				t.Fatalf("the cache of messages for this peer cannot be found")
			}
		}
		// for self
		if _, ok := backend.selfRecentMessages.Get(hash); tt.shouldCache != ok {
			t.Fatalf("the cache of messages must be nil")
		}
	}
}

func TestProxyConsensusForwarding(t *testing.T) {
	_, backend := newBlockChain(1, true)
	backend.config.Proxy = true

	// generate one msg
	data := []byte("data1")
	bytes, err := rlp.EncodeToBytes(data)
	if err != nil {
		t.Fatalf("Error encoding consensus message bytes: %v", err)
	}

	msg := &istanbul.Message{
		Code:      istanbulConsensusMsg,
		Msg:       bytes,
		Address:   backend.Address(),
		Signature: []byte{},
	}

	// Test sending a message with no validator signature.
	// Should fail because proxy expects consensus messages from validators.
	payloadNoSig, _ := msg.Payload()
	err = backend.handleConsensusMsg(&MockPeer{}, payloadNoSig)
	if err != errNonValidatorMessage {
		t.Errorf("Expected error sending message from non validator")
	}

	// Test sending a message with a legitimate validator signature.
	// Should succeed now.
	msg.Sign(backend.Sign)
	payloadWithSig, _ := msg.Payload()
	if err = backend.handleConsensusMsg(&MockPeer{}, payloadWithSig); err != nil {
		t.Errorf("error %v", err)
	}
	// Set back to false for other tests
	backend.config.Proxy = false
}

func TestReadValidatorHandshakeMessage(t *testing.T) {
	_, backend := newBlockChain(2, true)

	peer := &MockPeer{
		Messages:     make(chan p2p.Msg, 1),
		NodeOverride: backend.p2pserver.Self(),
	}

	// Test an empty message being sent
	emptyMsg := &istanbul.Message{}
	emptyMsgPayload, err := emptyMsg.Payload()
	if err != nil {
		t.Errorf("Error getting payload of empty msg %v", err)
	}
	peer.Messages <- makeMsg(istanbulValidatorHandshakeMsg, emptyMsgPayload)
	isValidator, err := backend.readValidatorHandshakeMessage(peer)
	if err != nil {
		t.Errorf("Error from readValidatorHandshakeMessage %v", err)
	}
	if isValidator {
		t.Errorf("Expected isValidator to be false with empty istanbul message")
	}

	var validMsg *istanbul.Message
	// The enodeCertificate is not set synchronously. Wait until it's been set
	for i := 0; i < 10; i++ {
		// Test a legitimate message being sent
		validMsg, err = backend.retrieveEnodeCertificateMsg()
		if err != nil {
			t.Errorf("Error from retrieveEnodeCertificateMsg %v", err)
		}
		if validMsg != nil {
			break
		}
		time.Sleep(time.Duration(i) * time.Second)
	}
	if validMsg == nil {
		t.Errorf("enodeCertificate is nil")
	}

	validMsgPayload, err := validMsg.Payload()
	if err != nil {
		t.Errorf("Error getting payload of valid msg %v", err)
	}
	peer.Messages <- makeMsg(istanbulValidatorHandshakeMsg, validMsgPayload)

	block := backend.currentBlock()
	valSet := backend.getValidators(block.Number().Uint64(), block.Hash())
	// set backend to a different validator
	backend.address = valSet.GetByIndex(1).Address()

	isValidator, err = backend.readValidatorHandshakeMessage(peer)
	if err != nil {
		t.Errorf("Error from readValidatorHandshakeMessage with valid message %v", err)
	}
	if !isValidator {
		t.Errorf("Expected isValidator to be true with valid message")
	}
}

func makeMsg(msgcode uint64, data interface{}) p2p.Msg {
	size, r, _ := rlp.EncodeToReader(data)
	return p2p.Msg{Code: msgcode, Size: uint32(size), Payload: r}
}
