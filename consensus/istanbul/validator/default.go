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
	"fmt"
	"io"
	"math"
	"math/big"
	"strings"
	"sync"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/rlp"
)

type defaultValidator struct {
	address                  common.Address
	blsPublicKey             blscrypto.SerializedPublicKey
	uncompressedBlsPublicKey []byte
}

func newValidatorFromDataWithBLSKeyCache(data *istanbul.ValidatorDataWithBLSKeyCache) *defaultValidator {
	return &defaultValidator{
		address:                  data.Address,
		blsPublicKey:             data.BLSPublicKey,
		uncompressedBlsPublicKey: data.UncompressedBLSPublicKey,
	}
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

func (val *defaultValidator) AsDataWithBLSKeyCache() *istanbul.ValidatorDataWithBLSKeyCache {
	return &istanbul.ValidatorDataWithBLSKeyCache{
		Address:                  val.address,
		BLSPublicKey:             val.blsPublicKey,
		UncompressedBLSPublicKey: val.uncompressedBlsPublicKey,
	}
}

func (val *defaultValidator) Address() common.Address                     { return val.address }
func (val *defaultValidator) BLSPublicKey() blscrypto.SerializedPublicKey { return val.blsPublicKey }
func (val *defaultValidator) String() string                              { return val.Address().String() }

func (val *defaultValidator) BLSPublicKeyUncompressed() []byte {
	if len(val.uncompressedBlsPublicKey) == 0 {
		log.Warn("Uncompressed BLS key wasn't cached", "address", val.address)
		val.CacheUncompressedBLSKey()
	}
	return val.uncompressedBlsPublicKey
}

func (val *defaultValidator) Copy() istanbul.Validator {
	return &defaultValidator{
		address:                  val.address,
		blsPublicKey:             val.blsPublicKey,
		uncompressedBlsPublicKey: val.uncompressedBlsPublicKey,
	}
}

func (val *defaultValidator) CacheUncompressedBLSKey() {
	if len(val.uncompressedBlsPublicKey) == 0 {
		uncompressed, err := blscrypto.UncompressKey(val.blsPublicKey)
		if err != nil {
			log.Error("Bad BLS public key", "adddress", val.address, "bls", val.blsPublicKey)
		}
		val.uncompressedBlsPublicKey = uncompressed
	}
}

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
}

func newDefaultSet(validators []istanbul.ValidatorData) *defaultSet {
	return &defaultSet{
		validators: mapDataToValidators(validators),
	}
}

func newDefaultSetFromDataWithBLSKeyCache(validators []istanbul.ValidatorDataWithBLSKeyCache) *defaultSet {
	return &defaultSet{
		validators: mapDataToValidatorsWithBLSKeyCache(validators),
	}
}

// We include the optimization described at https://arxiv.org/pdf/1901.07160.pdf as “PM-6” and
// discussed in Lemma 22. For values of N=3F for integer F=1,2,3,.. we can tolerate a quorum
// size one smaller than anticipated by Q = N - F. The intersection of any two sets of Q
// nodes of N=3F must contain an honest validator.
//
// For example, with N=9, F=2, Q=6. Any two sets of Q=6 from N=9 nodes must overlap
// by >9-6=3 nodes. At least 3-F=3-2=1 must be honest.
//
// 1 2 3 4 5 6 7 8 9
// x x x x x x
//       y y y y y y
//       F F H
//
// For N=10, F=3, Q=7. Any two sets of Q=7 nodes from N=10 must overlap by >4 nodes.
// At least 4-F=4-3=1 must be honest.
//
// 1 2 3 4 5 6 7 8 9 10
// x x x x x x x
//       y y y y y y y
//       F F F H

func (valSet *defaultSet) F() int             { return int(math.Ceil(float64(valSet.Size())/3)) - 1 }
func (valSet *defaultSet) MinQuorumSize() int { return int(math.Ceil(float64(2*valSet.Size()) / 3)) }

func (valSet *defaultSet) String() string {
	var buf strings.Builder
	if _, err := buf.WriteString("["); err != nil {
		return fmt.Sprintf("String()  error: %s", err)
	}
	for _, v := range valSet.List() {
		if _, err := buf.WriteString(v.String()); err != nil {
			return fmt.Sprintf("String()  error: %s", err)
		}
		if _, err := buf.WriteString(" "); err != nil {
			return fmt.Sprintf("String()  error: %s", err)
		}

	}
	if _, err := buf.WriteString("]"); err != nil {
		return fmt.Sprintf("String()  error: %s", err)
	}

	return fmt.Sprintf("{validators: %s}", buf.String())
}

func (valSet *defaultSet) CacheUncompressedBLSKey() {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	for _, v := range valSet.validators {
		v.CacheUncompressedBLSKey()
	}
}

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
	newValSet := &defaultSet{}
	newValSet.validators = make([]istanbul.Validator, len(valSet.validators))
	for i, v := range valSet.validators {
		newValSet.validators[i] = v.Copy()
	}
	return newValSet
}

func (valSet *defaultSet) HasBLSKeyCache() bool {
	for _, v := range valSet.validators {
		if v.AsDataWithBLSKeyCache().UncompressedBLSPublicKey == nil && v.BLSPublicKey() != (blscrypto.SerializedPublicKey{}) {
			return false
		}
	}
	return true
}

func (valSet *defaultSet) AsData() *istanbul.ValidatorSetData {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	return &istanbul.ValidatorSetData{
		Validators: MapValidatorsToData(valSet.validators),
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
	return nil
}

func (val *defaultSet) Serialize() ([]byte, error) { return rlp.EncodeToBytes(val) }

// Utility Functions

func MapValidatorsToData(validators []istanbul.Validator) []istanbul.ValidatorData {
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

func MapValidatorsToDataWithBLSKeyCache(validators []istanbul.Validator) []istanbul.ValidatorDataWithBLSKeyCache {
	validatorsData := make([]istanbul.ValidatorDataWithBLSKeyCache, len(validators))
	for i, v := range validators {
		validatorsData[i] = *v.AsDataWithBLSKeyCache()
	}
	return validatorsData
}

func mapDataToValidatorsWithBLSKeyCache(data []istanbul.ValidatorDataWithBLSKeyCache) []istanbul.Validator {
	validators := make([]istanbul.Validator, len(data))
	for i, v := range data {
		validators[i] = newValidatorFromDataWithBLSKeyCache(&v)
	}
	return validators
}
