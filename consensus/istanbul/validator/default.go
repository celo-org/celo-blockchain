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
	"math"
	"math/big"
	"reflect"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

type defaultValidator struct {
	address      common.Address
	blsPublicKey []byte
}

func (val *defaultValidator) Address() common.Address {
	return val.address
}

func (val *defaultValidator) BLSPublicKey() []byte {
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
	selector    istanbul.ProposalSelector
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
	valSet.selector = roundRobinProposer
	if policy == istanbul.Sticky {
		valSet.selector = stickyProposer
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
	valSet.proposer = valSet.selector(valSet, lastProposer, round)
}

func calcSeed(valSet istanbul.ValidatorSet, proposer common.Address, round uint64) uint64 {
	offset := 0
	if idx, val := valSet.GetByAddress(proposer); val != nil {
		offset = idx
	}
	return uint64(offset) + round
}

func emptyAddress(addr common.Address) bool {
	return addr == common.Address{}
}

func roundRobinProposer(valSet istanbul.ValidatorSet, proposer common.Address, round uint64) istanbul.Validator {
	if valSet.Size() == 0 {
		return nil
	}
	seed := uint64(0)
	if emptyAddress(proposer) {
		seed = round
	} else {
		seed = calcSeed(valSet, proposer, round) + 1
	}

	filteredList := []istanbul.Validator{}
	for i := 0; i < valSet.PaddedSize(); i++ {
		currentValidator := valSet.GetByIndex(uint64(i))
		if (currentValidator.Address() != common.Address{}) {
			filteredList = append(filteredList, currentValidator)
		}
	}
	pick := seed % uint64(valSet.Size())
	return filteredList[pick]
}

func stickyProposer(valSet istanbul.ValidatorSet, proposer common.Address, round uint64) istanbul.Validator {
	if valSet.Size() == 0 {
		return nil
	}
	seed := uint64(0)
	if emptyAddress(proposer) {
		seed = round
	} else {
		seed = calcSeed(valSet, proposer, round)
	}

	filteredList := []istanbul.Validator{}
	for i := 0; i < valSet.PaddedSize(); i++ {
		currentValidator := valSet.GetByIndex(uint64(i))
		if (currentValidator.Address() != common.Address{}) {
			filteredList = append(filteredList, currentValidator)
		}
	}
	pick := seed % uint64(valSet.Size())
	return filteredList[pick]
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
	if removedValidators.BitLen() == 0 {
		return true
	}

	valSet.validatorMu.Lock()
	defer valSet.validatorMu.Unlock()

	removeAddressesMap := make(map[common.Address]bool)
	removeAddressesIndexMap := make(map[common.Address]int)
	for i, v := range valSet.validators {
		if removedValidators.Bit(i) == 1 {
			removeAddressesIndexMap[v.Address()] = i
			removeAddressesMap[v.Address()] = true
		}
	}

	for _, v := range valSet.validators {
		if _, ok := removeAddressesMap[v.Address()]; ok {
			if (v.Address() != common.Address{}) {
				delete(removeAddressesMap, v.Address())
				valSet.validators[removeAddressesIndexMap[v.Address()]] = New(common.Address{}, nil)
			}
		}
	}

	if len(removeAddressesMap) > 0 {
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

func (valSet *defaultSet) Policy() istanbul.ProposerPolicy { return valSet.policy }
