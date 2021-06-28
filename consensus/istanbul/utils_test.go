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

package istanbul

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
)

func TestValidatorSetDiff(t *testing.T) {
	tests := []struct {
		inputOldValset      []common.Address
		inputNewValset      []common.Address
		expectedAddedVals   []common.Address
		expectedRemovedVals *big.Int
	}{
		{
			// Test validator sets that are both empty
			inputOldValset:      []common.Address{},
			inputNewValset:      []common.Address{},
			expectedAddedVals:   []common.Address{},
			expectedRemovedVals: big.NewInt(0),
		},

		{
			// Test validator sets that are the same
			inputOldValset: []common.Address{common.HexToAddress("0x64DB1B94A0304E4c27De2E758B2f962d09dFE503"),
				common.HexToAddress("0xC257274276a4E539741Ca11b590B9447B26A8051"),
				common.HexToAddress("0x2140eFD7Ba31169c69dfff6CDC66C542f0211825")},
			inputNewValset: []common.Address{common.HexToAddress("0x64DB1B94A0304E4c27De2E758B2f962d09dFE503"),
				common.HexToAddress("0xC257274276a4E539741Ca11b590B9447B26A8051"),
				common.HexToAddress("0x2140eFD7Ba31169c69dfff6CDC66C542f0211825")},
			expectedAddedVals:   []common.Address{},
			expectedRemovedVals: big.NewInt(0),
		},

		{
			// Test validator sets where one is empty
			inputOldValset: []common.Address{},
			inputNewValset: []common.Address{common.HexToAddress("0x64DB1B94A0304E4c27De2E758B2f962d09dFE503"),
				common.HexToAddress("0xC257274276a4E539741Ca11b590B9447B26A8051"),
				common.HexToAddress("0x2140eFD7Ba31169c69dfff6CDC66C542f0211825")},
			expectedAddedVals: []common.Address{common.HexToAddress("0x64DB1B94A0304E4c27De2E758B2f962d09dFE503"),
				common.HexToAddress("0xC257274276a4E539741Ca11b590B9447B26A8051"),
				common.HexToAddress("0x2140eFD7Ba31169c69dfff6CDC66C542f0211825")},
			expectedRemovedVals: big.NewInt(0),
		},

		{
			// Test validator sets where other is empty
			inputOldValset: []common.Address{common.HexToAddress("0x64DB1B94A0304E4c27De2E758B2f962d09dFE503"),
				common.HexToAddress("0xC257274276a4E539741Ca11b590B9447B26A8051"),
				common.HexToAddress("0x2140eFD7Ba31169c69dfff6CDC66C542f0211825")},
			inputNewValset:      []common.Address{},
			expectedAddedVals:   []common.Address{},
			expectedRemovedVals: big.NewInt(7), // 111, all were removed
		},

		{
			// Test validator sets that have some common elements
			inputOldValset: []common.Address{common.HexToAddress("0x64DB1B94A0304E4c27De2E758B2f962d09dFE503"),
				common.HexToAddress("0xC257274276a4E539741Ca11b590B9447B26A8051"),
				common.HexToAddress("0x2140eFD7Ba31169c69dfff6CDC66C542f0211825"),
				common.HexToAddress("0x18a00A3b357F7c309f0025dAe883170140527F76"),
				common.HexToAddress("0xaF6532a62c7c7c951129cd55078B19216E81Dad9"),
				common.HexToAddress("0x48Fa44872054C1426bdAB29834972c45D207D9DE")},
			inputNewValset: []common.Address{common.HexToAddress("0x64DB1B94A0304E4c27De2E758B2f962d09dFE503"),
				common.HexToAddress("0xC257274276a4E539741Ca11b590B9447B26A8051"),
				common.HexToAddress("0x2140eFD7Ba31169c69dfff6CDC66C542f0211825"),
				common.HexToAddress("0x31722d8C03e18a84891f45A4ECDe4444C8bE0907"),
				common.HexToAddress("0xB55A183bF5db01665f9fC5DfbA71Fc6f8b5e42e6"),
				common.HexToAddress("0x5B570EA42eBE010df95670389b93fd17d9Db9F23")},
			expectedAddedVals: []common.Address{common.HexToAddress("0x31722d8C03e18a84891f45A4ECDe4444C8bE0907"),
				common.HexToAddress("0xB55A183bF5db01665f9fC5DfbA71Fc6f8b5e42e6"),
				common.HexToAddress("0x5B570EA42eBE010df95670389b93fd17d9Db9F23")},
			expectedRemovedVals: big.NewInt(56),
		},

		{
			// Test validator sets that have no common elements
			inputOldValset: []common.Address{common.HexToAddress("0x18a00A3b357F7c309f0025dAe883170140527F76"),
				common.HexToAddress("0xaF6532a62c7c7c951129cd55078B19216E81Dad9"),
				common.HexToAddress("0x48Fa44872054C1426bdAB29834972c45D207D9DE")},
			inputNewValset: []common.Address{common.HexToAddress("0x31722d8C03e18a84891f45A4ECDe4444C8bE0907"),
				common.HexToAddress("0xB55A183bF5db01665f9fC5DfbA71Fc6f8b5e42e6"),
				common.HexToAddress("0x5B570EA42eBE010df95670389b93fd17d9Db9F23")},
			expectedAddedVals: []common.Address{common.HexToAddress("0x31722d8C03e18a84891f45A4ECDe4444C8bE0907"),
				common.HexToAddress("0xB55A183bF5db01665f9fC5DfbA71Fc6f8b5e42e6"),
				common.HexToAddress("0x5B570EA42eBE010df95670389b93fd17d9Db9F23")},
			expectedRemovedVals: big.NewInt(7), // 111, all were removed
		},
	}

	for i, tt := range tests {
		convertedInputOldValSet := []ValidatorData{}
		for _, addr := range tt.inputOldValset {
			convertedInputOldValSet = append(convertedInputOldValSet, ValidatorData{
				Address:      addr,
				BLSPublicKey: blscrypto.SerializedPublicKey{},
			})
		}
		convertedInputNewValSet := []ValidatorData{}
		for _, addr := range tt.inputNewValset {
			convertedInputNewValSet = append(convertedInputNewValSet, ValidatorData{
				Address:      addr,
				BLSPublicKey: blscrypto.SerializedPublicKey{},
			})
		}
		addedVals, removedVals := ValidatorSetDiff(convertedInputOldValSet, convertedInputNewValSet)
		addedValsAddresses, _ := SeparateValidatorDataIntoIstanbulExtra(addedVals)

		if !CompareValidatorSlices(addedValsAddresses, tt.expectedAddedVals) || removedVals.Cmp(tt.expectedRemovedVals) != 0 {
			t.Errorf("test %d failed - have: addedVals %v, removedVals %v; want: addedVals %v, removedVals %v", i, addedValsAddresses, removedVals, tt.expectedAddedVals, tt.expectedRemovedVals)
		}
	}

}

func TestGetEpochFirstBlockNumber(t *testing.T) {
	type args struct {
		epochNumber uint64
		epochSize   uint64
	}
	tests := []struct {
		name    string
		args    args
		want    uint64
		wantErr bool
	}{
		{"No epoch 0", args{0, 10}, 0, true},
		{"epoch1", args{1, 10}, 1, false},
		{"epoch2", args{2, 10}, 11, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetEpochFirstBlockNumber(tt.args.epochNumber, tt.args.epochSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetEpochFirstBlockNumber() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetEpochFirstBlockNumber() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEpochLastBlockNumber(t *testing.T) {
	type args struct {
		epochNumber uint64
		epochSize   uint64
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{"epoch 0", args{0, 10}, 0},
		{"epoch 1", args{1, 10}, 10},
		{"epoch 2", args{2, 10}, 20},
		{"epoch size 1", args{1, 1}, 1},
		{"epoch size 2", args{1, 2}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetEpochLastBlockNumber(tt.args.epochNumber, tt.args.epochSize); got != tt.want {
				t.Errorf("GetEpochLastBlockNumber() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetNumberWithinEpoch(t *testing.T) {
	type args struct {
		number    uint64
		epochSize uint64
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{"block 0", args{0, 10}, 10},
		{"block 0 other size", args{0, 15}, 15},
		{"block 1", args{1, 10}, 1},
		{"block 1 epoch 2", args{11, 10}, 1},
		{"block 5 epoch 2", args{15, 10}, 5},
		{"last block epoch 2", args{20, 10}, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetNumberWithinEpoch(tt.args.number, tt.args.epochSize); got != tt.want {
				t.Errorf("GetNumberWithinEpoch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLastBlockOfEpoch(t *testing.T) {
	type args struct {
		number    uint64
		epochSize uint64
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"genesis block", args{0, 10}, true},
		{"epoch 1, block 1", args{1, 10}, false},
		{"epoch 2, block 3", args{13, 10}, false},
		{"epoch 2, block 20", args{20, 10}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLastBlockOfEpoch(tt.args.number, tt.args.epochSize); got != tt.want {
				t.Errorf("IsLastBlockOfEpoch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsFirstBlockOfEpoch(t *testing.T) {
	type args struct {
		number    uint64
		epochSize uint64
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"genesis block", args{0, 10}, false},
		{"epoch 1, block 1", args{1, 10}, true},
		{"epoch 2, block 3", args{13, 10}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFirstBlockOfEpoch(tt.args.number, tt.args.epochSize); got != tt.want {
				t.Errorf("IsFirstBlockOfEpoch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEpochNumber(t *testing.T) {
	type args struct {
		number    uint64
		epochSize uint64
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{"genesis block", args{0, 10}, 0},
		{"epoch 1 first block", args{1, 10}, 1},
		{"epoch 1 last block", args{10, 10}, 1},
		{"epoch 2 first lbock", args{11, 10}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetEpochNumber(tt.args.number, tt.args.epochSize); got != tt.want {
				t.Errorf("GetEpochNumber() = %v, want %v", got, tt.want)
			}
		})
	}
}
