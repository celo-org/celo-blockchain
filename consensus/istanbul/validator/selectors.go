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
	"fmt"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

func proposerIndex(valSet istanbul.ValidatorSet, proposer common.Address) uint64 {
	if idx := valSet.GetIndex(proposer); idx >= 0 {
		return uint64(idx)
	}
	return 0
}

// RoundRobinProposer selects the next proposer with a round robin strategy according to storage order.
func RoundRobinProposer(valSet istanbul.ValidatorSet, proposer common.Address, round uint64) istanbul.Validator {
	if valSet.Size() == 0 {
		return nil
	}
	idx := round
	if proposer != (common.Address{}) {
		idx += proposerIndex(valSet, proposer) + 1
	}
	return valSet.List()[idx%uint64(valSet.Size())]
}

// StickyProposer selects the next proposer with a sticky strategy, advancing on round change.
func StickyProposer(valSet istanbul.ValidatorSet, proposer common.Address, round uint64) istanbul.Validator {
	if valSet.Size() == 0 {
		return nil
	}
	idx := round
	if proposer != (common.Address{}) {
		idx += proposerIndex(valSet, proposer)
	}
	return valSet.List()[idx%uint64(valSet.Size())]
}

// GetProposerSelector returns the ProposerSelector for the given Policy
func GetProposerSelector(pp istanbul.ProposerPolicy) istanbul.ProposerSelector {
	switch pp {
	case istanbul.Sticky:
		return StickyProposer
	case istanbul.RoundRobin:
		return RoundRobinProposer
	default:
		// Programming error.
		panic(fmt.Sprintf("unknown proposer selection policy: %v", pp))
	}
}
