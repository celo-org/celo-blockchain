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
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Election.json
const electionABIString string = `[
  {"constant": true,
              "inputs": [],
        "name": "electValidators",
        "outputs": [
       {
            "name": "",
      "type": "address[]"
       }
        ],
        "payable": false,
        "stateMutability": "view",
        "type": "function"
       },
			     {
      "constant": true,
      "inputs": [],
      "name": "getEligibleValidatorGroupsVoteTotals",
      "outputs": [
        {
          "name": "groups",
          "type": "address[]"
        },
        {
          "name": "values",
          "type": "uint256[]"
        }
      ],
      "payable": false,
      "stateMutability": "view",
      "type": "function"
    },
		    {
      "constant": false,
      "inputs": [
        {
          "name": "group",
          "type": "address"
        },
        {
          "name": "value",
          "type": "uint256"
        },
        {
          "name": "lesser",
          "type": "address"
        },
        {
          "name": "greater",
          "type": "address"
        }
      ],
      "name": "distributeEpochRewards",
      "outputs": [],
      "payable": false,
      "stateMutability": "nonpayable",
      "type": "function"
    }
]`

var electionABI, _ = abi.JSON(strings.NewReader(electionABIString))

func GetElectedValidators(header *types.Header, state vm.StateDB) ([]common.Address, error) {
	var newValSet []common.Address
	// Get the new epoch's validator set
	_, err := contract_comm.MakeStaticCall(params.ElectionRegistryId, electionABI, "electValidators", []interface{}{}, &newValSet, params.MaxGasForElectValidators, header, state)
	if err != nil {
		return nil, err
	}
	return newValSet, nil
}

type voteTotal struct {
	Group common.Address
	Value *big.Int
}

func getEligibleValidatorGroupsVoteTotals(header *types.Header, state vm.StateDB) ([]voteTotal, error) {
	var groups []common.Address
	var values []*big.Int
	_, err := contract_comm.MakeStaticCall(params.ElectionRegistryId, electionABI, "getEligibleValidatorGroupsVoteTotals", []interface{}{}, &[]interface{}{&groups, &values}, params.MaxGasForGetEligibleValidatorGroupsVoteTotals, header, state)
	if err != nil {
		log.Error("DistributeEpochRewards, error calling getEligibleValidatorGroupsVoteTotals", "err", err)
	}

	voteTotals := make([]voteTotal, len(groups))
	for i, group := range groups {
		log.Info("Got group vote total", "group", group, "value", values[i])
		voteTotals[i].Group = group
		voteTotals[i].Value = values[i]
	}
	return voteTotals, err
}

func DistributeEpochRewards(header *types.Header, state vm.StateDB, groups []common.Address) error {
	voteTotals, err := getEligibleValidatorGroupsVoteTotals(header, state)
	if err != nil {
		log.Error("DistributeEpochRewards, error calling getEligibleValidatorGroupsVoteTotals", "err", err)
	}

	// One gold
	reward := math.BigPow(10, 18)
	for _, group := range groups {
		for _, voteTotal := range voteTotals {
			if voteTotal.Group == group {
				voteTotal.Value.Add(voteTotal.Value, reward)
				break
			}
		}

		sort.Slice(voteTotals, func(i, j int) bool {
			return voteTotals[i].Value.Cmp(voteTotals[j].Value) < 0
		})

		lesser := common.ZeroAddress
		greater := common.ZeroAddress
		for i, voteTotal := range voteTotals {
			if voteTotal.Group == group {
				if i > 0 {
					lesser = voteTotals[i-1].Group
				}
				if i+1 < len(voteTotals) {
					greater = voteTotals[i+1].Group
				}
				break
			}
		}
		_, err := contract_comm.MakeCall(params.ElectionRegistryId, electionABI, "distributeEpochRewards", []interface{}{group, reward, lesser, greater}, nil, params.MaxGasForDistributeEpochRewards, common.Big0, header, state)

		if err != nil {
			log.Error("DistributeEpochRewards, error calling distributeEpochRewards", "err", err)
		}
	}
	return nil
}
