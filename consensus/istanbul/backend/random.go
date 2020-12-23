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

package backend

import (
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contract_comm/random"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// String for creating the random seed
var randomSeedString = []byte("Randomness seed string")

// GenerateRandomness will generate the random beacon randomness
func (sb *Backend) GenerateRandomness(parentHash common.Hash, header *types.Header, state *state.StateDB) (common.Hash, common.Hash, error) {
	logger := sb.logger.New("func", "GenerateRandomness")

	if !random.IsRunning() {
		return common.Hash{}, common.Hash{}, nil
	}

	sb.randomSeedMu.Lock()
	if sb.randomSeed == nil {
		var err error
		sb.randomSeed, err = sb.signHashFn(accounts.Account{Address: sb.address}, common.BytesToHash(randomSeedString).Bytes())
		if err != nil {
			logger.Error("Failed to create randomSeed", "err", err)
			sb.randomSeedMu.Unlock()
			return common.Hash{}, common.Hash{}, err
		}
	}
	sb.randomSeedMu.Unlock()

	randomness := crypto.Keccak256Hash(append(sb.randomSeed, parentHash.Bytes()...))
	commitment, err := random.ComputeCommitment(header, state, randomness)
	if err != nil {
		logger.Error("Failed to compute commitment", "err", err)
		return common.Hash{}, common.Hash{}, err
	}

	return randomness, commitment, nil
}
