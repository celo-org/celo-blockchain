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
	"fmt"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"math"
	"math/big"
	"reflect"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

type defaultValidator struct {
	address      common.Address
	blsPublicKey blscrypto.SerializedPublicKey
}

func (val *defaultValidator) Address() common.Address {
	return val.address
}

func (val *defaultValidator) BLSPublicKey() blscrypto.SerializedPublicKey {
	return val.blsPublicKey
}

func (val *defaultValidator) String() string {
	return val.Address().String()
}

// ----------------------------------------------------------------------------

type defaultSet struct {
	validators istanbul.Validators
	policy     istanbul.ProposerPolicy

	proposer    istanbul.Validator
	validatorMu sync.RWMutex
	selector    istanbul.ProposerSelector
	randomness  common.Hash
}

func newDefaultSet(validators []istanbul.ValidatorData, policy istanbul.ProposerPolicy) *defaultSet {
	valSet := &defaultSet{}

	valSet.policy = policy
	// init validators
	valSet.validators = make([]istanbul.Validator, len(validators))
	for i, validator := range validators {
		valSet.validators[i] = New(validator.Address, validator.BLSPublicKey)
	}
	// init proposer
	if valSet.Size() > 0 {
		valSet.proposer = valSet.GetByIndex(0)
	}

	switch policy {
	case istanbul.Sticky:
		valSet.selector = StickyProposer
	case istanbul.RoundRobin:
		valSet.selector = RoundRobinProposer
	case istanbul.ShuffledRoundRobin:
		valSet.selector = ShuffledRoundRobinProposer
	default:
		// Programming error.
		panic(fmt.Sprintf("unknown proposer selection policy: %v", policy))
	}

	return valSet
}

func (valSet *defaultSet) Size() int {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()

	size := 0
	for i := range valSet.validators {
		if (valSet.validators[i].Address() != common.Address{}) {
			size++
		}
	}
	return size
}

func (valSet *defaultSet) PaddedSize() int {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()

	return len(valSet.validators)
}

func (valSet *defaultSet) List() []istanbul.Validator {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	return valSet.validators
}

func (valSet *defaultSet) FilteredList() []istanbul.Validator {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()

	filteredList := []istanbul.Validator{}
	for i := 0; i < valSet.PaddedSize(); i++ {
		currentValidator := valSet.GetByIndex(uint64(i))
		if (currentValidator.Address() != common.Address{}) {
			filteredList = append(filteredList, currentValidator)
		}
	}
	return filteredList
}

func (valSet *defaultSet) GetByIndex(i uint64) istanbul.Validator {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	if i < uint64(valSet.PaddedSize()) {
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
	for _, val := range valSet.List() {
		if addr == val.Address() {
			return true
		}
	}
	return false
}

func (valSet *defaultSet) GetFilteredIndex(addr common.Address) int {
	for i, val := range valSet.FilteredList() {
		if addr == val.Address() {
			return i
		}
	}
	return -1
}

func (valSet *defaultSet) GetProposer() istanbul.Validator {
	return valSet.proposer
}

func (valSet *defaultSet) IsProposer(address common.Address) bool {
	_, val := valSet.GetByAddress(address)
	return reflect.DeepEqual(valSet.GetProposer(), val)
}

func (valSet *defaultSet) CalcProposer(lastProposer common.Address, round uint64) {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()
	valSet.proposer = valSet.selector(valSet, lastProposer, round, valSet.randomness)
}

func (valSet *defaultSet) AddValidators(validators []istanbul.ValidatorData) bool {
	newValidators := make([]istanbul.Validator, 0, len(validators))
	newAddressesMap := make(map[common.Address]bool)
	for i := range validators {
		address := validators[i].Address
		blsPublicKey := validators[i].BLSPublicKey

		newAddressesMap[address] = true
		newValidators = append(newValidators, New(address, blsPublicKey))
	}

	valSet.validatorMu.Lock()
	defer valSet.validatorMu.Unlock()

	// Verify that the validators to add is not already in the valset
	for _, v := range valSet.validators {
		if _, ok := newAddressesMap[v.Address()]; ok {
			return false
		}
	}

	currentValidatorIndex := 0
	for i, v := range valSet.validators {
		if currentValidatorIndex == len(newValidators) {
			break
		}
		if (v.Address() == common.Address{}) {
			valSet.validators[i] = New(newValidators[currentValidatorIndex].Address(), newValidators[currentValidatorIndex].BLSPublicKey())
			currentValidatorIndex++
		}
	}
	if currentValidatorIndex < len(newValidators) {
		valSet.validators = append(valSet.validators, newValidators[currentValidatorIndex:]...)
	}
	return true
}

func (valSet *defaultSet) RemoveValidators(removedValidators *big.Int) bool {
	if removedValidators.BitLen() == 0 || (removedValidators.BitLen() > len(valSet.validators)) {
		return true
	}

	valSet.validatorMu.Lock()
	defer valSet.validatorMu.Unlock()

	hadRemoval := false
	for i, v := range valSet.validators {
		if removedValidators.Bit(i) == 1 && (v.Address() != common.Address{}) {
			hadRemoval = true
			valSet.validators[i] = New(common.Address{}, blscrypto.SerializedPublicKey{})
		}
	}

	if !hadRemoval {
		return false
	} else {
		return true
	}
}

func (valSet *defaultSet) Copy() istanbul.ValidatorSet {
	valSet.validatorMu.RLock()
	defer valSet.validatorMu.RUnlock()

	validators := make([]istanbul.ValidatorData, 0, len(valSet.validators))
	for _, v := range valSet.validators {
		validators = append(validators, istanbul.ValidatorData{
			v.Address(),
			v.BLSPublicKey(),
		})
	}

	return NewSet(validators, valSet.policy)
}

func (valSet *defaultSet) F() int { return int(math.Ceil(float64(valSet.Size())/3)) - 1 }

func (valSet *defaultSet) MinQuorumSize() int {
	return int(math.Ceil(float64(2*valSet.Size()) / 3))
}

func (valSet *defaultSet) Policy() istanbul.ProposerPolicy { return valSet.policy }

func (valSet *defaultSet) SetRandomness(seed common.Hash) { valSet.randomness = seed }
