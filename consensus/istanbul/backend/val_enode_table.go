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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"sync"
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
}

func newValidatorEnodeTable() *validatorEnodeTable {
	vet := &validatorEnodeTable{valEnodeTable: make(map[common.Address]*validatorEnode),
		reverseValEnodeTable: make(map[string]common.Address),
		valEnodeTableMu:      new(sync.RWMutex)}

	return vet
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

func (vet *validatorEnodeTable) String() string {
	outputString := "ValEnodeTable:"

	for _, valEnode := range vet.valEnodeTable {
		fmt.Sprintf(outputString, "%s\t%s", outputString, valEnode.String())
	}

	return outputString
}

// This function will test to see if there is an existing entry.  If there is one, it will check if it needs to update it,
// and do so accordingly.  If there is no existing entry, it will add an entry.  It does this all atomically.
func (vet *validatorEnodeTable) testAndSet(address common.Address, newValEnode *validatorEnode) (bool, error) {
	vet.valEnodeTableMu.Lock()
	defer vet.valEnodeTableMu.Unlock()

	enodeURLUpdated := false

	if oldValEnode := vet.getUsingAddress(address); oldValEnode != nil {
		// If it is an old message, ignore it.
		if newValEnode.view.Cmp(oldValEnode.view) <= 0 {
			return false, errOldAnnounceMessage
		} else {
			// Check if the valEnodeEntry has been changed
			if (newValEnode.enodeURL != oldValEnode.enodeURL) || (newValEnode.view.Cmp(oldValEnode.view) > 0) {
				if newValEnode.enodeURL != oldValEnode.enodeURL {
					enodeURLUpdated = true
					vet.reverseValEnodeTable[newValEnode.enodeURL] = address
				}

				vet.valEnodeTable[address] = newValEnode
				log.Trace("Updated an entry in the valEnodeTable", "address", address, "ValidatorEnode", vet.valEnodeTable[address].String())
			}
		}
	} else {
		vet.valEnodeTable[address] = newValEnode
		vet.reverseValEnodeTable[newValEnode.enodeURL] = address
		log.Trace("Created an entry in the valEnodeTable", "address", address, "ValidatorEnode", vet.valEnodeTable[address].String())
	}

	return enodeURLUpdated, nil
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
