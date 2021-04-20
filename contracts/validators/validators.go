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
	"fmt"
	"math/big"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/vm"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/params"
)

// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Validators.json
const validatorsABIString string = `[
    {
        "constant": true,
        "inputs": [],
        "name": "getRegisteredValidatorSigners",
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
    {
      "constant": true,
      "inputs": [
        {
          "name": "signer",
          "type": "address"
        }
      ],
      "name": "getValidatorBlsPublicKeyFromSigner",
      "outputs": [
        {
          "name": "blsKey",
          "type": "bytes"
        }
      ],
      "payable": false,
      "stateMutability": "view",
      "type": "function"
    },
	  {
      "constant": true,
      "inputs": [
        {
          "name": "account",
          "type": "address"
        }
      ],
      "name": "getValidator",
      "outputs": [
        {
          "name": "ecdsaPublicKey",
          "type": "bytes"
        },
        {
          "name": "blsPublicKey",
          "type": "bytes"
        },
        {
          "name": "affiliation",
          "type": "address"
        },
        {
          "name": "score",
          "type": "uint256"
        },
        {
          "name": "signer",
          "type": "address"
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
          "name": "validator",
          "type": "address"
        },
        {
          "name": "maxPayment",
          "type": "uint256"
        }
      ],
      "name": "distributeEpochPaymentsFromSigner",
      "outputs": [
        {
          "name": "",
          "type": "uint256"
        }
      ],
      "payable": false,
      "stateMutability": "nonpayable",
      "type": "function"
    },
    {
      "constant": false,
      "inputs": [
        {
          "name": "validator",
          "type": "address"
        },
        {
          "name": "uptime",
          "type": "uint256"
        }
      ],
      "name": "updateValidatorScoreFromSigner",
      "outputs": [],
      "payable": false,
      "stateMutability": "nonpayable",
      "type": "function"
    },
    {
      "constant": true,
      "inputs": [
        {
          "name": "account",
          "type": "address"
        }
      ],
      "name": "getMembershipInLastEpochFromSigner",
      "outputs": [
        {
          "name": "",
          "type": "address"
        }
      ],
      "payable": false,
      "stateMutability": "view",
      "type": "function"
    }
]`

type ValidatorContractData struct {
	EcdsaPublicKey []byte
	BlsPublicKey   []byte
	Affiliation    common.Address
	Score          *big.Int
	Signer         common.Address
}

var (
	getRegisteredValidatorSignersMethod      *contracts.BoundMethod
	getRegisteredValidatorsMethod            *contracts.BoundMethod
	getValidatorBlsPublicKeyFromSignerMethod *contracts.BoundMethod
	getMembershipInLastEpochFromSignerMethod *contracts.BoundMethod
	getValidatorMethod                       *contracts.BoundMethod
	updateValidatorScoreFromSignerMethod     *contracts.BoundMethod
	distributeEpochPaymentsFromSignerMethod  *contracts.BoundMethod
)

func init() {

	validatorsABI, err := abi.JSON(strings.NewReader(validatorsABIString))
	if err != nil {
		panic(err)
	}

	getRegisteredValidatorSignersMethod = contracts.NewRegisteredContractMethod(params.ValidatorsRegistryId, &validatorsABI, "getRegisteredValidatorSigners", params.MaxGasForGetRegisteredValidators)
	getRegisteredValidatorsMethod = contracts.NewRegisteredContractMethod(params.ValidatorsRegistryId, &validatorsABI, "getRegisteredValidators", params.MaxGasForGetRegisteredValidators)
	getValidatorBlsPublicKeyFromSignerMethod = contracts.NewRegisteredContractMethod(params.ValidatorsRegistryId, &validatorsABI, "getValidatorBlsPublicKeyFromSigner", params.MaxGasForGetValidator)
	getMembershipInLastEpochFromSignerMethod = contracts.NewRegisteredContractMethod(params.ValidatorsRegistryId, &validatorsABI, "getMembershipInLastEpochFromSigner", params.MaxGasForGetMembershipInLastEpoch)
	getValidatorMethod = contracts.NewRegisteredContractMethod(params.ValidatorsRegistryId, &validatorsABI, "getValidator", params.MaxGasForGetValidator)
	updateValidatorScoreFromSignerMethod = contracts.NewRegisteredContractMethod(params.ValidatorsRegistryId, &validatorsABI, "updateValidatorScoreFromSigner", params.MaxGasForUpdateValidatorScore)
	distributeEpochPaymentsFromSignerMethod = contracts.NewRegisteredContractMethod(params.ValidatorsRegistryId, &validatorsABI, "distributeEpochPaymentsFromSigner", params.MaxGasForDistributeEpochPayment)
}

func RetrieveRegisteredValidatorSigners(vmRunner vm.EVMRunner) ([]common.Address, error) {
	// Get the new epoch's validator signer set
	var regVals []common.Address
	if err := getRegisteredValidatorSignersMethod.Query(vmRunner, &regVals); err != nil {
		return nil, err
	}

	return regVals, nil
}

func RetrieveRegisteredValidators(vmRunner vm.EVMRunner) ([]common.Address, error) {
	// Get the new epoch's validator set
	var regVals []common.Address
	if err := getRegisteredValidatorsMethod.Query(vmRunner, &regVals); err != nil {
		return nil, err
	}

	return regVals, nil
}

func GetValidator(vmRunner vm.EVMRunner, validatorAddress common.Address) (ValidatorContractData, error) {
	var validator ValidatorContractData
	err := getValidatorMethod.Query(vmRunner, &validator, validatorAddress)
	if err != nil {
		return validator, err
	}
	if len(validator.BlsPublicKey) != blscrypto.PUBLICKEYBYTES {
		return validator, fmt.Errorf("length of bls public key incorrect. Expected %d, got %d", blscrypto.PUBLICKEYBYTES, len(validator.BlsPublicKey))
	}
	return validator, nil
}

func GetValidatorData(vmRunner vm.EVMRunner, validatorAddresses []common.Address) ([]istanbul.ValidatorData, error) {
	var validatorData []istanbul.ValidatorData
	for _, addr := range validatorAddresses {
		var blsKey []byte
		err := getValidatorBlsPublicKeyFromSignerMethod.Query(vmRunner, &blsKey, addr)
		if err != nil {
			return nil, err
		}

		if len(blsKey) != blscrypto.PUBLICKEYBYTES {
			return nil, fmt.Errorf("length of bls public key incorrect. Expected %d, got %d", blscrypto.PUBLICKEYBYTES, len(blsKey))
		}
		blsKeyFixedSize := blscrypto.SerializedPublicKey{}
		copy(blsKeyFixedSize[:], blsKey)
		validator := istanbul.ValidatorData{
			Address:      addr,
			BLSPublicKey: blsKeyFixedSize,
		}
		validatorData = append(validatorData, validator)
	}
	return validatorData, nil
}

func UpdateValidatorScore(vmRunner vm.EVMRunner, address common.Address, uptime *big.Int) error {
	err := updateValidatorScoreFromSignerMethod.Execute(vmRunner, nil, common.Big0, address, uptime)
	return err
}

func DistributeEpochReward(vmRunner vm.EVMRunner, address common.Address, maxReward *big.Int) (*big.Int, error) {
	var epochReward *big.Int
	err := distributeEpochPaymentsFromSignerMethod.Execute(vmRunner, &epochReward, common.Big0, address, maxReward)
	return epochReward, err
}

func GetMembershipInLastEpoch(vmRunner vm.EVMRunner, validator common.Address) (common.Address, error) {
	var group common.Address
	err := getMembershipInLastEpochFromSignerMethod.Query(vmRunner, &group, validator)
	if err != nil {
		return common.ZeroAddress, err
	}
	return group, nil
}
