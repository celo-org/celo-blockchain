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

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Election.json
const epochRewardsABIString string = `[
    {
      "constant": true,
      "inputs": [],
      "name": "calculateTargetEpochPaymentAndRewards",
      "outputs": [
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
    }
]
`

var epochRewardsABI, _ = abi.JSON(strings.NewReader(epochRewardsABIString))

func CalculateTargetEpochPaymentAndRewards(header *types.Header, state vm.StateDB) (*big.Int, *big.Int, error) {
	var validatorEpochPayment *big.Int
	var totalVoterRewards *big.Int
	_, err := contract_comm.MakeStaticCall(params.EpochRewardsRegistryId, epochRewardsABI, "calculateTargetEpochPaymentAndRewards", []interface{}{}, &[]interface{}{&validatorEpochPayment, &totalVoterRewards}, params.MaxGasForCalculateTargetEpochPaymentAndRewards, header, state)
	if err != nil {
		return nil, nil, err
	}
	return validatorEpochPayment, totalVoterRewards, nil
}
