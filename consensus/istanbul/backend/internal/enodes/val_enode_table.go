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

package enodes

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
)

var (
	// errOldAnnounceMessage is returned when the received announce message's block number is earlier
	// than a previous received message
	errOldAnnounceMessage = errors.New("old announce message")
)

// Entries for the valEnodeTable
type validatorEnode struct {
	enodeURL string
	view     *istanbul.View
}

func (ve *validatorEnode) String() string {
	return fmt.Sprintf("{enodeURL: %v, view: %v}", ve.enodeURL, ve.view)
}

// ValidatorEnodeTable represents a Map that can be accessed either
// by address or enode
type ValidatorEnodeTable struct {
	valEnodeTable        map[common.Address]*validatorEnode
	reverseValEnodeTable map[string]common.Address // enodeURL -> Address mapping
	valEnodeTableMu      *sync.RWMutex             // This mutex protects both valEnodeTable and reverseValEnodeTable, since they are modified at the same time
}

// NewValidatorEnodeTable creates a validator-enodes-table
func NewValidatorEnodeTable() *ValidatorEnodeTable {
	vet := &ValidatorEnodeTable{
		valEnodeTable:        make(map[common.Address]*validatorEnode),
		reverseValEnodeTable: make(map[string]common.Address),
		valEnodeTableMu:      new(sync.RWMutex),
	}

	return vet
}

func (vet *ValidatorEnodeTable) String() string {
	var b strings.Builder
	b.WriteString("ValEnodeTable:")
	for _, valEnode := range vet.valEnodeTable {
		fmt.Fprintf(&b, "%s\t", valEnode.String())
	}
	return b.String()
}

// GetEnodeURLFromAddress will return the enodeURL for a address if it's kwown
func (vet *ValidatorEnodeTable) GetEnodeURLFromAddress(address common.Address) (string, bool) {
	vet.valEnodeTableMu.RLock()
	defer vet.valEnodeTableMu.RUnlock()
	if enode, ok := vet.valEnodeTable[address]; ok {
		return enode.enodeURL, ok
	}
	return "", false
}

// GetAddressFromEnodeURL will return the address for a enodeURL if it's kwown
func (vet *ValidatorEnodeTable) GetAddressFromEnodeURL(enodeURL string) (common.Address, bool) {
	vet.valEnodeTableMu.RLock()
	defer vet.valEnodeTableMu.RUnlock()
	address, ok := vet.reverseValEnodeTable[enodeURL]
	return address, ok
}

// Upsert will update or insert a validator enode entry.  It will also do the associated set/remove to the validator connection.
func (vet *ValidatorEnodeTable) Upsert(remoteAddress common.Address, enodeURL string, view *istanbul.View) (string, error) {
	vet.valEnodeTableMu.Lock()
	defer vet.valEnodeTableMu.Unlock()

	var oldEnodeURL string
	if oldValEnode, ok := vet.valEnodeTable[remoteAddress]; ok {
		// If it is an old message, ignore it.
		if view.Cmp(oldValEnode.view) <= 0 {
			return oldEnodeURL, errOldAnnounceMessage
		}

		// If enodeURL changed, delete old entry in reverse table
		if enodeURL != oldValEnode.enodeURL {
			delete(vet.reverseValEnodeTable, oldValEnode.enodeURL)

			oldEnodeURL = oldValEnode.enodeURL
		}
		vet.valEnodeTable[remoteAddress] = &validatorEnode{enodeURL: enodeURL, view: view}
		vet.reverseValEnodeTable[enodeURL] = remoteAddress
		log.Trace("Updated an entry in the valEnodeTable", "address", remoteAddress, "ValidatorEnode", vet.valEnodeTable[remoteAddress].String())
	} else {
		vet.valEnodeTable[remoteAddress] = &validatorEnode{enodeURL: enodeURL, view: view}
		vet.reverseValEnodeTable[enodeURL] = remoteAddress
		log.Trace("Created an entry in the valEnodeTable", "address", remoteAddress, "ValidatorEnode", vet.valEnodeTable[remoteAddress].String())
	}

	return oldEnodeURL, nil
}

// RemoveEntry will remove an entry from the table
func (vet *ValidatorEnodeTable) RemoveEntry(address common.Address) {
	vet.valEnodeTableMu.Lock()
	defer vet.valEnodeTableMu.Unlock()
	vet.doRemoveEntry(address)
}

// PruneEntries will remove entries for all address not present in addressesToKeep
func (vet *ValidatorEnodeTable) PruneEntries(addressesToKeep map[common.Address]bool) {
	vet.valEnodeTableMu.Lock()
	defer vet.valEnodeTableMu.Unlock()

	for remoteAddress := range vet.valEnodeTable {
		if !addressesToKeep[remoteAddress] {
			vet.doRemoveEntry(remoteAddress)
		}
	}
}

func (vet *ValidatorEnodeTable) doRemoveEntry(address common.Address) {
	log.Trace("Deleting entry from the valEnodeTable and reverseValEnodeTable table", "address", address, "valEnodeEntry", vet.valEnodeTable[address].String())
	delete(vet.reverseValEnodeTable, vet.valEnodeTable[address].enodeURL)
	delete(vet.valEnodeTable, address)
}
