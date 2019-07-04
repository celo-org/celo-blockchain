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
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/userspace_communication"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Validators.json
	getRegisteredValidatorsABI = `[{"constant": true,
                                        "inputs": [],
                                        "name": "getRegisteredValidators",
                                        "outputs": [
                                          {
                                            "name": "",
                                            "type": "address[]"
                                          }
                                        ],
                                        "payable": false,
                                        "stateMutability": "view",
                                        "type": "function"
                                      }]`
)

var (
	getRegisteredValidatorsFuncABI, _ = abi.JSON(strings.NewReader(getRegisteredValidatorsABI))
)

// This function will retrieve the set of registered validators from the validator election
// smart contract.
func (sb *Backend) retrieveRegisteredValidators() (map[common.Address]bool, error) {
	var regVals []common.Address

	validatorAddress, _ := sb.regAdd.GetRegisteredAddressAtCurrentHeader(params.ValidatorsRegistryId)
	if validatorAddress == nil {
		return nil, errValidatorsContractNotRegistered
	} else {
		// Get the new epoch's validator set
		maxGasForGetRegisteredValidators := uint64(1000000)
		if _, err := userspace_communication.MakeStaticCall(*validatorAddress, getRegisteredValidatorsFuncABI, "getRegisteredValidators", []interface{}{}, &regVals, maxGasForGetRegisteredValidators, nil, nil); err != nil {
			return nil, err
		}
	}

	returnMap := make(map[common.Address]bool)

	for _, address := range regVals {
		returnMap[address] = true
	}

	return returnMap, nil
}
