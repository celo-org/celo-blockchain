// Copyright 2017 The Celo Authors
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

package consensustest

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type MockBroadcaster struct{}

func (b *MockBroadcaster) Enqueue(id string, block *types.Block) {
}

func (b *MockBroadcaster) FindPeers(targets map[enode.ID]bool, purpose p2p.PurposeFlag) map[enode.ID]consensus.Peer {
	return make(map[enode.ID]consensus.Peer)
}

type MockP2PServer struct {
	Node *enode.Node
}

func (serv *MockP2PServer) Self() *enode.Node {
	return serv.Node
}

func (serv *MockP2PServer) AddPeer(node *enode.Node, purpose p2p.PurposeFlag) {}

func (serv *MockP2PServer) RemovePeer(node *enode.Node, purpose p2p.PurposeFlag) {}

func (serv *MockP2PServer) AddTrustedPeer(node *enode.Node, purpose p2p.PurposeFlag) {}

func (serv *MockP2PServer) RemoveTrustedPeer(node *enode.Node, purpose p2p.PurposeFlag) {}
