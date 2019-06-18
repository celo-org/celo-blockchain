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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

// This will create 'validator' type peers to all the valset validators, and disconnect from the
// peers that are not part of the valset.
// It will also disconnect all validator connections if this node is not a validator.
// Note that adding and removing validators are idempotent operations.  If the validator
// being added or removed is already added or removed, then a no-op will be done.
func (sb *Backend) RefreshValPeers(valset istanbul.ValidatorSet) {
	sb.logger.Trace("Called RefreshValPeers", "valset length", valset.Size())

	currentValPeers := sb.GetValidatorPeers()

	// Disconnect all validator peers if this node is not in the valset
	if _, val := valset.GetByAddress(sb.Address()); val == nil {
		for _, peerEnodeURL := range currentValPeers {
			sb.RemoveValidatorPeer(peerEnodeURL)
		}
	} else {
		// Add all of the valset entries as validator peers
		for _, val := range valset.List() {
			if valEnodeEntry, ok := sb.valEnodeTable.getUsingAddress(val.Address()); ok {
				sb.AddValidatorPeer(valEnodeEntry.enodeURL)
			}
		}

		// Remove the peers that are not in the valset
		for _, peerEnodeURL := range currentValPeers {
			if peerAddress, ok := sb.reverseValEnodeTable[peerEnodeURL]; ok {
				if _, src := valset.GetByAddress(peerAddress); src == nil {
					sb.RemoveValidatorPeer(peerEnodeURL)
				}
			}
		}
	}
}

// This function will retrieve the set of registered validators from the validator election
// smart contract.
// TODO (kevjue) - Right now it will return the active epoch valitors and itself in the set.
//                 Need to change to actually read from the smart contract.
func (sb *Backend) retrieveRegisteredValidators() map[common.Address]bool {
	returnMap := make(map[common.Address]bool)

	currentBlock := sb.currentBlock()
	valset := sb.getValidators(currentBlock.Number().Uint64(), currentBlock.Hash())

	for _, val := range valset.List() {
		returnMap[val.Address()] = true
	}

	returnMap[sb.Address()] = true

	return returnMap
}
