// Copyright 2017 The go-ethereum Authors
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

// Package consensus implements different Ethereum consensus engines.
package consensus

import (
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// Broadcaster defines the interface to enqueue blocks to fetcher, find peer
type Broadcaster interface {
	// FindPeers retrieves peers by addresses
	FindPeers(targets map[enode.ID]bool, purpose p2p.PurposeFlag) map[enode.ID]Peer
}

// P2PServer defines the interface for a p2p.server to get the local node's enode and to add/remove for static/trusted peers
type P2PServer interface {
	// Gets this node's enode
	Self() *enode.Node
	// AddPeer will add a peer to the p2p server instance
	AddPeer(node *enode.Node, purpose p2p.PurposeFlag)
	// RemovePeer will remove a peer from the p2p server instance
	RemovePeer(node *enode.Node, purpose p2p.PurposeFlag)
	// AddTrustedPeer will add a trusted peer to the p2p server instance
	AddTrustedPeer(node *enode.Node, purpose p2p.PurposeFlag)
	// RemoveTrustedPeer will remove a trusted peer from the p2p server instance
	RemoveTrustedPeer(node *enode.Node, purpose p2p.PurposeFlag)
}

// Peer defines the interface for a p2p.peer
type Peer interface {
	// Send sends the message to this peer
	Send(msgcode uint64, data interface{}) error
	// Node returns the peer's enode
	Node() *enode.Node
	// Version returns the peer's version
	Version() int
	// Blocks until a message is read directly from the peer.
	// This should only be used during a handshake.
	ReadMsg() (p2p.Msg, error)
	// Inbound returns if the peer connection is inbound
	Inbound() bool
	// PurposeIsSet returns if the peer has a purpose set
	PurposeIsSet(purpose p2p.PurposeFlag) bool
}
