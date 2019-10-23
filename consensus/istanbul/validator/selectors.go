// Copyright 2019 The Celo Authors
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

package validator

import (
	"encoding/binary"
	"math/rand"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

func proposerIndex(valSet istanbul.ValidatorSet, proposer common.Address) uint64 {
	if idx := valSet.GetFilteredIndex(proposer); idx >= 0 {
		return uint64(idx)
	}
	return 0
}

// TODO: Pull ordering from smart contract and deprecate this function.
func randFromHash(hash common.Hash) *rand.Rand {
	// Reduce the hash to 64 bits to use as the seed.
	var seed uint64
	for i := 0; i < common.HashLength; i += 8 {
		seed ^= binary.BigEndian.Uint64(hash[i : i+8])
	}
	return rand.New(rand.NewSource(int64(seed)))
}

// ShuffledRoundRobinProposer selects the next proposer with a round robin strategy according to a shuffled order.
func ShuffledRoundRobinProposer(valSet istanbul.ValidatorSet, proposer common.Address, round uint64, seed common.Hash) istanbul.Validator {
	if valSet.Size() == 0 {
		return nil
	}
	shuffle := randFromHash(seed).Perm(valSet.Size())
	idx := round
	if proposer != (common.Address{}) {
		idx += proposerIndex(valSet, proposer) + 1
	}
	return valSet.FilteredList()[shuffle[idx%uint64(valSet.Size())]]
}

// RoundRobinProposer selects the next proposer with a round robin strategy according to storage order.
func RoundRobinProposer(valSet istanbul.ValidatorSet, proposer common.Address, round uint64, _ common.Hash) istanbul.Validator {
	if valSet.Size() == 0 {
		return nil
	}
	idx := round
	if proposer != (common.Address{}) {
		idx += proposerIndex(valSet, proposer) + 1
	}
	return valSet.FilteredList()[idx%uint64(valSet.Size())]
}

// StickyProposer selects the next proposer with a sticky strategy, advancing on round change.
func StickyProposer(valSet istanbul.ValidatorSet, proposer common.Address, round uint64, _ common.Hash) istanbul.Validator {
	if valSet.Size() == 0 {
		return nil
	}
	idx := round
	if proposer != (common.Address{}) {
		idx += proposerIndex(valSet, proposer)
	}
	return valSet.FilteredList()[idx%uint64(valSet.Size())]
}
