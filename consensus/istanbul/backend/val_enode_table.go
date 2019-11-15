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

package backend

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// Entries for the valEnodeTable
type validatorEnode struct {
	node *enode.Node
	view *istanbul.View
	peer consensus.Peer
}

func (ve *validatorEnode) String() string {
	return fmt.Sprintf("{node: %s, view: %v}", ve.node, ve.view)
}

type validatorEnodeTable struct {
	valEnodeTable        map[common.Address]*validatorEnode
	reverseValEnodeTable map[enode.ID]common.Address // node ID -> Address mapping
	valEnodeTableMu      *sync.RWMutex               // This mutex protects both valEnodeTable and reverseValEnodeTable, since they are modified at the same time
	p2pserver            consensus.P2PServer
}

func newValidatorEnodeTable() *validatorEnodeTable {
	vet := &validatorEnodeTable{valEnodeTable: make(map[common.Address]*validatorEnode),
		reverseValEnodeTable: make(map[enode.ID]common.Address),
		valEnodeTableMu:      new(sync.RWMutex),
	}

	return vet
}

func (vet *validatorEnodeTable) String() string {
	outputString := "ValEnodeTable:"

	for valAddr, valEnode := range vet.valEnodeTable {
		outputString = fmt.Sprintf("%s\t%s %s", outputString, valAddr, valEnode.String())
	}

	return outputString
}

func (vet *validatorEnodeTable) getUsingAddress(address common.Address) *validatorEnode {
	return vet.valEnodeTable[address]
}

func (vet *validatorEnodeTable) getUsingNodeID(nodeID enode.ID) (common.Address, *validatorEnode) {
	if address, ok := vet.reverseValEnodeTable[nodeID]; ok {
		return address, vet.getUsingAddress(address)
	} else {
		return common.ZeroAddress, nil
	}
}

// Locks the valEnodeTable mutex and updates or inserts a validator enode entry by calling upsertNonLocking
func (vet *validatorEnodeTable) upsert(remoteAddress common.Address, newValEnodeURL string, newValView *istanbul.View, valSet istanbul.ValidatorSet, localAddress common.Address, isProxied bool, isProxy bool) error {
	vet.valEnodeTableMu.Lock()
	defer vet.valEnodeTableMu.Unlock()

	return vet.upsertNonLocking(remoteAddress, newValEnodeURL, newValView, valSet, localAddress, isProxied, isProxy)
}

// This function will update or insert a validator enode entry.  It will also do the associated set/remove to the validator connection.
// This does not lock the table mutex
func (vet *validatorEnodeTable) upsertNonLocking(remoteAddress common.Address, newValEnodeURL string, newValView *istanbul.View, valSet istanbul.ValidatorSet, localValAddress common.Address, isProxied bool, isProxy bool) error {
	newValNode, err := enode.ParseV4(newValEnodeURL)
	if err != nil {
		log.Error("Invalid Enode", "newValEnodeURL", newValEnodeURL, "err", err)
		return err
	}

	if currentValEntry := vet.getUsingAddress(remoteAddress); currentValEntry != nil {
		// If it is an old message, ignore it.
		if newValView.Cmp(currentValEntry.view) <= 0 {
			return errOldAnnounceMessage
		} else {
			// Check if the validator's enode URL has been changed
			if (newValEnodeURL != currentValEntry.node.String()) || (newValView.Cmp(currentValEntry.view) > 0) {
				if newValEnodeURL != currentValEntry.node.String() {
					vet.reverseValEnodeTable[newValNode.ID()] = remoteAddress
					delete(vet.reverseValEnodeTable, currentValEntry.node.ID())
					// Disconnect from the peer
					vet.p2pserver.RemovePeer(currentValEntry.node, "validator")
					vet.p2pserver.RemoveTrustedPeer(currentValEntry.node, "validator")
				}
				vet.valEnodeTable[remoteAddress] = &validatorEnode{node: newValNode, view: newValView}
				log.Trace("Updated an entry in the valEnodeTable", "address", remoteAddress, "ValidatorEnode", vet.valEnodeTable[remoteAddress].String())
			}
		}
	} else {
		vet.valEnodeTable[remoteAddress] = &validatorEnode{node: newValNode, view: newValView}
		vet.reverseValEnodeTable[newValNode.ID()] = remoteAddress
		log.Trace("Created an entry in the valEnodeTable", "address", remoteAddress, "ValidatorEnode", vet.valEnodeTable[remoteAddress].String())
	}

	// Connect to the remote peer if it's part of the current epoch's valset and
	// if this node is also part of the current epoch's valset and
	// the remoteAddress does not equal the localValAddress (no need for a proxy to establish a validator
	// connection to it's validator).
	// Proxied validators are not responsible for connecting directly to other validators.
	if !isProxied || isProxy {
		if _, remoteVal := valSet.GetByAddress(remoteAddress); remoteVal != nil {
			// This node should connect only if it's a standalone validator or a proxy of a validator
			if _, localVal := valSet.GetByAddress(localValAddress); localVal != nil {
				vet.p2pserver.AddPeer(newValNode, "validator")
				vet.p2pserver.AddTrustedPeer(newValNode, "validator")
			}
		}
	}

	return nil
}

func (vet *validatorEnodeTable) pruneEntries(addressesToKeep map[common.Address]bool) {
	vet.valEnodeTableMu.Lock()
	defer vet.valEnodeTableMu.Unlock()

	for remoteAddress := range vet.valEnodeTable {
		if !addressesToKeep[remoteAddress] {
			log.Trace("Deleting entry from the valEnodeTable and reverseValEnodeTable table", "address", remoteAddress, "valEnodeEntry", vet.valEnodeTable[remoteAddress].String())
			delete(vet.reverseValEnodeTable, vet.valEnodeTable[remoteAddress].node.ID())
			delete(vet.valEnodeTable, remoteAddress)
		}
	}
}

func (vet *validatorEnodeTable) refreshValPeers(valSet istanbul.ValidatorSet, currentValPeers map[enode.ID]consensus.Peer) {
	vet.valEnodeTableMu.RLock()
	defer vet.valEnodeTableMu.RUnlock()

	// Add all of the valSet entries as validator peers
	for _, val := range valSet.List() {
		if valEnodeEntry := vet.getUsingAddress(val.Address()); valEnodeEntry != nil {
			vet.p2pserver.AddPeer(valEnodeEntry.node, "validator")
			vet.p2pserver.AddTrustedPeer(valEnodeEntry.node, "validator")
		}
	}

	// Remove the peers that are not in the valset
	for id, valPeer := range currentValPeers {
		if peerAddress, ok := vet.reverseValEnodeTable[id]; ok {
			if _, src := valSet.GetByAddress(peerAddress); src == nil {
				vet.p2pserver.RemovePeer(valPeer.Node(), "validator")
				vet.p2pserver.RemoveTrustedPeer(valPeer.Node(), "validator")
			}
		}
	}
}
