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
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

type validatorPeerHandler struct {
	sb *Backend

	threadRunning   bool
	threadRunningMu sync.RWMutex
	threadWg        *sync.WaitGroup
	threadQuit      chan struct{}
}

func newVPH(sb *Backend) *validatorPeerHandler {
	return &validatorPeerHandler{
		sb:         sb,
		threadWg:   new(sync.WaitGroup),
		threadQuit: make(chan struct{}),
	}
}

func (vph *validatorPeerHandler) startThread() error {
	vph.threadRunningMu.Lock()
	defer vph.threadRunningMu.Unlock()
	if vph.threadRunning {
		return istanbul.ErrStartedVPHThread
	}

	vph.threadRunning = true
	go vph.thread()

	return nil
}

func (vph *validatorPeerHandler) stopThread() error {
	vph.threadRunningMu.Lock()
	defer vph.threadRunningMu.Unlock()

	if !vph.threadRunning {
		return istanbul.ErrStoppedVPHThread
	}

	vph.threadQuit <- struct{}{}
	vph.threadWg.Wait()

	vph.threadRunning = false
	return nil
}

func (vph *validatorPeerHandler) thread() {
	vph.threadWg.Add(1)
	defer vph.threadWg.Done()

	refreshValidatorPeersTicker := time.NewTicker(1 * time.Minute)

	refreshValPeersFunc := func() {
		if vph.MaintainValConnections() {
			if err := vph.sb.RefreshValPeers(); err != nil {
				vph.sb.logger.Warn("Error refreshing validator peers", "err", err)
			}
		}
	}

	refreshValPeersFunc()
	// Every 5 minute, check to see if we need to refresh the validator peers
	for {
		select {
		case <-refreshValidatorPeersTicker.C:
			refreshValPeersFunc()

		case <-vph.threadQuit:
			refreshValidatorPeersTicker.Stop()
			return
		}
	}
}

// Returns whether this node should maintain validator connections
// Only proxies and non proxied validators need to connect maintain validator connections
func (vph *validatorPeerHandler) MaintainValConnections() bool {
	return vph.sb.IsProxy() || (vph.sb.IsValidator() && !vph.sb.IsProxiedValidator())
}

func (vph *validatorPeerHandler) AddValidatorPeer(node *enode.Node, address common.Address) {
	// addr := vph.sb.Address()
	// fmt.Printf("Addr: %s adding validator peer: %s\n%s", hexutil.Encode(addr[:2]), hexutil.Encode(address[:2]), string(debug.Stack()))
	if !vph.MaintainValConnections() {
		println("returning early")
		return
	}

	// Connect to the remote peer if it's part of the current epoch's valset and
	// if this node is also part of the current epoch's valset
	valConnSet, err := vph.sb.RetrieveValidatorConnSet()
	if err != nil {
		vph.sb.logger.Error("Error in retrieving val conn set in AddValidatorPeer", "err", err)
		return
	}
	if valConnSet[address] && valConnSet[vph.sb.ValidatorAddress()] {
		vph.sb.p2pserver.AddPeer(node, p2p.ValidatorPurpose)
		vph.sb.p2pserver.AddTrustedPeer(node, p2p.ValidatorPurpose)
	}
}

func (vph *validatorPeerHandler) RemoveValidatorPeer(node *enode.Node) {
	addr := vph.sb.Address()
	peerAddr := crypto.PubkeyToAddress(*node.Pubkey())
	fmt.Printf("Addr: %s removing peer: %s\n%s", hexutil.Encode(addr[:2]), hexutil.Encode(peerAddr[:2]), string(debug.Stack()))
	vph.sb.p2pserver.RemovePeer(node, p2p.ValidatorPurpose)
	vph.sb.p2pserver.RemoveTrustedPeer(node, p2p.ValidatorPurpose)
}

func (vph *validatorPeerHandler) ReplaceValidatorPeers(newNodes []*enode.Node) {
	nodeIDSet := make(map[enode.ID]bool)
	for _, node := range newNodes {
		nodeIDSet[node.ID()] = true
	}

	// Remove old Validator Peers
	for existingPeerID, existingPeer := range vph.sb.broadcaster.FindPeers(nil, p2p.ValidatorPurpose) {
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

func (sb *Backend) AddPeer(node *enode.Node, purpose p2p.PurposeFlag) {
	sb.p2pserver.AddPeer(node, purpose)
}

func (sb *Backend) RemovePeer(node *enode.Node, purpose p2p.PurposeFlag) {
	sb.p2pserver.RemovePeer(node, purpose)
}
