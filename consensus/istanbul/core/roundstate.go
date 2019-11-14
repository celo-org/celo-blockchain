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
	"io"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/rlp"
)

// newRoundState creates a new roundState instance with the given view and validatorSet
func newRoundState(view *istanbul.View, validatorSet istanbul.ValidatorSet, preprepare *istanbul.Preprepare, pendingRequest *istanbul.Request, preparedCertificate istanbul.PreparedCertificate, parentCommits *messageSet, hasBadProposal func(hash common.Hash) bool) *roundState {
	return &roundState{
		round:               view.Round,
		desiredRound:        view.Round,
		sequence:            view.Sequence,
		Preprepare:          preprepare,
		Prepares:            newMessageSet(validatorSet),
		Commits:             newMessageSet(validatorSet),
		ParentCommits:       parentCommits,
		mu:                  new(sync.RWMutex),
		pendingRequest:      pendingRequest,
		preparedCertificate: preparedCertificate,
		hasBadProposal:      hasBadProposal,
	}
}

// roundState stores the consensus state
type roundState struct {
	round               *big.Int
	desiredRound        *big.Int
	sequence            *big.Int
	Preprepare          *istanbul.Preprepare
	Prepares            *messageSet
	Commits             *messageSet
	ParentCommits       *messageSet
	pendingRequest      *istanbul.Request
	preparedCertificate istanbul.PreparedCertificate

	mu             *sync.RWMutex
	hasBadProposal func(hash common.Hash) bool
}

func (s *roundState) GetPrepareOrCommitSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := s.Prepares.Size() + s.Commits.Size()

	// find duplicate one
	for _, m := range s.Prepares.Values() {
		if s.Commits.Get(m.Address) != nil {
			result--
		}
	}
	return result
}

func (s *roundState) Subject() *istanbul.Subject {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Preprepare == nil {
		return nil
	}

	return &istanbul.Subject{
		View: &istanbul.View{
			Round:    new(big.Int).Set(s.round),
			Sequence: new(big.Int).Set(s.sequence),
		},
		Digest: s.Preprepare.Proposal.Hash(),
	}
}

func (s *roundState) SetPreprepare(preprepare *istanbul.Preprepare) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Preprepare = preprepare
}

func (s *roundState) Proposal() istanbul.Proposal {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Preprepare != nil {
		return s.Preprepare.Proposal
	}

	return nil
}

func (s *roundState) SetRound(r *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.round = new(big.Int).Set(r)
}

func (s *roundState) Round() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.round
}

func (s *roundState) SetDesiredRound(r *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.desiredRound = new(big.Int).Set(r)
}

func (s *roundState) DesiredRound() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.desiredRound
}

func (s *roundState) SetSequence(seq *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sequence = seq
}

func (s *roundState) Sequence() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.sequence
}

func (s *roundState) CreateAndSetPreparedCertificate(quorumSize int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages := make([]istanbul.Message, quorumSize)
	i := 0
	for _, message := range s.Prepares.Values() {
		if i == quorumSize {
			break
		}
		messages[i] = *message
		i++
	}
	for _, message := range s.Commits.Values() {
		if i == quorumSize {
			break
		}
		if s.Prepares.Get(message.Address) == nil {
			messages[i] = *message
			i++
		}
	}
	if i != quorumSize {
		return errFailedCreatePreparedCertificate
	}
	s.preparedCertificate = istanbul.PreparedCertificate{
		Proposal:                s.Preprepare.Proposal,
		PrepareOrCommitMessages: messages,
	}
	return nil
}

// The DecodeRLP method should read one value from the given
// Stream. It is not forbidden to read less or more, but it might
// be confusing.
func (s *roundState) DecodeRLP(stream *rlp.Stream) error {
	var ss struct {
		Round          *big.Int
		Sequence       *big.Int
		Preprepare     *istanbul.Preprepare
		Prepares       *messageSet
		Commits        *messageSet
		pendingRequest *istanbul.Request
	}

	if err := stream.Decode(&ss); err != nil {
		return err
	}
	s.round = ss.Round
	s.sequence = ss.Sequence
	s.Preprepare = ss.Preprepare
	s.Prepares = ss.Prepares
	s.Commits = ss.Commits
	s.pendingRequest = ss.pendingRequest
	s.mu = new(sync.RWMutex)

	return nil
}

// EncodeRLP should write the RLP encoding of its receiver to w.
// If the implementation is a pointer method, it may also be
// called for nil pointers.
//
// Implementations should generate valid RLP. The data written is
// not verified at the moment, but a future version might. It is
// recommended to write only a single value but writing multiple
// values or no value at all is also permitted.
func (s *roundState) EncodeRLP(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return rlp.Encode(w, []interface{}{
		s.round,
		s.sequence,
		s.Preprepare,
		s.Prepares,
		s.Commits,
		s.pendingRequest,
	})
}
