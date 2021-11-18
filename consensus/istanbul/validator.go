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
	"bytes"
	"errors"
	"fmt"
	"math/big"

	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"

	"github.com/celo-org/celo-blockchain/common"
)

var (
	errInvalidValidatorSetDiffSize = errors.New("istanbul extra validator set data has different size")
)

type ValidatorData struct {
	Address      common.Address
	BLSPublicKey blscrypto.SerializedPublicKey
}

type ValidatorDataWithBLSKeyCache struct {
	Address                  common.Address
	BLSPublicKey             blscrypto.SerializedPublicKey
	UncompressedBLSPublicKey []byte
}

type Validator interface {
	fmt.Stringer

	// Address returns address
	Address() common.Address

	// BLSPublicKey returns the BLS public key (compressed format)
	BLSPublicKey() blscrypto.SerializedPublicKey

	// BLSPublicKeyUncompressed returns the BLS public key (uncompressed format)
	BLSPublicKeyUncompressed() []byte

	// Serialize returns binary reprenstation of the Validator
	// can be use used to instantiate a validator with DeserializeValidator()
	Serialize() ([]byte, error)

	// AsData returns Validator representation as ValidatorData
	AsData() *ValidatorData

	// AsData returns Validator representation as ValidatorData
	AsDataWithBLSKeyCache() *ValidatorDataWithBLSKeyCache

	// CacheUncompressedBLSKey stores the uncompressed BLS public key to cache
	CacheUncompressedBLSKey()

	// Copy validator
	Copy() Validator
}

// MapValidatorsToAddresses maps a slice of validator to a slice of addresses
func MapValidatorsToAddresses(validators []Validator) []common.Address {
	returnList := make([]common.Address, len(validators))

	for i, val := range validators {
		returnList[i] = val.Address()
	}

	return returnList
}

// MapValidatorsToPublicKeys maps a slice of validator to a slice of public keys
func MapValidatorsToPublicKeys(validators []Validator) []blscrypto.SerializedPublicKey {
	returnList := make([]blscrypto.SerializedPublicKey, len(validators))

	for i, val := range validators {
		returnList[i] = val.BLSPublicKey()
	}

	return returnList
}

// ----------------------------------------------------------------------------

type ValidatorsDataByAddress []ValidatorData

func (a ValidatorsDataByAddress) Len() int      { return len(a) }
func (a ValidatorsDataByAddress) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ValidatorsDataByAddress) Less(i, j int) bool {
	return bytes.Compare(a[i].Address[:], a[j].Address[:]) < 0
}

// ----------------------------------------------------------------------------

type ValidatorSet interface {
	fmt.Stringer

	// Sets the randomness for use in the proposer policy.
	// This is injected into the ValidatorSet when we call `getOrderedValidators`
	SetRandomness(seed common.Hash)
	// Sets the randomness for use in the proposer policy
	GetRandomness() common.Hash

	// Return the validator size
	Size() int
	// Get the maximum number of faulty nodes
	F() int
	// Get the minimum quorum size
	MinQuorumSize() int

	// List returns all the validators
	List() []Validator
	// Return the validator index
	GetIndex(addr common.Address) int
	// Get validator by index
	GetByIndex(i uint64) Validator

	// Get validator by given address, returns the index of the validator and
	// the validator. If there is no validator with the given address then -1
	// and nil are returned.
	GetByAddress(addr common.Address) (int, Validator)
	// CointainByAddress indicates if a validator with the given address is present
	ContainsByAddress(add common.Address) bool

	// Add validators
	AddValidators(validators []ValidatorData) bool
	// Remove validators
	RemoveValidators(removedValidators *big.Int) bool
	// Copy validator set
	Copy() ValidatorSet

	// CacheUncompressedBLSKey stores the uncompressed BLS public key to cache for each validator in the valset
	CacheUncompressedBLSKey()

	// HasBLSKeyCache tests that all uncompressed BLS public keys are in the cache, otherwise returns false
	HasBLSKeyCache() bool

	// Serialize returns binary reprentation of the ValidatorSet
	// can be use used to instantiate a validator with DeserializeValidatorSet()
	Serialize() ([]byte, error)
}

type ValidatorSetData struct {
	Validators []ValidatorData
	Randomness common.Hash
}

type ValidatorSetDataWithBLSKeyCache struct {
	Validators []ValidatorDataWithBLSKeyCache
	Randomness common.Hash
}

// ----------------------------------------------------------------------------

// ProposerSelector returns the block proposer for a round given the last proposer, round number, and randomness.
type ProposerSelector func(validatorSet ValidatorSet, lastBlockProposer common.Address, currentRound uint64) Validator

// ----------------------------------------------------------------------------

func CombineIstanbulExtraToValidatorData(addrs []common.Address, blsPublicKeys []blscrypto.SerializedPublicKey) ([]ValidatorData, error) {
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

func SeparateValidatorDataIntoIstanbulExtra(validators []ValidatorData) ([]common.Address, []blscrypto.SerializedPublicKey) {
	addrs := []common.Address{}
	pubKeys := []blscrypto.SerializedPublicKey{}
	for i := range validators {
		addrs = append(addrs, validators[i].Address)
		pubKeys = append(pubKeys, validators[i].BLSPublicKey)
	}

	return addrs, pubKeys
}
