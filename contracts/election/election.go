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
	"sort"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/internal/n"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

const (
	maxGasForGetElectableValidators               uint64 = 100 * n.Thousand
	maxGasForGetEligibleValidatorGroupsVoteTotals uint64 = 1 * n.Million
	maxGasForElectValidators                      uint64 = 50 * n.Million
	maxGasForElectNValidatorSigners               uint64 = 50 * n.Million
	maxGasForGetGroupEpochRewards                 uint64 = 500 * n.Thousand
	maxGasForDistributeEpochRewards               uint64 = 1 * n.Million
)

var (
	electValidatorSignersMethod                   = contracts.NewRegisteredContractMethod(config.ElectionRegistryId, abis.Elections, "electValidatorSigners", maxGasForElectValidators)
	getElectableValidatorsMethod                  = contracts.NewRegisteredContractMethod(config.ElectionRegistryId, abis.Elections, "getElectableValidators", maxGasForGetElectableValidators)
	electNValidatorSignersMethod                  = contracts.NewRegisteredContractMethod(config.ElectionRegistryId, abis.Elections, "electNValidatorSigners", maxGasForElectNValidatorSigners)
	getTotalVotesForEligibleValidatorGroupsMethod = contracts.NewRegisteredContractMethod(config.ElectionRegistryId, abis.Elections, "getTotalVotesForEligibleValidatorGroups", maxGasForGetEligibleValidatorGroupsVoteTotals)
	getGroupEpochRewardsMethod                    = contracts.NewRegisteredContractMethod(config.ElectionRegistryId, abis.Elections, "getGroupEpochRewards", maxGasForGetGroupEpochRewards)
	distributeEpochRewardsMethod                  = contracts.NewRegisteredContractMethod(config.ElectionRegistryId, abis.Elections, "distributeEpochRewards", maxGasForDistributeEpochRewards)
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

type voteTotal struct {
	Group common.Address
	Value *big.Int
}

func getTotalVotesForEligibleValidatorGroups(vmRunner vm.EVMRunner) ([]voteTotal, error) {
	var groups []common.Address
	var values []*big.Int
	err := getTotalVotesForEligibleValidatorGroupsMethod.Query(vmRunner, &[]interface{}{&groups, &values})
	if err != nil {
		return nil, err
	}

	voteTotals := make([]voteTotal, len(groups))
	for i, group := range groups {
		log.Trace("Got group vote total", "group", group, "value", values[i])
		voteTotals[i].Group = group
		voteTotals[i].Value = values[i]
	}
	return voteTotals, err
}

func getGroupEpochRewards(vmRunner vm.EVMRunner, group common.Address, maxRewards *big.Int, uptimes []*big.Int) (*big.Int, error) {
	var groupEpochRewards *big.Int
	err := getGroupEpochRewardsMethod.Query(vmRunner, &groupEpochRewards, group, maxRewards, uptimes)
	if err != nil {
		return nil, err
	}
	return groupEpochRewards, nil
}

func DistributeEpochRewards(vmRunner vm.EVMRunner, groups []common.Address, maxTotalRewards *big.Int, uptimes map[common.Address][]*big.Int) (*big.Int, error) {
	totalRewards := big.NewInt(0)
	voteTotals, err := getTotalVotesForEligibleValidatorGroups(vmRunner)
	if err != nil {
		return totalRewards, err
	}

	rewards := make([]*big.Int, len(groups))
	for i, group := range groups {
		reward, err := getGroupEpochRewards(vmRunner, group, maxTotalRewards, uptimes[group])
		if err != nil {
			return totalRewards, err
		}
		rewards[i] = reward
		log.Debug("Reward for group voters", "reward", reward, "group", group.String())
	}

	for i, group := range groups {
		reward := rewards[i]
		for _, voteTotal := range voteTotals {
			if voteTotal.Group == group {
				voteTotal.Value.Add(voteTotal.Value, reward)
				break
			}
		}

		// Sorting in descending order is necessary to match the order on-chain.
		// TODO: We could make this more efficient by only moving the newly vote member.
		sort.SliceStable(voteTotals, func(j, k int) bool {
			return voteTotals[j].Value.Cmp(voteTotals[k].Value) > 0
		})

		lesser := common.ZeroAddress
		greater := common.ZeroAddress
		for j, voteTotal := range voteTotals {
			if voteTotal.Group == group {
				if j > 0 {
					greater = voteTotals[j-1].Group
				}
				if j+1 < len(voteTotals) {
					lesser = voteTotals[j+1].Group
				}
				break
			}
		}
		err := distributeEpochRewardsMethod.Execute(vmRunner, nil, common.Big0, group, reward, lesser, greater)
		if err != nil {
			return totalRewards, err
		}
		totalRewards.Add(totalRewards, reward)
	}
	return totalRewards, nil
}
