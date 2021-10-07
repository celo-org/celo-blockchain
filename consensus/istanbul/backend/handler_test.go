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

package backend_test

import (
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/rlp"
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
	chain, backend := NewBlockChain(1, true)
	defer chain.Stop()

	// generate one msg
	data := []byte("data1")
	msg := makeMsg(istanbul.QueryEnodeMsg, data)
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
			ethMsgCode:  istanbul.ConsensusMsg,
			shouldCache: false,
		},
		{
			ethMsgCode:  istanbul.QueryEnodeMsg,
			shouldCache: true,
		},
		{
			ethMsgCode:  istanbul.ValEnodesShareMsg,
			shouldCache: false,
		},
		{
			ethMsgCode:  istanbul.FwdMsg,
			shouldCache: false,
		},
		{
			ethMsgCode:  istanbul.VersionCertificatesMsg,
			shouldCache: true,
		},
		{
			ethMsgCode:  istanbul.EnodeCertificateMsg,
			shouldCache: false,
		},
		{
			ethMsgCode:  istanbul.ValidatorHandshakeMsg,
			shouldCache: false,
		},
	}

	for _, tt := range tests {
		chain, backend := NewBlockChain(1, true)

		// generate a msg that is not an Announce
		data := []byte("data1")
		msg := makeMsg(tt.ethMsgCode, data)
		addr := common.BytesToAddress([]byte("address"))

		// 1. this message should not be in cache
		// for peers
		if backend.gossipCache.CheckIfMessageProcessedByPeer(addr, data) {
			t.Fatalf("the cache of messages for this peer should be nil")
		}

		// for self
		if backend.gossipCache.CheckIfMessageProcessedBySelf(data) {
			t.Fatalf("the cache of messages should be nil")
		}

		// 2. this message should be in cache only when ethMsgCode == istanbulQueryEnodeMsg || ethMsgCode == istanbulVersionCertificatesMsg
		_, err := backend.HandleMsg(addr, msg, &MockPeer{})
		if err != nil {
			t.Fatalf("handle message failed: %v", err)
		}

		// Sleep for a bit, since some of the messages are handled in a different thread
		time.Sleep(10 * time.Second)

		// for peers
		if ok := backend.gossipCache.CheckIfMessageProcessedByPeer(addr, data); tt.shouldCache != ok {
			t.Fatalf("the cache of messages for this peer should be nil")
		}
		// for self
		if ok := backend.gossipCache.CheckIfMessageProcessedBySelf(data); tt.shouldCache != ok {
			t.Fatalf("the cache of messages must be nil")
		}

		chain.Stop()
	}
}

func TestReadValidatorHandshakeMessage(t *testing.T) {
	chain, backend := NewBlockChain(2, true)
	defer chain.Stop()

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
	peer.Messages <- makeMsg(istanbul.ValidatorHandshakeMsg, emptyMsgPayload)
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
		enodeCertMsg := backend.RetrieveEnodeCertificateMsgMap()[backend.SelfNode().ID()]
		if enodeCertMsg != nil {
			validMsg = enodeCertMsg.Msg
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
	peer.Messages <- makeMsg(istanbul.ValidatorHandshakeMsg, validMsgPayload)

	block := backend.currentBlock()
	valSet := backend.getValidators(block.Number().Uint64(), block.Hash())
	// set backend to a different validator
	backend.wallets().Ecdsa.Address = valSet.GetByIndex(1).Address()

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
