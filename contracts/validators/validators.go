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

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/internal/n"
	"github.com/celo-org/celo-blockchain/core/vm"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
)

type ValidatorContractData struct {
	EcdsaPublicKey []byte
	BlsPublicKey   []byte
	Affiliation    common.Address
	Score          *big.Int
	Signer         common.Address
}

const (
	maxGasForGetRegisteredValidators uint64 = 2 * n.Million
	maxGasForGetValidator            uint64 = 100 * n.Thousand
)

var (
	getRegisteredValidatorSignersMethod      = contracts.NewRegisteredContractMethod(config.ValidatorsRegistryId, abis.Validators, "getRegisteredValidatorSigners", maxGasForGetRegisteredValidators)
	getRegisteredValidatorsMethod            = contracts.NewRegisteredContractMethod(config.ValidatorsRegistryId, abis.Validators, "getRegisteredValidators", maxGasForGetRegisteredValidators)
	getValidatorBlsPublicKeyFromSignerMethod = contracts.NewRegisteredContractMethod(config.ValidatorsRegistryId, abis.Validators, "getValidatorBlsPublicKeyFromSigner", maxGasForGetValidator)

	getValidatorMethod = contracts.NewRegisteredContractMethod(config.ValidatorsRegistryId, abis.Validators, "getValidator", maxGasForGetValidator)
)

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
