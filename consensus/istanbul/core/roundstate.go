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
	"io"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	// errFailedCreatePreparedCertificate is returned when there aren't enough PREPARE messages to create a PREPARED certificate.
	errFailedCreatePreparedCertificate = errors.New("failed to create PREPARED certficate")
)

// newRoundState creates a new roundState instance with the given view and validatorSet
func newRoundState(view *istanbul.View, validatorSet istanbul.ValidatorSet, preprepare *istanbul.Preprepare, pendingRequest *istanbul.Request, preparedCertificate istanbul.PreparedCertificate, parentCommits MessageSet, hasBadProposal func(hash common.Hash) bool) RoundState {
	return &roundStateImpl{
		round:               view.Round,
		desiredRound:        view.Round,
		sequence:            view.Sequence,
		preprepare:          preprepare,
		prepares:            newMessageSet(validatorSet),
		commits:             newMessageSet(validatorSet),
		parentCommits:       parentCommits,
		mu:                  new(sync.RWMutex),
		pendingRequest:      pendingRequest,
		preparedCertificate: preparedCertificate,
		hasBadProposal:      hasBadProposal,
	}
}

type RoundState interface {
	GetPrepareOrCommitSize() int
	Subject() *istanbul.Subject
	Preprepare() *istanbul.Preprepare
	SetPreprepare(preprepare *istanbul.Preprepare)
	Proposal() istanbul.Proposal
	SetRound(r *big.Int)
	Round() *big.Int
	SetDesiredRound(r *big.Int)
	DesiredRound() *big.Int
	SetSequence(seq *big.Int)
	Commits() MessageSet
	Prepares() MessageSet
	ParentCommits() MessageSet
	SetPendingRequest(pendingRequest *istanbul.Request)
	PendingRequest() *istanbul.Request
	Sequence() *big.Int
	CreateAndSetPreparedCertificate(quorumSize int) error
	PreparedCertificate() istanbul.PreparedCertificate
}

// RoundState stores the consensus state
type roundStateImpl struct {
	round               *big.Int
	desiredRound        *big.Int
	sequence            *big.Int
	preprepare          *istanbul.Preprepare
	prepares            MessageSet
	commits             MessageSet
	parentCommits       MessageSet
	pendingRequest      *istanbul.Request
	preparedCertificate istanbul.PreparedCertificate

	mu             *sync.RWMutex
	hasBadProposal func(hash common.Hash) bool
}

func (s *roundStateImpl) Commits() MessageSet {
	return s.commits
}
func (s *roundStateImpl) Prepares() MessageSet {
	return s.prepares
}
func (s *roundStateImpl) ParentCommits() MessageSet {
	return s.parentCommits
}

func (s *roundStateImpl) GetPrepareOrCommitSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := s.prepares.Size() + s.commits.Size()

	// find duplicate one
	for _, m := range s.prepares.Values() {
		if s.commits.Get(m.Address) != nil {
			result--
		}
	}
	return result
}

func (s *roundStateImpl) Subject() *istanbul.Subject {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.preprepare == nil {
		return nil
	}

	return &istanbul.Subject{
		View: &istanbul.View{
			Round:    new(big.Int).Set(s.round),
			Sequence: new(big.Int).Set(s.sequence),
		},
		Digest: s.preprepare.Proposal.Hash(),
	}
}

func (s *roundStateImpl) Preprepare() *istanbul.Preprepare {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.preprepare
}
func (s *roundStateImpl) SetPreprepare(preprepare *istanbul.Preprepare) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.preprepare = preprepare
}

func (s *roundStateImpl) Proposal() istanbul.Proposal {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.preprepare != nil {
		return s.preprepare.Proposal
	}

	return nil
}

func (s *roundStateImpl) SetRound(r *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.round = new(big.Int).Set(r)
}

func (s *roundStateImpl) Round() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.round
}

func (s *roundStateImpl) SetDesiredRound(r *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.desiredRound = new(big.Int).Set(r)
}

func (s *roundStateImpl) DesiredRound() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.desiredRound
}

func (s *roundStateImpl) SetSequence(seq *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sequence = seq
}

func (s *roundStateImpl) SetPendingRequest(pendingRequest *istanbul.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pendingRequest = pendingRequest
}

func (s *roundStateImpl) PendingRequest() *istanbul.Request {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingRequest
}

func (s *roundStateImpl) Sequence() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.sequence
}

func (s *roundStateImpl) CreateAndSetPreparedCertificate(quorumSize int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages := make([]istanbul.Message, quorumSize)
	i := 0
	for _, message := range s.prepares.Values() {
		if i == quorumSize {
			break
		}
		messages[i] = *message
		i++
	}
	for _, message := range s.commits.Values() {
		if i == quorumSize {
			break
		}
		if s.prepares.Get(message.Address) == nil {
			messages[i] = *message
			i++
		}
	}
	if i != quorumSize {
		return errFailedCreatePreparedCertificate
	}
	s.preparedCertificate = istanbul.PreparedCertificate{
		Proposal:                s.preprepare.Proposal,
		PrepareOrCommitMessages: messages,
	}
	return nil
}

func (s *roundStateImpl) PreparedCertificate() istanbul.PreparedCertificate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.preparedCertificate
}

// The DecodeRLP method should read one value from the given
// Stream. It is not forbidden to read less or more, but it might
// be confusing.
func (s *roundStateImpl) DecodeRLP(stream *rlp.Stream) error {
	var ss struct {
		Round          *big.Int
		Sequence       *big.Int
		Preprepare     *istanbul.Preprepare
		Prepares       MessageSet
		Commits        MessageSet
		pendingRequest *istanbul.Request
	}

	if err := stream.Decode(&ss); err != nil {
		return err
	}
	s.round = ss.Round
	s.sequence = ss.Sequence
	s.preprepare = ss.Preprepare
	s.prepares = ss.Prepares
	s.commits = ss.Commits
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
func (s *roundStateImpl) EncodeRLP(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return rlp.Encode(w, []interface{}{
		s.round,
		s.sequence,
		s.preprepare,
		s.prepares,
		s.commits,
		s.pendingRequest,
	})
}
