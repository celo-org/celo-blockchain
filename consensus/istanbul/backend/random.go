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
	"github.com/celo-org/celo-blockchain/accounts"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/random"
	"github.com/celo-org/celo-blockchain/crypto"
)

// String for creating the random seed
var randomSeedString = []byte("Randomness seed string")

// GenerateRandomness will generate the random beacon randomness
func (sb *Backend) GenerateRandomness(parentHash common.Hash) (common.Hash, common.Hash, error) {
	logger := sb.logger.New("func", "GenerateRandomness")

	// TODO(HF) check which state the vm runner should use (probably not current block's)
	vmRunner, err := sb.chain.NewEVMRunnerForCurrentBlock()
	if err != nil {
		return common.Hash{}, common.Hash{}, nil
	}

	if !random.IsRunning(vmRunner) {
		return common.Hash{}, common.Hash{}, nil
	}

	sb.randomSeedMu.Lock()
	if sb.randomSeed == nil {
		var err error
		ai := sb.authorizeInfo.Load().(*AuthorizeInfo)
		sb.randomSeed, err = ai.SignHashFn(accounts.Account{Address: ai.Address}, common.BytesToHash(randomSeedString).Bytes())
		if err != nil {
			logger.Error("Failed to create randomSeed", "err", err)
			sb.randomSeedMu.Unlock()
			return common.Hash{}, common.Hash{}, err
		}
	}
	sb.randomSeedMu.Unlock()

	randomness := crypto.Keccak256Hash(append(sb.randomSeed, parentHash.Bytes()...))

	// The logic to compute the commitment via the randomness is in the random smart contract.
	// That logic is stateless, so passing in any block header and state is fine.  There is a TODO for
	// that commitment computation logic to be removed fromthe random smart contract.
	commitment, err := random.ComputeCommitment(vmRunner, randomness)
	if err != nil {
		logger.Error("Failed to compute commitment", "err", err)
		return common.Hash{}, common.Hash{}, err
	}

	return randomness, commitment, nil
}
