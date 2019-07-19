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
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/sha3"
)

func RLPHash(v interface{}) (h common.Hash) {
	hw := sha3.NewLegacyKeccak256()
	rlp.Encode(hw, v)
	hw.Sum(h[:0])
	return h
}

// GetSignatureAddress gets the signer address from the signature
func GetSignatureAddress(data []byte, sig []byte) (common.Address, error) {
	// 1. Keccak data
	hashData := crypto.Keccak256(data)
	// 2. Recover public key
	pubkey, err := crypto.SigToPub(hashData, sig)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*pubkey), nil
}

func CheckValidatorSignature(valSet ValidatorSet, data []byte, sig []byte) (common.Address, error) {
	// 1. Get signature address
	signer, err := GetSignatureAddress(data, sig)
	if err != nil {
		log.Error("Failed to get signer address", "err", err)
		return common.Address{}, err
	}

	// 2. Check validator
	if _, val := valSet.GetByAddress(signer); val != nil {
		return val.Address(), nil
	}

	return common.Address{}, ErrUnauthorizedAddress
}

// Retrieves the block number within an epoch.  The return value will be 1-based.
// There is a special case if the number == 0.  It is basically the last block of the 0th epoch, and should have a value of epochSize
func getNumberWithinEpoch(number uint64, epochSize uint64) uint64 {
	number = number % epochSize
	if number == 0 {
		return epochSize
	} else {
		return number
	}
}

func IsLastBlockOfEpoch(number uint64, epochSize uint64) bool {
	return getNumberWithinEpoch(number, epochSize) == epochSize
}

// Retrieves the epoch number given the block number.
// There is a special case if the number == 0 (the genesis block).  That block will be in the
// 1st epoch.
func GetEpochNumber(number uint64, epochSize uint64) uint64 {
	if number == 0 {
		return 0
	} else {
		return (number / epochSize) + 1
	}
}

func GetEpochFirstBlockNumber(epochNumber uint64, epochSize uint64) (uint64, error) {
	if epochNumber == 0 {
		return 0, errors.New("No first block for epoch 0")
	}

	return ((epochNumber - 1) * epochSize) + 1, nil
}

func GetEpochLastBlockNumber(epochNumber uint64, epochSize uint64) uint64 {
	if epochNumber == 0 {
		return 0
	}

	firstBlockNum, _ := GetEpochFirstBlockNumber(epochNumber, epochSize)
	return firstBlockNum + (epochSize - 1)
}

func ValidatorSetDiff(oldValSet []ValidatorData, newValSet []ValidatorData) ([]ValidatorData, []ValidatorData) {
	valSetMap := make(map[common.Address]bool)

	for _, oldVal := range oldValSet {
		valSetMap[oldVal.Address] = true
	}

	oldValSetToPubKey := make(map[common.Address][]byte)
	for i := range oldValSet {
		oldValSetToPubKey[oldValSet[i].Address] = oldValSet[i].BLSPublicKey
	}

	var addedValidators []ValidatorData
	for _, newVal := range newValSet {
		if _, ok := valSetMap[newVal.Address]; ok {
			// We found a common validator.  Pop from the map
			delete(valSetMap, newVal.Address)
		} else {
			// We found a new validator that is not in the old validator set
			addedValidators = append(addedValidators, ValidatorData{
				newVal.Address,
				newVal.BLSPublicKey,
			})
		}
	}
	sort.Slice(addedValidators, func(i, j int) bool {
		return strings.Compare(addedValidators[i].Address.String(), addedValidators[j].Address.String()) < 0
	})

	// Any remaining validators in the map are the removed validators
	removedValidators := make([]ValidatorData, 0, len(valSetMap))
	for rmVal := range valSetMap {
		removedValidators = append(removedValidators, ValidatorData{
			rmVal,
			oldValSetToPubKey[rmVal],
		})
	}

	sort.Slice(removedValidators, func(i, j int) bool {
		return strings.Compare(removedValidators[i].Address.String(), removedValidators[j].Address.String()) < 0
	})

	return addedValidators, removedValidators
}

// This function assumes that valSet1 and valSet2 are sorted
func CompareValidatorSlices(valSet1 []common.Address, valSet2 []common.Address) bool {
	if len(valSet1) != len(valSet2) {
		return false
	}

	for i := 0; i < len(valSet1); i++ {
		if valSet1[i] != valSet2[i] {
			return false
		}
	}

	return true
}

func GetNodeID(enodeURL string) (*enode.ID, error) {
	node, err := enode.ParseV4(enodeURL)
	if err != nil {
		return nil, err
	}

	id := node.ID()
	return &id, nil
}
