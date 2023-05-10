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

package freezer

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/internal/n"
	"github.com/celo-org/celo-blockchain/core/vm"
)

const (
	maxGasForIsFrozen uint64 = 20 * n.Thousand
)

var (
	isFrozenMethod = contracts.NewRegisteredContractMethod(config.FreezerRegistryId, abis.Freezer, "isFrozen", maxGasForIsFrozen)
)

func IsFrozen(vmRunner vm.EVMRunner, registryId common.Hash) (bool, error) {
	address, err := contracts.GetRegisteredAddress(vmRunner, registryId)
	if err != nil {
		return false, err
	}

	var isFrozen bool
	if err := isFrozenMethod.Query(vmRunner, &isFrozen, address); err != nil {
		return false, err
	}

	return isFrozen, nil
}
