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
)

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
