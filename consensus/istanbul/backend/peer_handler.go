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

package backend

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type validatorPeerHandler struct {
	sb *Backend
}

func (vpl *validatorPeerHandler) AddValidatorPeer(node *enode.Node, address common.Address) {
	if !vpl.sb.MaintainValConnections() {
		return
	}

	// Connect to the remote peer if it's part of the current epoch's valset and
	// if this node is also part of the current epoch's valset

	block := vpl.sb.currentBlock()
	valSet := vpl.sb.getValidators(block.Number().Uint64(), block.Hash())
	if valSet.ContainsByAddress(address) && valSet.ContainsByAddress(vpl.sb.ValidatorAddress()) {
		vpl.sb.p2pserver.AddPeer(node, "validator")
		vpl.sb.p2pserver.AddTrustedPeer(node, "validator")
	}
}

func (vpl *validatorPeerHandler) RemoveValidatorPeer(node *enode.Node) {
	vpl.sb.p2pserver.RemovePeer(node, "validator")
	vpl.sb.p2pserver.RemoveTrustedPeer(node, "validator")
}

func (vpl *validatorPeerHandler) ReplaceValidatorPeers(newNodes []*enode.Node) {
	nodeIDSet := make(map[enode.ID]bool)
	for _, node := range newNodes {
		nodeIDSet[node.ID()] = true
	}

	// Remove old Validator Peers
	for existingPeerID, existingPeer := range vpl.sb.broadcaster.FindPeers(nodeIDSet, "validator") {
		if !nodeIDSet[existingPeerID] {
			vpl.RemoveValidatorPeer(existingPeer.Node())
		}
	}

	if vpl.sb.MaintainValConnections() {
		// Add new Validator Peers (adds all the nodes in newNodes.  Note that add is noOp on already existent ones)
		for _, newNode := range newNodes {
			vpl.sb.p2pserver.AddPeer(newNode, "validator")
			vpl.sb.p2pserver.AddTrustedPeer(newNode, "validator")
		}
	}
}

func (vpl *validatorPeerHandler) ClearValidatorPeers() {
	for _, peer := range vpl.sb.broadcaster.FindPeers(nil, "validator") {
		vpl.sb.p2pserver.RemovePeer(peer.Node(), "validator")
		vpl.sb.p2pserver.RemoveTrustedPeer(peer.Node(), "validator")
	}
}
