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
	"bytes"
	"errors"
	"io"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	// errFailedCreatePreparedCertificate is returned when there aren't enough PREPARE messages to create a PREPARED certificate.
	errFailedCreatePreparedCertificate = errors.New("failed to create PREPARED certficate")
)

type RoundState interface {
	// mutation functions
	StartNewRound(nextRound *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator) error
	StartNewSequence(nextSequence *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator, parentCommits MessageSet) error
	TransitionToPreprepared(preprepare *istanbul.Preprepare) error
	TransitionToWaitingForNewRound(r *big.Int, nextProposer istanbul.Validator) error
	TransitionToCommited() error
	TransitionToPrepared(quorumSize int) error
	AddCommit(msg *istanbul.Message) error
	AddPrepare(msg *istanbul.Message) error
	AddParentCommit(msg *istanbul.Message) error
	SetPendingRequest(pendingRequest *istanbul.Request) error

	// view functions
	DesiredRound() *big.Int
	State() State
	GetPrepareOrCommitSize() int
	GetValidatorByAddress(address common.Address) istanbul.Validator
	ValidatorSet() istanbul.ValidatorSet
	Proposer() istanbul.Validator
	IsProposer(address common.Address) bool
	Subject() *istanbul.Subject
	Preprepare() *istanbul.Preprepare
	Proposal() istanbul.Proposal
	Round() *big.Int
	Commits() MessageSet
	Prepares() MessageSet
	ParentCommits() MessageSet
	PendingRequest() *istanbul.Request
	Sequence() *big.Int
	View() *istanbul.View
	PreparedCertificate() istanbul.PreparedCertificate

	// Close performs any necessary action before erasing RoundState
	Close() error
}

// RoundState stores the consensus state
type roundStateImpl struct {
	state        State
	round        *big.Int
	desiredRound *big.Int
	sequence     *big.Int

	// data for current round
	preprepare *istanbul.Preprepare
	prepares   MessageSet
	commits    MessageSet
	proposer   istanbul.Validator

	// data saves across rounds, same sequence
	validatorSet        istanbul.ValidatorSet
	parentCommits       MessageSet
	pendingRequest      *istanbul.Request
	preparedCertificate istanbul.PreparedCertificate

	mu     *sync.RWMutex
	logger log.Logger
}

func newRoundState(view *istanbul.View, validatorSet istanbul.ValidatorSet, proposer istanbul.Validator) RoundState {
	return &roundStateImpl{
		state:        StateAcceptRequest,
		round:        view.Round,
		desiredRound: view.Round,
		sequence:     view.Sequence,

		// data for current round
		// preprepare: nil,
		prepares: newMessageSet(validatorSet),
		commits:  newMessageSet(validatorSet),
		proposer: proposer,

		// data saves across rounds, same sequence
		validatorSet:        validatorSet,
		parentCommits:       newMessageSet(validatorSet),
		pendingRequest:      nil,
		preparedCertificate: istanbul.EmptyPreparedCertificate(),

		mu:     new(sync.RWMutex),
		logger: log.New(),
	}
}

func (s *roundStateImpl) Commits() MessageSet       { return s.commits }
func (s *roundStateImpl) Prepares() MessageSet      { return s.prepares }
func (s *roundStateImpl) ParentCommits() MessageSet { return s.parentCommits }

func (s *roundStateImpl) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *roundStateImpl) View() *istanbul.View {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &istanbul.View{
		Sequence: new(big.Int).Set(s.sequence),
		Round:    new(big.Int).Set(s.round),
	}
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

func (s *roundStateImpl) IsProposer(address common.Address) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.proposer.Address() == address
}

func (s *roundStateImpl) Proposer() istanbul.Validator {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.proposer
}

func (s *roundStateImpl) ValidatorSet() istanbul.ValidatorSet {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.validatorSet
}

func (s *roundStateImpl) GetValidatorByAddress(address common.Address) istanbul.Validator {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, validator := s.validatorSet.GetByAddress(address)
	return validator
}

func (s *roundStateImpl) Preprepare() *istanbul.Preprepare {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.preprepare
}

func (s *roundStateImpl) Proposal() istanbul.Proposal {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.preprepare != nil {
		return s.preprepare.Proposal
	}

	return nil
}

func (s *roundStateImpl) Round() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.round
}

func (s *roundStateImpl) changeRound(nextRound *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator) {
	s.state = StateAcceptRequest
	s.round = nextRound
	s.desiredRound = nextRound

	// TODO MC use old valset
	s.prepares = newMessageSet(validatorSet)
	s.commits = newMessageSet(validatorSet)
	s.proposer = nextProposer

	// ??
	s.preprepare = nil
}

func (s *roundStateImpl) StartNewRound(nextRound *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	logger := s.newLogger()
	s.changeRound(nextRound, validatorSet, nextProposer)
	logger.Debug("Starting new round", "next_round", nextRound, "next_proposer", nextProposer.Address().Hex())
	return nil
}

func (s *roundStateImpl) StartNewSequence(nextSequence *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator, parentCommits MessageSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	logger := s.newLogger()

	s.validatorSet = validatorSet

	s.changeRound(big.NewInt(0), validatorSet, nextProposer)

	s.sequence = nextSequence
	s.preparedCertificate = istanbul.EmptyPreparedCertificate()
	s.pendingRequest = nil
	s.parentCommits = parentCommits

	logger.Debug("Starting new sequence", "next_sequence", nextSequence, "next_proposer", nextProposer.Address().Hex())
	return nil
}

func (s *roundStateImpl) TransitionToCommited() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = StateCommitted
	return nil
}

func (s *roundStateImpl) TransitionToPreprepared(preprepare *istanbul.Preprepare) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.preprepare = preprepare
	s.state = StatePreprepared
	return nil
}

func (s *roundStateImpl) TransitionToWaitingForNewRound(r *big.Int, nextProposer istanbul.Validator) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.desiredRound = new(big.Int).Set(r)
	s.proposer = nextProposer
	s.state = StateWaitingForNewRound
	return nil
}

// TransitionToPrepared will create a PreparedCertificate and change state to Prepared
func (s *roundStateImpl) TransitionToPrepared(quorumSize int) error {
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

	s.state = StatePrepared
	return nil
}

func (s *roundStateImpl) AddCommit(msg *istanbul.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commits.Add(msg)
}

func (s *roundStateImpl) AddPrepare(msg *istanbul.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prepares.Add(msg)
}

func (s *roundStateImpl) AddParentCommit(msg *istanbul.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.parentCommits.Add(msg)
}

func (s *roundStateImpl) DesiredRound() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.desiredRound
}

func (s *roundStateImpl) SetPendingRequest(pendingRequest *istanbul.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pendingRequest = pendingRequest
	return nil
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

func (s *roundStateImpl) PreparedCertificate() istanbul.PreparedCertificate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.preparedCertificate
}

func (s *roundStateImpl) Close() error {
	return nil
}

func (s *roundStateImpl) newLogger(ctx ...interface{}) log.Logger {
	logger := s.logger.New(ctx...)
	return logger.New("cur_seq", s.sequence, "cur_round", s.round, "state", s.state)
}

type roundStateRLP struct {
	State               State
	Round               *big.Int
	DesiredRound        *big.Int
	Sequence            *big.Int
	PreparedCertificate *istanbul.PreparedCertificate

	// custom serialized fields
	SerializedValSet         []byte
	SerializedProposer       []byte
	SerializedParentCommits  []byte
	SerializedPrepares       []byte
	SerializedCommits        []byte
	SerializedPreprepare     []byte
	SerializedPendingRequest []byte
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

	serializedValSet, err := s.validatorSet.Serialize()
	if err != nil {
		return err
	}
	serializedProposer, err := s.proposer.Serialize()
	if err != nil {
		return err
	}

	serializedParentCommits, err := s.parentCommits.Serialize()
	if err != nil {
		return err
	}
	serializedPrepares, err := s.prepares.Serialize()
	if err != nil {
		return err
	}
	serializedCommits, err := s.commits.Serialize()
	if err != nil {
		return err
	}

	// handle nullable field. Serialized them to rlp.EmptyList or the rlp version of them
	var serializedPendingRequest []byte
	if s.pendingRequest == nil {
		serializedPendingRequest = rlp.EmptyList
	} else {
		serializedPendingRequest, err = rlp.EncodeToBytes(s.pendingRequest)
		if err != nil {
			return err
		}
	}

	var serializedPreprepare []byte
	if s.preprepare == nil {
		serializedPreprepare = rlp.EmptyList
	} else {
		serializedPreprepare, err = rlp.EncodeToBytes(s.preprepare)
		if err != nil {
			return err
		}
	}

	entry := roundStateRLP{
		State:               s.state,
		Round:               s.round,
		DesiredRound:        s.desiredRound,
		Sequence:            s.sequence,
		PreparedCertificate: &s.preparedCertificate,

		SerializedValSet:         serializedValSet,
		SerializedProposer:       serializedProposer,
		SerializedParentCommits:  serializedParentCommits,
		SerializedPrepares:       serializedPrepares,
		SerializedCommits:        serializedCommits,
		SerializedPendingRequest: serializedPendingRequest,
		SerializedPreprepare:     serializedPreprepare,
	}
	return rlp.Encode(w, entry)
}

// The DecodeRLP method should read one value from the given
// Stream. It is not forbidden to read less or more, but it might
// be confusing.
func (s *roundStateImpl) DecodeRLP(stream *rlp.Stream) error {
	var data roundStateRLP
	err := stream.Decode(&data)
	if err != nil {
		return err
	}

	s.mu = new(sync.RWMutex)
	s.state = data.State
	s.round = data.Round
	s.desiredRound = data.DesiredRound
	s.sequence = data.Sequence
	s.preparedCertificate = *data.PreparedCertificate

	s.prepares, err = deserializeMessageSet(data.SerializedPrepares)
	if err != nil {
		return err
	}
	s.parentCommits, err = deserializeMessageSet(data.SerializedParentCommits)
	if err != nil {
		return err
	}
	s.commits, err = deserializeMessageSet(data.SerializedCommits)
	if err != nil {
		return err
	}
	s.validatorSet, err = validator.DeserializeValidatorSet(data.SerializedValSet)
	if err != nil {
		return err
	}
	s.proposer, err = validator.DeserializeValidator(data.SerializedProposer)
	if err != nil {
		return err
	}

	if !bytes.Equal(data.SerializedPendingRequest, rlp.EmptyList) {
		var value istanbul.Request
		err := rlp.DecodeBytes(data.SerializedPendingRequest, &value)
		if err != nil {
			return err
		}
		s.pendingRequest = &value

	}

	if !bytes.Equal(data.SerializedPreprepare, rlp.EmptyList) {
		var value istanbul.Preprepare
		err := rlp.DecodeBytes(data.SerializedPreprepare, &value)
		if err != nil {
			return err
		}
		s.preprepare = &value
	}

	return nil
}
