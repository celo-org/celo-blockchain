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
package validators

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Validators.json
const validatorsABIString string = `[
  {
    "constant": true,
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
  },
  {"constant": true,
              "inputs": [],
        "name": "getValidators",
        "outputs": [
       {
            "name": "",
      "type": "address[]"
       }
        ],
        "payable": false,
        "stateMutability": "view",
        "type": "function"
       }, {
        "name": "getValidator",
        "inputs": [
          {
            "name": "account",
            "type": "address"
          }
        ],
        "outputs": [
          {
            "name": "identifier",
            "type": "string"
          },
          {
            "name": "name",
            "type": "string"
          },
          {
            "name": "url",
            "type": "string"
          },
          {
            "name": "publicKeysData",
            "type": "bytes"
          },
          {
            "name": "affiliation",
            "type": "address"
          }
        ],
        "payable": false,
        "stateMutability": "view",
          "type": "function"
          }
]`

var validatorsABI, _ = abi.JSON(strings.NewReader(validatorsABIString))

func RetrieveRegisteredValidators(header *types.Header, state vm.StateDB) (map[common.Address]bool, error) {
	var regVals []common.Address

	// Get the new epoch's validator set
	maxGasForGetRegisteredValidators := uint64(1000000)
	if _, err := contract_comm.MakeStaticCall(params.ValidatorsRegistryId, validatorsABI, "getRegisteredValidators", []interface{}{}, &regVals, maxGasForGetRegisteredValidators, header, state); err != nil {
		return nil, err
	}

	returnMap := make(map[common.Address]bool)

	for _, address := range regVals {
		returnMap[address] = true
	}

	return returnMap, nil
}


func (sb *Backend) GetValidatorSet(header *types.Header, state *state.StateDB) ([]istanbul.ValidatorData, error) {
	var newValSet []istanbul.ValidatorData
	var newValSetAddresses []common.Address
  // Get the new epoch's validator set
  maxGasForGetValidators := uint64(10000000)
  // TODO(asa) - Once the validator election smart contract is completed, then a more accurate gas value should be used.
	_, err := contract_comm.MakeStaticCall(params.ValidatorsRegistryId, validatorsABI, "getValidators", []interface{}{}, &newValSetAddresses, maxGasForGetValidators, header, state)
  if err != nil {
    return nil, err
  }

  for _, addr := range newValSetAddresses {
    validator := struct {
      Identifier     string
      Name           string
      Url            string
      PublicKeysData []byte
      Affiliation    common.Address
    }{}
    _, err := contract_comm.MakeStaticCallWithAddress(params.ValidatorsRegistryId, getValidatorsFuncABI, "getValidator", []interface{}{addr}, &validator, maxGasForGetValidators, header, state)
    if err != nil {
      log.Error("Unable to retrieve Validator Account from Validator smart contract", "err", err)
      return nil, err
    }
    expectedLength := 64 + blscrypto.PUBLICKEYBYTES + blscrypto.SIGNATUREBYTES
    if len(validator.PublicKeysData) != expectedLength {
      return nil, fmt.Errorf("length of publicKeysData incorrect. Expected %d, got %d", expectedLength, len(validator.PublicKeysData))
    }
    blsPublicKey := validator.PublicKeysData[64 : 64+blscrypto.PUBLICKEYBYTES]
    newValSet = append(newValSet, istanbul.ValidatorData{
      addr,
      blsPublicKey,
    })
  }
  return newValSet, nil
}


