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

package istanbul

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

var (
	errInvalidValidatorSetDiffSize = errors.New("istanbul extra validator set data has different size")
)

func CombineIstanbulExtraToValidatorData(addrs []common.Address, blsPublicKeys [][]byte) ([]ValidatorData, error) {
	if len(addrs) != len(blsPublicKeys) {
		return nil, errInvalidValidatorSetDiffSize
	}
	validators := []ValidatorData{}
	for i := range addrs {
		validators = append(validators, ValidatorData{
			Address:      addrs[i],
			BLSPublicKey: blsPublicKeys[i],
		})
	}

	return validators, nil
}

func SeparateValidatorDataIntoIstanbulExtra(validators []ValidatorData) ([]common.Address, [][]byte) {
	addrs := []common.Address{}
	pubKeys := [][]byte{}
	for i := range validators {
		addrs = append(addrs, validators[i].Address)
		pubKeys = append(pubKeys, validators[i].BLSPublicKey)
	}

	return addrs, pubKeys
}

type ValidatorData struct {
	Address      common.Address
	BLSPublicKey []byte
}

type Validator interface {
	// Address returns address
	Address() common.Address

	BLSPublicKey() []byte

	// String representation of Validator
	String() string
}

func GetAddressesFromValidatorList(validators []Validator) []common.Address {
	returnList := make([]common.Address, len(validators))

	for i, val := range validators {
		returnList[i] = val.Address()
	}

	return returnList
}

// ----------------------------------------------------------------------------

type Validators []Validator

// ----------------------------------------------------------------------------

type ValidatorSet interface {
	// Sets the randomness for use in the proposer policy.
	// This is injected into the ValidatorSet when we call `getOrderedValidators`
	SetRandomness(seed common.Hash)
	// Sets the randomness for use in the proposer policy
	GetRandomness() common.Hash

	// Return the validator size
	PaddedSize() int
	Size() int
	// Get the maximum number of faulty nodes
	F() int
	// Get the minimum quorum size
	MinQuorumSize() int

	// Return the validator array
	List() []Validator
	// Return the validator array without holes
	FilteredList() []Validator
	// Return the validator index in the filtered list
	GetFilteredIndex(addr common.Address) int
	// Get validator by index
	GetByIndex(i uint64) Validator
	// Get validator by given address
	GetByAddress(addr common.Address) (int, Validator)
	// CointainByAddress indicates if a validator with the given address is present
	ContainsByAddress(add common.Address) bool

	// Add validators
	AddValidators(validators []ValidatorData) bool
	// Remove validators
	RemoveValidators(removedValidators *big.Int) bool
	// Copy validator set
	Copy() ValidatorSet
}

// ----------------------------------------------------------------------------

// ProposerSelector returns the block proposer for a round given the last proposer, round number, and randomness.
type ProposerSelector func(validatorSet ValidatorSet, lastBlockProposer common.Address, currentRound uint64) Validator
