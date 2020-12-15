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
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"

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

	return common.Address{}, fmt.Errorf("not an elected validator %s", signer.Hex())
}

// GetNumberWithinEpoch retrieves the block number within an epoch.
// The return value will be 1-based; thus first block of epoch is 1, and last block of epoch is `epochSize`
// There is a special case if the number == 0. It is basically the last block of the 0th epoch, and should have a value of epochSize
func GetNumberWithinEpoch(number uint64, epochSize uint64) uint64 {
	number = number % epochSize
	if number == 0 {
		return epochSize
	}
	return number
}

// IsLastBlockOfEpoch indicates if block number is the last block of its epoch
func IsLastBlockOfEpoch(number uint64, epochSize uint64) bool {
	return GetNumberWithinEpoch(number, epochSize) == epochSize
}

// IsFirstBlockOfEpoch indicates if block number is the first block of its epoch
func IsFirstBlockOfEpoch(number uint64, epochSize uint64) bool {
	return GetNumberWithinEpoch(number, epochSize) == 1
}

// GetEpochNumber retrieves the epoch number given the block number.
// Epoch 0 is a special block that only contains the genesis block (block 0), epoch 1
// starts at block 1
func GetEpochNumber(number uint64, epochSize uint64) uint64 {
	epochNumber := number / epochSize
	if IsLastBlockOfEpoch(number, epochSize) {
		return epochNumber
	}
	return epochNumber + 1
}

// GetEpochFirstBlockNumber retrieves first block of epoch.
func GetEpochFirstBlockNumber(epochNumber uint64, epochSize uint64) (uint64, error) {
	// Epoch 0 is just the genesis block, it doesn't have a first block (only last)
	if epochNumber == 0 {
		return 0, errors.New("No first block for epoch 0")
	}

	return ((epochNumber - 1) * epochSize) + 1, nil
}

// GetEpochLastBlockNumber retrieves last block of epoch
func GetEpochLastBlockNumber(epochNumber uint64, epochSize uint64) uint64 {
	if epochNumber == 0 {
		return 0
	}
	// Epoch 0 is just the genesis bock, so epoch 1 starts at block 1 and ends at block epochSize
	// And from then on, it's epochSize more for each epoch
	return epochNumber * epochSize
}

// GetUptimeMonitoringWindow retrieves the range [first block, last block] where uptime is to be monitored
// for a give epoch. The range is inclusive.
// First blocks of an epoch need to be skipped since we can't assess the last `lookbackWindow` block for validators
// as those are froma different epoch.
// Similarly, last block of epoch is skipped since we can't obtaine the signer for it; as they are in the next block
func GetUptimeMonitoringWindow(epochNumber uint64, epochSize uint64, lookbackWindowSize uint64) (uint64, uint64) {
	if epochNumber == 0 {
		panic("no monitoring window for epoch 0")
	}

	epochFirstBlock, _ := GetEpochFirstBlockNumber(epochNumber, epochSize)
	epochLastBlock := GetEpochLastBlockNumber(epochNumber, epochSize)

	// first block to monitor:
	// We need to wait for the completion of the first window with the start window's block being the
	// 2nd block of the epoch, before we start tallying the validator score for epoch "epochNumber".
	// We can't include the epoch's first block since it's aggregated parent seals
	// is for the previous epoch's valset.
	firstBlockToMonitor := epochFirstBlock + 1 + (lookbackWindowSize - 1)

	// last block to monitor:
	// We stop tallying for epoch "epochNumber" at the second to last block of that epoch.
	// We can't include that epoch's last block as part of the tally because the epoch val score is calculated
	// using a tally that is updated AFTER a block is finalized.
	// Note that it's possible to count up to the last block of the epoch, but it's much harder to implement
	// than couting up to the second to last one.
	lastBlockToMonitor := epochLastBlock - 1

	return firstBlockToMonitor, lastBlockToMonitor
}

func ValidatorSetDiff(oldValSet []ValidatorData, newValSet []ValidatorData) ([]ValidatorData, *big.Int) {
	valSetMap := make(map[common.Address]bool)
	oldValSetIndices := make(map[common.Address]int)

	for i, oldVal := range oldValSet {
		if (oldVal.Address != common.Address{}) {
			valSetMap[oldVal.Address] = true
			oldValSetIndices[oldValSet[i].Address] = i
		}
	}

	var addedValidators []ValidatorData
	for _, newVal := range newValSet {
		index, ok := oldValSetIndices[newVal.Address]
		if ok && (oldValSet[index].BLSPublicKey == newVal.BLSPublicKey) {
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

	removedValidatorsBitmap := big.NewInt(0)
	for rmVal := range valSetMap {
		removedValidatorsBitmap = removedValidatorsBitmap.SetBit(removedValidatorsBitmap, oldValSetIndices[rmVal], 1)
	}

	return addedValidators, removedValidatorsBitmap
}

// CompareValidatorSlices compares 2 validator slices and indicate if they are equal.
// Equality is defined as: valseSet1[i] must be equal to valSet2[i] for every i.
// (aka. order matters)
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

func CompareValidatorPublicKeySlices(valSet1 []blscrypto.SerializedPublicKey, valSet2 []blscrypto.SerializedPublicKey) bool {
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

func ConvertPublicKeysToStringSlice(publicKeys []blscrypto.SerializedPublicKey) []string {
	publicKeyStrs := []string{}
	for i := 0; i < len(publicKeys); i++ {
		publicKeyStrs = append(publicKeyStrs, hex.EncodeToString(publicKeys[i][:]))
	}

	return publicKeyStrs
}

func GetNodeID(enodeURL string) (*enode.ID, error) {
	node, err := enode.ParseV4(enodeURL)
	if err != nil {
		return nil, err
	}

	id := node.ID()
	return &id, nil
}

// RandomnessCommitmentDBLocation will return the key for where the
// given commitment's cached key-value entry
func RandomnessCommitmentDBLocation(commitment common.Hash) []byte {
	dbRandomnessPrefix := []byte("db-randomness-prefix")
	return append(dbRandomnessPrefix, commitment.Bytes()...)
}
