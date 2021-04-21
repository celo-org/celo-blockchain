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
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/contract_comm"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Freezer.json
	isFrozenABI = `[{
      "constant": true,
      "inputs": [
        {
          "name": "",
          "type": "address"
        }
      ],
      "name": "isFrozen",
      "outputs": [
        {
          "name": "",
          "type": "bool"
        }
      ],
      "payable": false,
      "stateMutability": "view",
      "type": "function"
    }]`
)

var (
	isFrozenFuncABI, _ = abi.JSON(strings.NewReader(isFrozenABI))
)

func IsFrozen(registryId [32]byte, header *types.Header, state vm.StateDB) (bool, error) {
	address, err := contract_comm.GetRegisteredAddress(registryId, header, state)
	if err != nil {
		return false, err
	}
	var isFrozen bool
	if err := contract_comm.MakeStaticCall(params.FreezerRegistryId, isFrozenFuncABI, "isFrozen", []interface{}{address}, &isFrozen, params.MaxGasForIsFrozen, header, state); err != nil {
		return false, err
	}

	return isFrozen, nil
}
