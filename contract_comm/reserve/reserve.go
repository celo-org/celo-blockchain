// Copyright 2020 The Celo Authors
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
package reserve

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/reserve.json
const reserveABIString string = `[
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
    }
  ]
`

var reserveABI, _ = abi.JSON(strings.NewReader(reserveABIString))

// Determines if the reserve is below it's critical threshold
func IsReserveLow(header *types.Header, state vm.StateDB) (bool, error) {
	var isLow bool
	_, err := contract_comm.MakeStaticCall(params.ReserveRegistryId, reserveABI, "isReserveLow", []interface{}{}, &[]interface{}{&isLow}, params.MaxGasForIsReserveLow, header, state)
	if err != nil {
		return false, err
	}
	return isLow, nil
}
