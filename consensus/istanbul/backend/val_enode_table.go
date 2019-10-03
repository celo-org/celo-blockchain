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
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
)

// Entries for the valEnodeTable
type validatorEnode struct {
	enodeURL string
	view     *istanbul.View
}

func (ve *validatorEnode) String() string {
	return fmt.Sprintf("{enodeURL: %v, view: %v}", ve.enodeURL, ve.view)
}

type validatorEnodeTable struct {
	valEnodeTable        map[common.Address]*validatorEnode
	reverseValEnodeTable map[string]common.Address // EnodeURL -> Address mapping
	valEnodeTableMu      *sync.RWMutex             // This mutex protects both valEnodeTable and reverseValEnodeTable, since they are modified at the same time

	// This is used to set and remove the enodeURL validator connections.  Those adds and removes needs to be synchronized with the
	// updates/inserts/removes to their associated entries in the valEnodeTable.
	addValidatorPeer    func(enodeURL string)
	removeValidatorPeer func(enodeURL string)

	address common.Address // Address of the local node
}

func newValidatorEnodeTable(addValidatorPeer func(enodeURL string), removeValidatorPeer func(enodeURL string)) *validatorEnodeTable {
	vet := &validatorEnodeTable{valEnodeTable: make(map[common.Address]*validatorEnode),
		reverseValEnodeTable: make(map[string]common.Address),
		valEnodeTableMu:      new(sync.RWMutex),
		addValidatorPeer:     addValidatorPeer,
		removeValidatorPeer:  removeValidatorPeer}

	return vet
}

func (vet *validatorEnodeTable) String() string {
	outputString := "ValEnodeTable:"

	for _, valEnode := range vet.valEnodeTable {
		outputString = fmt.Sprintf("%s\t%s", outputString, valEnode.String())
	}

	return outputString
}

func (vet *validatorEnodeTable) getUsingAddress(address common.Address) *validatorEnode {
	return vet.valEnodeTable[address]
}

func (vet *validatorEnodeTable) getUsingEnodeURL(enodeURL string) (common.Address, *validatorEnode) {
	if address, ok := vet.reverseValEnodeTable[enodeURL]; ok {
		return address, vet.getUsingAddress(address)
	} else {
		return common.ZeroAddress, nil
	}
}

// This function will update or insert a validator enode entry.  It will also do the associated set/remove to the validator connection.
func (vet *validatorEnodeTable) upsert(remoteAddress common.Address, newValEnode *validatorEnode, valSet istanbul.ValidatorSet, localAddress common.Address, isProxied bool, isSentry bool) error {
	vet.valEnodeTableMu.Lock()
	defer vet.valEnodeTableMu.Unlock()

	if oldValEnode := vet.getUsingAddress(remoteAddress); oldValEnode != nil {
		// If it is an old message, ignore it.
		if newValEnode.view.Cmp(oldValEnode.view) <= 0 {
			return errOldAnnounceMessage
		} else {
			// Check if the valEnodeEntry has been changed
			if (newValEnode.enodeURL != oldValEnode.enodeURL) || (newValEnode.view.Cmp(oldValEnode.view) > 0) {
				if newValEnode.enodeURL != oldValEnode.enodeURL {
					vet.reverseValEnodeTable[newValEnode.enodeURL] = remoteAddress
					delete(vet.reverseValEnodeTable, oldValEnode.enodeURL)
					// Disconnect from the peer
					vet.removeValidatorPeer(oldValEnode.enodeURL)
				}
				vet.valEnodeTable[remoteAddress] = newValEnode
				log.Trace("Updated an entry in the valEnodeTable", "address", remoteAddress, "ValidatorEnode", vet.valEnodeTable[remoteAddress].String())
			}
		}
	} else {
		vet.valEnodeTable[remoteAddress] = newValEnode
		vet.reverseValEnodeTable[newValEnode.enodeURL] = remoteAddress
		log.Trace("Created an entry in the valEnodeTable", "address", remoteAddress, "ValidatorEnode", vet.valEnodeTable[remoteAddress].String())
	}

	// Connect to the remote peer if it's part of the current epoch's valset and
	// if this node is also part of the current epoch's valset
	if _, remoteNode := valSet.GetByAddress(remoteAddress); remoteNode != nil {
		// TODO: remove `forcePeer` once we can tell if the current node is a proxy
		// for a validator in the current valSet
		if _, localNode := valSet.GetByAddress(localAddress); (localNode != nil && !isProxied) || isSentry {
			vet.addValidatorPeer(newValEnode.enodeURL)
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
			delete(vet.reverseValEnodeTable, vet.valEnodeTable[remoteAddress].enodeURL)
			delete(vet.valEnodeTable, remoteAddress)
		}
	}
}

func (vet *validatorEnodeTable) refreshValPeers(valSet istanbul.ValidatorSet, currentValPeers []string) {
	vet.valEnodeTableMu.RLock()
	defer vet.valEnodeTableMu.RUnlock()

	// Add all of the valSet entries as validator peers
	for _, val := range valSet.List() {
		if valEnodeEntry := vet.getUsingAddress(val.Address()); valEnodeEntry != nil {
			vet.addValidatorPeer(valEnodeEntry.enodeURL)
		}
	}

	// Remove the peers that are not in the valset
	for _, peerEnodeURL := range currentValPeers {
		if peerAddress, ok := vet.reverseValEnodeTable[peerEnodeURL]; ok {
			if _, src := valSet.GetByAddress(peerAddress); src == nil {
				vet.removeValidatorPeer(peerEnodeURL)
			}
		}
	}
}
