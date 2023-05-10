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
package election

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/internal/n"
	"github.com/celo-org/celo-blockchain/core/vm"
)

const (
	maxGasForGetElectableValidators uint64 = 100 * n.Thousand
	maxGasForElectValidators        uint64 = 50 * n.Million
	maxGasForElectNValidatorSigners uint64 = 50 * n.Million
)

var (
	electValidatorSignersMethod  = contracts.NewRegisteredContractMethod(config.ElectionRegistryId, abis.Elections, "electValidatorSigners", maxGasForElectValidators)
	getElectableValidatorsMethod = contracts.NewRegisteredContractMethod(config.ElectionRegistryId, abis.Elections, "getElectableValidators", maxGasForGetElectableValidators)
	electNValidatorSignersMethod = contracts.NewRegisteredContractMethod(config.ElectionRegistryId, abis.Elections, "electNValidatorSigners", maxGasForElectNValidatorSigners)
)

func GetElectedValidators(vmRunner vm.EVMRunner) ([]common.Address, error) {
	// Get the new epoch's validator set
	var newValSet []common.Address
	err := electValidatorSignersMethod.Query(vmRunner, &newValSet)

	if err != nil {
		return nil, err
	}
	return newValSet, nil
}

func ElectNValidatorSigners(vmRunner vm.EVMRunner, additionalAboveMaxElectable int64) ([]common.Address, error) {
	// Get the electable min and max
	var minElectableValidators *big.Int
	var maxElectableValidators *big.Int
	err := getElectableValidatorsMethod.Query(vmRunner, &[]interface{}{&minElectableValidators, &maxElectableValidators})
	if err != nil {
		return nil, err
	}

	// Run the validator election for up to maxElectable + getTotalVotesForEligibleValidatorGroup
	var electedValidators []common.Address
	err = electNValidatorSignersMethod.Query(vmRunner, &electedValidators, minElectableValidators, maxElectableValidators.Add(maxElectableValidators, big.NewInt(additionalAboveMaxElectable)))
	if err != nil {
		return nil, err
	}

	return electedValidators, nil

}
