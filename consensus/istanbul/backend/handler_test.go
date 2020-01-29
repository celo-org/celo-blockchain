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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
)

type MockPeer struct{}

func (p *MockPeer) Send(msgcode uint64, data interface{}) error {
	return nil
}

func (p *MockPeer) Node() *enode.Node {
	return nil
}

func (p *MockPeer) Version() int {
	return 0
}

func TestIstanbulMessage(t *testing.T) {
	_, backend := newBlockChain(1, true)

	// generate one msg
	data := []byte("data1")
	msg := makeMsg(istanbulAnnounceMsg, data)
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
			ethMsgCode:  istanbulAnnounceMsg,
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
			ethMsgCode:  istanbulGetAnnouncesMsg,
			shouldCache: false,
		},
		{
			ethMsgCode:  istanbulGetAnnounceVersionsMsg,
			shouldCache: false,
		},
		{
			ethMsgCode:  istanbulAnnounceVersionsMsg,
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

		// 2. this message should be in cache only when ethMsgCode == istanbulAnnounceMsg
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

func makeMsg(msgcode uint64, data interface{}) p2p.Msg {
	size, r, _ := rlp.EncodeToReader(data)
	return p2p.Msg{Code: msgcode, Size: uint32(size), Payload: r}
}
