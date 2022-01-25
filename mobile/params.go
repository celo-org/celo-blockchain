// Copyright 2016 The go-ethereum Authors
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

// Contains all the wrappers from the params package.

package geth

import (
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/params"
)

// DefaultBootnodes returns the enode URLs of the P2P bootstrap nodes operated
// by cLabs running the V5 discovery protocol.
func DefaultBootnodes(networkId uint64) *Enodes {
	// Set the default bootnode urls from the network we are on.
	// Don't default to mainnet bootnodes, as this is likely incorrect behavior.
	var urls []string
	switch networkId {
	case params.MainnetNetworkId:
		urls = params.MainnetBootnodes
	case params.AlfajoresNetworkId:
		urls = params.AlfajoresBootnodes
	case params.BaklavaNetworkId:
		urls = params.BaklavaBootnodes
	}

	// Parse the given bootnode urls.
	nodes := &Enodes{nodes: make([]*enode.Node, len(urls))}
	for i, url := range urls {
		var err error
		nodes.nodes[i], err = enode.Parse(enode.ValidSchemes, url)
		if err != nil {
			panic("invalid node URL: " + err.Error())
		}
	}
	return nodes
}
