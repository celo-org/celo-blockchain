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
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
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
