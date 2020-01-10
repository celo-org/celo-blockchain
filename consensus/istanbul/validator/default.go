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
	"encoding/json"
	"io"
	"math"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/rlp"
)

type defaultValidator struct {
	address      common.Address
	blsPublicKey []byte
}

func newValidatorFromData(data *istanbul.ValidatorData) *defaultValidator {
	return &defaultValidator{
		address:      data.Address,
		blsPublicKey: data.BLSPublicKey,
	}
}

func (val *defaultValidator) AsData() *istanbul.ValidatorData {
	return &istanbul.ValidatorData{
		Address:      val.address,
		BLSPublicKey: val.blsPublicKey,
	}
}

func (val *defaultValidator) Address() common.Address { return val.address }
func (val *defaultValidator) BLSPublicKey() []byte    { return val.blsPublicKey }
func (val *defaultValidator) String() string          { return val.Address().String() }

func (val *defaultValidator) Serialize() ([]byte, error) { return rlp.EncodeToBytes(val) }

// JSON Encoding -----------------------------------------------------------------------

func (val *defaultValidator) MarshalJSON() ([]byte, error) { return json.Marshal(val.AsData()) }
func (val *defaultValidator) UnmarshalJSON(b []byte) error {
	var data istanbul.ValidatorData
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	*val = *newValidatorFromData(&data)
	return nil
}

// RLP Encoding -----------------------------------------------------------------------

func (val *defaultValidator) EncodeRLP(w io.Writer) error { return rlp.Encode(w, val.AsData()) }
func (val *defaultValidator) DecodeRLP(stream *rlp.Stream) error {
	var v istanbul.ValidatorData
	if err := stream.Decode(&v); err != nil {
		return err
	}

	*val = *newValidatorFromData(&v)
	return nil
}

// ----------------------------------------------------------------------------

type defaultSet struct {
	validators  []istanbul.Validator
	validatorMu sync.RWMutex
	// This is set when we call `getOrderedValidators`
	// TODO Rename to `EpochState` that has validators & randomness
	randomness common.Hash
}

func newDefaultSet(validators []istanbul.ValidatorData) *defaultSet {
	return &defaultSet{
		validators: mapDataToValidators(validators),
	}
}

func (val *defaultSet) Serialize() ([]byte, error)        { return rlp.EncodeToBytes(val) }
func (valSet *defaultSet) F() int                         { return int(math.Ceil(float64(valSet.Size())/3)) - 1 }
func (valSet *defaultSet) MinQuorumSize() int             { return int(math.Ceil(float64(2*valSet.Size()) / 3)) }
func (valSet *defaultSet) SetRandomness(seed common.Hash) { valSet.randomness = seed }
func (valSet *defaultSet) GetRandomness() common.Hash     { return valSet.randomness }

func (valSet *defaultSet) Size() int {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()

	return len(valSet.validators)
}

func (valSet *defaultSet) List() []istanbul.Validator {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	return valSet.validators
}

func (valSet *defaultSet) GetByIndex(i uint64) istanbul.Validator {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	if i < uint64(valSet.Size()) {
		return valSet.validators[i]
	}
	return nil
}

func (valSet *defaultSet) GetByAddress(addr common.Address) (int, istanbul.Validator) {
	for i, val := range valSet.List() {
		if addr == val.Address() {
			return i, val
		}
	}
	return -1, nil
}

func (valSet *defaultSet) ContainsByAddress(addr common.Address) bool {
	i, _ := valSet.GetByAddress(addr)
	return i != -1
}

func (valSet *defaultSet) GetIndex(addr common.Address) int {
	i, _ := valSet.GetByAddress(addr)
	return i
}

func (valSet *defaultSet) AddValidators(validators []istanbul.ValidatorData) bool {
	newValidators := make([]istanbul.Validator, 0, len(validators))
	newAddressesMap := make(map[common.Address]bool)
	for i := range validators {
		newAddressesMap[validators[i].Address] = true
		newValidators = append(newValidators, newValidatorFromData(&validators[i]))
	}

	valSet.validatorMu.Lock()
	defer valSet.validatorMu.Unlock()

	// Verify that the validators to add is not already in the valset
	for _, v := range valSet.validators {
		if _, ok := newAddressesMap[v.Address()]; ok {
			return false
		}
	}

	valSet.validators = append(valSet.validators, newValidators...)

	return true
}

func (valSet *defaultSet) RemoveValidators(removedValidators *big.Int) bool {
	if removedValidators.BitLen() == 0 {
		return true
	}

	if removedValidators.BitLen() > len(valSet.validators) {
		return false
	}

	valSet.validatorMu.Lock()
	defer valSet.validatorMu.Unlock()

	// Using this method to filter the validators list: https://github.com/golang/go/wiki/SliceTricks#filtering-without-allocating, so that no
	// new memory will be allocated
	tempList := valSet.validators[:0]
	for i, v := range valSet.validators {
		if removedValidators.Bit(i) == 0 {
			tempList = append(tempList, v)
		}
	}

	valSet.validators = tempList
	return true
}

func (valSet *defaultSet) Copy() istanbul.ValidatorSet {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	newValSet := NewSet(mapValidatorsToData(valSet.validators))
	newValSet.SetRandomness(valSet.randomness)
	return newValSet
}

func (valSet *defaultSet) AsData() *istanbul.ValidatorSetData {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	return &istanbul.ValidatorSetData{
		Validators: mapValidatorsToData(valSet.validators),
		Randomness: valSet.randomness,
	}
}

// JSON Encoding -----------------------------------------------------------------------

func (val *defaultSet) MarshalJSON() ([]byte, error) { return json.Marshal(val.AsData()) }

func (val *defaultSet) UnmarshalJSON(b []byte) error {
	var data istanbul.ValidatorSetData
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	*val = *newDefaultSet(data.Validators)
	val.SetRandomness(data.Randomness)
	return nil
}

// RLP Encoding -----------------------------------------------------------------------

func (val *defaultSet) EncodeRLP(w io.Writer) error { return rlp.Encode(w, val.AsData()) }

func (val *defaultSet) DecodeRLP(stream *rlp.Stream) error {
	var data istanbul.ValidatorSetData
	if err := stream.Decode(&data); err != nil {
		return err
	}
	*val = *newDefaultSet(data.Validators)
	val.SetRandomness(data.Randomness)
	return nil
}

// Utility Functions

func mapValidatorsToData(validators []istanbul.Validator) []istanbul.ValidatorData {
	validatorsData := make([]istanbul.ValidatorData, len(validators))
	for i, v := range validators {
		validatorsData[i] = *v.AsData()
	}
	return validatorsData
}

func mapDataToValidators(data []istanbul.ValidatorData) []istanbul.Validator {
	validators := make([]istanbul.Validator, len(data))
	for i, v := range data {
		validators[i] = newValidatorFromData(&v)
	}
	return validators
}
