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
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type validatorPeerHandler struct {
	sb *Backend
}

// Returns whether this node should maintain validator connections
// Only proxies and non proxied validators need to connect maintain validator connections
func (vph *validatorPeerHandler) MaintainValConnections() bool {
	return vph.sb.config.Proxy || (vph.sb.coreStarted && !vph.sb.config.Proxied)
}

func (vph *validatorPeerHandler) AddValidatorPeer(node *enode.Node, address common.Address) {
	if !vph.MaintainValConnections() {
		return
	}

	// Connect to the remote peer if it's part of the current epoch's valset and
	// if this node is also part of the current epoch's valset

	block := vph.sb.currentBlock()
	valSet := vph.sb.getValidators(block.Number().Uint64(), block.Hash())
	if valSet.ContainsByAddress(address) && valSet.ContainsByAddress(vph.sb.ValidatorAddress()) {
		vph.sb.p2pserver.AddPeer(node, p2p.ValidatorPurpose)
		vph.sb.p2pserver.AddTrustedPeer(node, p2p.ValidatorPurpose)
	}
}

func (vph *validatorPeerHandler) RemoveValidatorPeer(node *enode.Node) {
	vph.sb.p2pserver.RemovePeer(node, p2p.ValidatorPurpose)
	vph.sb.p2pserver.RemoveTrustedPeer(node, p2p.ValidatorPurpose)
}

func (vph *validatorPeerHandler) ReplaceValidatorPeers(newNodes []*enode.Node) {
	nodeIDSet := make(map[enode.ID]bool)
	for _, node := range newNodes {
		nodeIDSet[node.ID()] = true
	}

	// Remove old Validator Peers
	for existingPeerID, existingPeer := range vph.sb.broadcaster.FindPeers(nodeIDSet, p2p.ValidatorPurpose) {
		if !nodeIDSet[existingPeerID] {
			vph.RemoveValidatorPeer(existingPeer.Node())
		}
	}

	if vph.MaintainValConnections() {
		// Add new Validator Peers (adds all the nodes in newNodes.  Note that add is noOp on already existent ones)
		for _, newNode := range newNodes {
			vph.sb.p2pserver.AddPeer(newNode, p2p.ValidatorPurpose)
			vph.sb.p2pserver.AddTrustedPeer(newNode, p2p.ValidatorPurpose)
		}
	}
}

func (vph *validatorPeerHandler) ClearValidatorPeers() {
	for _, peer := range vph.sb.broadcaster.FindPeers(nil, p2p.ValidatorPurpose) {
		vph.sb.p2pserver.RemovePeer(peer.Node(), p2p.ValidatorPurpose)
		vph.sb.p2pserver.RemoveTrustedPeer(peer.Node(), p2p.ValidatorPurpose)
	}
}
