// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package validator

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/rlp"
)

func New(addr common.Address, blsPublicKey blscrypto.SerializedPublicKey) istanbul.Validator {
	return &defaultValidator{
		address:                  addr,
		blsPublicKey:             blsPublicKey,
		uncompressedBlsPublicKey: nil,
	}
}

func DeserializeValidator(binaryData []byte) (istanbul.Validator, error) {
	var value defaultValidator

	err := rlp.DecodeBytes(binaryData, &value)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func NewSet(validators []istanbul.ValidatorData) istanbul.ValidatorSet {
	return newDefaultSet(validators)
}

func NewSetFromDataWithBLSKeyCache(validators []istanbul.ValidatorDataWithBLSKeyCache) istanbul.ValidatorSet {
	return newDefaultSetFromDataWithBLSKeyCache(validators)
}

func DeserializeValidatorSet(binaryData []byte) (istanbul.ValidatorSet, error) {
	var value defaultSet

	err := rlp.DecodeBytes(binaryData, &value)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func ExtractValidators(extraData []byte) []istanbul.ValidatorData {
	// get the validator addresses
	validatorLength := common.AddressLength + blscrypto.PUBLICKEYBYTES
	validators := make([]istanbul.ValidatorData, (len(extraData) / validatorLength))
	for i := 0; i < len(validators); i++ {
		copy(validators[i].Address[:], extraData[i*validatorLength:i*validatorLength+common.AddressLength])
		copy(validators[i].BLSPublicKey[:], extraData[i*validatorLength+common.AddressLength:])
	}

	return validators
}

// Check whether the extraData is presented in prescribed form
func ValidExtraData(extraData []byte) bool {
	return len(extraData)%common.AddressLength == 0
}
