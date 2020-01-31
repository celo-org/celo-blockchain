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
)

func TestValSetDiff(t *testing.T) {
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
				addr,
				[]byte{},
			})
		}
		convertedInputNewValSet := []ValidatorData{}
		for _, addr := range tt.inputNewValset {
			convertedInputNewValSet = append(convertedInputNewValSet, ValidatorData{
				addr,
				[]byte{},
			})
		}
		addedVals, removedVals := ValidatorSetDiff(convertedInputOldValSet, convertedInputNewValSet)
		addedValsAddresses, _ := SeparateValidatorDataIntoIstanbulExtra(addedVals)

		if !CompareValidatorSlices(addedValsAddresses, tt.expectedAddedVals) || removedVals.Cmp(tt.expectedRemovedVals) != 0 {
			t.Errorf("test %d failed - have: addedVals %v, removedVals %v; want: addedVals %v, removedVals %v", i, addedValsAddresses, removedVals, tt.expectedAddedVals, tt.expectedRemovedVals)
		}
	}

}
