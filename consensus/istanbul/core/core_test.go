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

package core

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/core/types"
)

func makeBlock(number int64) *types.Block {
	header := &types.Header{
		Number:  big.NewInt(number),
		GasUsed: 0,
		Time:    uint64(0),
	}
	return types.NewBlock(header, nil, nil, nil)
}

func newTestProposalWithNum(num int64) istanbul.Proposal {
	return makeBlock(num)
}

func newTestProposal() istanbul.Proposal {
	return makeBlock(1)
}

var InvalidProposalError = errors.New("invalid proposal")



