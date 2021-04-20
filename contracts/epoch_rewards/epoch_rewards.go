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
package epoch_rewards

import (
	"math/big"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Election.json
const epochRewardsABIString string = `[
    {
      "constant": true,
      "inputs": [],
      "name": "calculateTargetEpochRewards",
      "outputs": [
        {
          "name": "",
          "type": "uint256"
        },
        {
          "name": "",
          "type": "uint256"
        },
        {
          "name": "",
          "type": "uint256"
        },
        {
          "name": "",
          "type": "uint256"
        }
      ],
      "payable": false,
      "stateMutability": "view",
      "type": "function"
    },
    { 
      "constant": true,
      "inputs": [],
      "name": "carbonOffsettingPartner",
      "outputs": [
        { 
          "name": "",
          "type": "address"
        }
      ],
      "payable": false,
      "stateMutability": "view",
      "type": "function"
    },
    {
      "constant": false,
      "inputs": [],
      "name": "updateTargetVotingYield",
      "outputs": [],
      "payable": false,
      "stateMutability": "nonpayable",
      "type": "function"
    },
    {
      "constant": true,
      "inputs": [],
      "name": "isReserveLow",
      "outputs": [
        {
          "name": "",
          "type": "bool"
        }
      ],
      "payable": false,
      "stateMutability": "view",
      "type": "function"
    },
    {
      "constant": true,
      "inputs": [],
      "name": "frozen",
      "outputs": [
        {
          "name": "",
          "type": "bool"
        }
      ],
      "payable": false,
      "stateMutability": "view",
      "type": "function"
    }
]
`

var (
	calculateTargetEpochRewardsMethod *contracts.BoundMethod
	isReserveLowMethod                *contracts.BoundMethod
	carbonOffsettingPartnerMethod     *contracts.BoundMethod
	updateTargetVotingYieldMethod     *contracts.BoundMethod
)

func init() {
	epochRewardsABI, err := abi.JSON(strings.NewReader(epochRewardsABIString))
	if err != nil {
		panic(err)
	}

	calculateTargetEpochRewardsMethod = contracts.NewRegisteredContractMethod(params.EpochRewardsRegistryId, &epochRewardsABI, "calculateTargetEpochRewards", params.MaxGasForCalculateTargetEpochPaymentAndRewards)
	isReserveLowMethod = contracts.NewRegisteredContractMethod(params.EpochRewardsRegistryId, &epochRewardsABI, "isReserveLow", params.MaxGasForIsReserveLow)
	carbonOffsettingPartnerMethod = contracts.NewRegisteredContractMethod(params.EpochRewardsRegistryId, &epochRewardsABI, "carbonOffsettingPartner", params.MaxGasForGetCarbonOffsettingPartner)
	updateTargetVotingYieldMethod = contracts.NewRegisteredContractMethod(params.EpochRewardsRegistryId, &epochRewardsABI, "updateTargetVotingYield", params.MaxGasForUpdateTargetVotingYield)
}

func UpdateTargetVotingYield(vmRunner vm.EVMRunner) error {
	err := updateTargetVotingYieldMethod.Execute(vmRunner, nil, common.Big0)
	return err
}

// Returns the per validator epoch reward, the total voter reward, the total community reward, and
// the total carbon offsetting partner award, for the epoch.
func CalculateTargetEpochRewards(vmRunner vm.EVMRunner) (*big.Int, *big.Int, *big.Int, *big.Int, error) {
	var validatorEpochReward *big.Int
	var totalVoterRewards *big.Int
	var totalCommunityReward *big.Int
	var totalCarbonOffsettingPartnerReward *big.Int
	err := calculateTargetEpochRewardsMethod.Query(vmRunner, &[]interface{}{&validatorEpochReward, &totalVoterRewards, &totalCommunityReward, &totalCarbonOffsettingPartnerReward})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return validatorEpochReward, totalVoterRewards, totalCommunityReward, totalCarbonOffsettingPartnerReward, nil
}

// Determines if the reserve is below it's critical threshold
func IsReserveLow(vmRunner vm.EVMRunner) (bool, error) {
	var isLow bool
	err := isReserveLowMethod.Query(vmRunner, &isLow)
	if err != nil {
		return false, err
	}
	return isLow, nil
}

// Returns the address of the carbon offsetting partner
func GetCarbonOffsettingPartnerAddress(vmRunner vm.EVMRunner) (common.Address, error) {
	var carbonOffsettingPartner common.Address
	err := carbonOffsettingPartnerMethod.Query(vmRunner, &carbonOffsettingPartner)
	if err != nil {
		return common.ZeroAddress, err
	}
	return carbonOffsettingPartner, nil
}
