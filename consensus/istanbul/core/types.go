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
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/rlp"
)

type Engine interface {
	Start() error
	Stop() error
	// CurrentView returns the current view or nil if none
	CurrentView() *istanbul.View
	// CurrentRoundState returns the current roundState or nil if none
	CurrentRoundState() RoundState
	// CurrentRoundChangeSet returns the current round change set summary:
	// a collection of the latest round change messages from all other
	// validators.
	CurrentRoundChangeSet() *RoundChangeSetSummary

	SetAddress(common.Address)
	// Validator -> CommittedSeal from Parent Block
	ParentCommits() MessageSet
	// ForceRoundChange will force round change to the current desiredRound + 1
	ForceRoundChange()

	// ResendPreprepare sends again the preprepare message.
	ResendPreprepare() error
	// GossipPrepares gossips to other validators all the prepares received in the current round.
	GossipPrepares() error
	// GossipCommits gossips to other validators all the commits received in the current round.
	GossipCommits() error

	// DecodeMessage decodes an istanbul message
	DecodeMessage(payload []byte) (*istanbul.Message, istanbul.Validator, error)
}

// State represents the IBFT state
type State uint64

// Different IBFT Core States
const (
	StateAcceptRequest State = iota
	StatePreprepared
	StatePrepared
	StateCommitted
	StateWaitingForNewRound
)

func (s State) String() string {
	if s == StateAcceptRequest {
		return "Accept request"
	} else if s == StatePreprepared {
		return "Preprepared"
	} else if s == StatePrepared {
		return "Prepared"
	} else if s == StateCommitted {
		return "Committed"
	} else if s == StateWaitingForNewRound {
		return "Waiting for new round"
	} else {
		return "Unknown"
	}
}

// Cmp compares s and y and returns:
//   -1 if s is the previous state of y
//    0 if s and y are the same state
//   +1 if s is the next state of y
func (s State) Cmp(y State) int {
	if uint64(s) < uint64(y) {
		return -1
	}
	if uint64(s) > uint64(y) {
		return 1
	}
	return 0
}

// ==============================================
//
// helper functions

func Encode(val interface{}) ([]byte, error) {
	return rlp.EncodeToBytes(val)
}
