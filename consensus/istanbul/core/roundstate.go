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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
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
	TransitionToCommitted() error
	TransitionToPrepared(quorumSize int) error
	AddCommit(msg *istanbul.Message) error
	AddPrepare(msg *istanbul.Message) error
	AddParentCommit(msg *istanbul.Message) error
	SetPendingRequest(pendingRequest *istanbul.Request) error
	SetProposalVerificationStatus(proposalHash common.Hash, verificationStatus error)
	SetStateProcessResult(proposalHash common.Hash, blockProcessResult *StateProcessResult)

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
	GetProposalVerificationStatus(proposalHash common.Hash) (verificationStatus error, isCached bool)
	GetStateProcessResult(proposalHash common.Hash) (result *StateProcessResult)
	Summary() *RoundStateSummary
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

	// Verification status for proposals seen in this view
	// Note that this field will not get RLP enoded and persisted, since it contains an error type,
	// which doesn't have a native RLP encoding.  Also, this is a cache, so it's not necessary for it
	// to be persisted.
	proposalVerificationStatus map[common.Hash]error

	// Cache for StateProcessResult in this sequence.
	stateProcessResults map[common.Hash]*StateProcessResult

	mu     *sync.RWMutex
	logger log.Logger

	// Gauges for current round, desiredRound, and sequence
	roundGauge        metrics.Gauge
	desiredRoundGauge metrics.Gauge
	sequenceGauge     metrics.Gauge
}

type RoundStateSummary struct {
	State              string       `json:"state"`
	Sequence           *big.Int     `json:"sequence"`
	Round              *big.Int     `json:"round"`
	DesiredRound       *big.Int     `json:"desiredRound"`
	PendingRequestHash *common.Hash `json:"pendingRequestHash"`

	ValidatorSet []common.Address `json:"validatorSet"`
	Proposer     common.Address   `json:"proposer"`

	Prepares      []common.Address `json:"prepares"`
	Commits       []common.Address `json:"commits"`
	ParentCommits []common.Address `json:"parentCommits"`

	Preprepare          *istanbul.PreprepareSummary          `json:"preprepare"`
	PreparedCertificate *istanbul.PreparedCertificateSummary `json:"preparedCertificate"`
}

func newRoundState(view *istanbul.View, validatorSet istanbul.ValidatorSet, proposer istanbul.Validator) RoundState {
	if proposer == nil {
		log.Crit("Proposer cannot be nil")
	}
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

		roundGauge:        metrics.NewRegisteredGauge("consensus/istanbul/core/round", nil),
		desiredRoundGauge: metrics.NewRegisteredGauge("consensus/istanbul/core/desiredround", nil),
		sequenceGauge:     metrics.NewRegisteredGauge("consensus/istanbul/core/sequence", nil),
	}
}

func (rs *roundStateImpl) Commits() MessageSet       { return rs.commits }
func (rs *roundStateImpl) Prepares() MessageSet      { return rs.prepares }
func (rs *roundStateImpl) ParentCommits() MessageSet { return rs.parentCommits }

func (rs *roundStateImpl) State() State {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.state
}

func (rs *roundStateImpl) View() *istanbul.View {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return &istanbul.View{
		Sequence: new(big.Int).Set(rs.sequence),
		Round:    new(big.Int).Set(rs.round),
	}
}

func (rs *roundStateImpl) GetPrepareOrCommitSize() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	result := rs.prepares.Size() + rs.commits.Size()

	// find duplicate one
	for _, m := range rs.prepares.Values() {
		if rs.commits.Get(m.Address) != nil {
			result--
		}
	}
	return result
}

func (rs *roundStateImpl) Subject() *istanbul.Subject {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if rs.preprepare == nil {
		return nil
	}

	return &istanbul.Subject{
		View: &istanbul.View{
			Round:    new(big.Int).Set(rs.round),
			Sequence: new(big.Int).Set(rs.sequence),
		},
		Digest: rs.preprepare.Proposal.Hash(),
	}
}

func (rs *roundStateImpl) IsProposer(address common.Address) bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return rs.proposer.Address() == address
}

func (rs *roundStateImpl) Proposer() istanbul.Validator {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return rs.proposer
}

func (rs *roundStateImpl) ValidatorSet() istanbul.ValidatorSet {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return rs.validatorSet
}

func (rs *roundStateImpl) GetValidatorByAddress(address common.Address) istanbul.Validator {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	_, validator := rs.validatorSet.GetByAddress(address)
	return validator
}

func (rs *roundStateImpl) Preprepare() *istanbul.Preprepare {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return rs.preprepare
}

func (rs *roundStateImpl) Proposal() istanbul.Proposal {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if rs.preprepare != nil {
		return rs.preprepare.Proposal
	}

	return nil
}

func (rs *roundStateImpl) Round() *big.Int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return rs.round
}

func (rs *roundStateImpl) changeRound(nextRound *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator) {
	if nextProposer == nil {
		log.Crit("Proposer cannot be nil")
	}

	rs.state = StateAcceptRequest
	rs.round = nextRound
	rs.desiredRound = nextRound

	// Update gauges
	rs.roundGauge.Update(nextRound.Int64())
	rs.desiredRoundGauge.Update(nextRound.Int64())

	// TODO MC use old valset
	rs.prepares = newMessageSet(validatorSet)
	rs.commits = newMessageSet(validatorSet)
	rs.proposer = nextProposer

	// ??
	rs.preprepare = nil
}

func (rs *roundStateImpl) StartNewRound(nextRound *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	logger := rs.newLogger()
	rs.changeRound(nextRound, validatorSet, nextProposer)
	logger.Debug("Starting new round", "next_round", nextRound, "next_proposer", nextProposer.Address().Hex())
	return nil
}

func (rs *roundStateImpl) StartNewSequence(nextSequence *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator, parentCommits MessageSet) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	logger := rs.newLogger()

	rs.validatorSet = validatorSet

	rs.changeRound(big.NewInt(0), validatorSet, nextProposer)

	rs.sequence = nextSequence
	rs.preparedCertificate = istanbul.EmptyPreparedCertificate()
	rs.pendingRequest = nil
	rs.parentCommits = parentCommits
	rs.proposalVerificationStatus = nil
	rs.stateProcessResults = nil

	// Update sequence gauge
	rs.sequenceGauge.Update(nextSequence.Int64())

	logger.Debug("Starting new sequence", "next_sequence", nextSequence, "next_proposer", nextProposer.Address().Hex())
	return nil
}

func (rs *roundStateImpl) TransitionToCommitted() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.state = StateCommitted
	return nil
}

func (rs *roundStateImpl) TransitionToPreprepared(preprepare *istanbul.Preprepare) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.preprepare = preprepare
	rs.state = StatePreprepared
	return nil
}

func (rs *roundStateImpl) TransitionToWaitingForNewRound(desiredRound *big.Int, nextProposer istanbul.Validator) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.round.Cmp(desiredRound) > 0 {
		return errInvalidState
	}
	if nextProposer == nil {
		log.Crit("Proposer cannot be nil")
	}

	rs.desiredRound = new(big.Int).Set(desiredRound)
	rs.proposer = nextProposer
	rs.state = StateWaitingForNewRound

	// Update gauge
	rs.desiredRoundGauge.Update(desiredRound.Int64())

	return nil
}

// TransitionToPrepared will create a PreparedCertificate and change state to Prepared
func (rs *roundStateImpl) TransitionToPrepared(quorumSize int) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	messages := make([]istanbul.Message, quorumSize)
	i := 0
	for _, message := range rs.prepares.Values() {
		if i == quorumSize {
			break
		}
		messages[i] = *message
		i++
	}
	for _, message := range rs.commits.Values() {
		if i == quorumSize {
			break
		}
		if rs.prepares.Get(message.Address) == nil {
			messages[i] = *message
			i++
		}
	}
	if i != quorumSize {
		return errFailedCreatePreparedCertificate
	}
	rs.preparedCertificate = istanbul.PreparedCertificate{
		Proposal:                rs.preprepare.Proposal,
		PrepareOrCommitMessages: messages,
	}

	rs.state = StatePrepared
	return nil
}

func (rs *roundStateImpl) AddCommit(msg *istanbul.Message) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.commits.Add(msg)
}

func (rs *roundStateImpl) AddPrepare(msg *istanbul.Message) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.prepares.Add(msg)
}

func (rs *roundStateImpl) AddParentCommit(msg *istanbul.Message) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.parentCommits.Add(msg)
}

func (rs *roundStateImpl) DesiredRound() *big.Int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return rs.desiredRound
}

func (rs *roundStateImpl) SetPendingRequest(pendingRequest *istanbul.Request) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.pendingRequest = pendingRequest
	return nil
}

func (rs *roundStateImpl) PendingRequest() *istanbul.Request {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.pendingRequest
}

func (rs *roundStateImpl) Sequence() *big.Int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return rs.sequence
}

func (rs *roundStateImpl) PreparedCertificate() istanbul.PreparedCertificate {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.preparedCertificate
}

func (rs *roundStateImpl) SetProposalVerificationStatus(proposalHash common.Hash, verificationStatus error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.proposalVerificationStatus == nil {
		rs.proposalVerificationStatus = make(map[common.Hash]error)
	}

	rs.proposalVerificationStatus[proposalHash] = verificationStatus
}

func (rs *roundStateImpl) GetProposalVerificationStatus(proposalHash common.Hash) (verificationStatus error, isCached bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	verificationStatus, isCached = nil, false

	if rs.proposalVerificationStatus != nil {
		verificationStatus, isCached = rs.proposalVerificationStatus[proposalHash]
	}

	return
}

// StateProcessResult represents processing results from StateProcessor.
type StateProcessResult struct {
	State    *state.StateDB
	Receipts types.Receipts
	Logs     []*types.Log
}

func (rs *roundStateImpl) SetStateProcessResult(proposalHash common.Hash, stateProcessResult *StateProcessResult) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.stateProcessResults == nil {
		rs.stateProcessResults = make(map[common.Hash]*StateProcessResult)
	}

	rs.stateProcessResults[proposalHash] = stateProcessResult
}

func (rs *roundStateImpl) GetStateProcessResult(proposalHash common.Hash) (stateProcessResult *StateProcessResult) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if rs.stateProcessResults != nil {
		stateProcessResult = rs.stateProcessResults[proposalHash]
	}

	return
}

func (rs *roundStateImpl) Summary() *RoundStateSummary {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	summary := &RoundStateSummary{
		State:        rs.state.String(),
		Sequence:     rs.sequence,
		Round:        rs.round,
		DesiredRound: rs.desiredRound,

		Proposer:     rs.proposer.Address(),
		ValidatorSet: istanbul.MapValidatorsToAddresses(rs.validatorSet.List()),

		Prepares:      rs.prepares.Addresses(),
		Commits:       rs.commits.Addresses(),
		ParentCommits: rs.parentCommits.Addresses(),
	}

	if rs.pendingRequest != nil {
		hash := rs.pendingRequest.Proposal.Hash()
		summary.PendingRequestHash = &hash
	}

	if rs.preprepare != nil {
		summary.Preprepare = rs.preprepare.Summary()
	}

	if !rs.preparedCertificate.IsEmpty() {
		summary.PreparedCertificate = rs.preparedCertificate.Summary()
	}

	return summary
}

func (rs *roundStateImpl) newLogger(ctx ...interface{}) log.Logger {
	logger := rs.logger.New(ctx...)
	return logger.New("cur_seq", rs.sequence, "cur_round", rs.round, "state", rs.state)
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
func (rs *roundStateImpl) EncodeRLP(w io.Writer) error {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	serializedValSet, err := rs.validatorSet.Serialize()
	if err != nil {
		return err
	}
	if rs.proposer == nil {
		return errors.New("Invalid RoundState: no proposer")
	}
	serializedProposer, err := rs.proposer.Serialize()
	if err != nil {
		return err
	}

	serializedParentCommits, err := rs.parentCommits.Serialize()
	if err != nil {
		return err
	}
	serializedPrepares, err := rs.prepares.Serialize()
	if err != nil {
		return err
	}
	serializedCommits, err := rs.commits.Serialize()
	if err != nil {
		return err
	}

	// handle nullable field. Serialized them to rlp.EmptyList or the rlp version of them
	var serializedPendingRequest []byte
	if rs.pendingRequest == nil {
		serializedPendingRequest = rlp.EmptyList
	} else {
		serializedPendingRequest, err = rlp.EncodeToBytes(rs.pendingRequest)
		if err != nil {
			return err
		}
	}

	var serializedPreprepare []byte
	if rs.preprepare == nil {
		serializedPreprepare = rlp.EmptyList
	} else {
		serializedPreprepare, err = rlp.EncodeToBytes(rs.preprepare)
		if err != nil {
			return err
		}
	}

	entry := roundStateRLP{
		State:               rs.state,
		Round:               rs.round,
		DesiredRound:        rs.desiredRound,
		Sequence:            rs.sequence,
		PreparedCertificate: &rs.preparedCertificate,

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
func (rs *roundStateImpl) DecodeRLP(stream *rlp.Stream) error {
	var data roundStateRLP
	err := stream.Decode(&data)
	if err != nil {
		return err
	}

	rs.logger = log.New()
	rs.mu = new(sync.RWMutex)
	rs.roundGauge = metrics.NewRegisteredGauge("consensus/istanbul/core/round", nil)
	rs.desiredRoundGauge = metrics.NewRegisteredGauge("consensus/istanbul/core/desiredround", nil)
	rs.sequenceGauge = metrics.NewRegisteredGauge("consensus/istanbul/core/sequence", nil)
	rs.state = data.State
	rs.round = data.Round
	rs.desiredRound = data.DesiredRound
	rs.sequence = data.Sequence
	rs.preparedCertificate = *data.PreparedCertificate

	rs.prepares, err = deserializeMessageSet(data.SerializedPrepares)
	if err != nil {
		return err
	}
	rs.parentCommits, err = deserializeMessageSet(data.SerializedParentCommits)
	if err != nil {
		return err
	}
	rs.commits, err = deserializeMessageSet(data.SerializedCommits)
	if err != nil {
		return err
	}
	rs.validatorSet, err = validator.DeserializeValidatorSet(data.SerializedValSet)
	if err != nil {
		return err
	}
	rs.proposer, err = validator.DeserializeValidator(data.SerializedProposer)
	if err != nil {
		return err
	}

	if !bytes.Equal(data.SerializedPendingRequest, rlp.EmptyList) {
		var value istanbul.Request
		err := rlp.DecodeBytes(data.SerializedPendingRequest, &value)
		if err != nil {
			return err
		}
		rs.pendingRequest = &value

	}

	if !bytes.Equal(data.SerializedPreprepare, rlp.EmptyList) {
		var value istanbul.Preprepare
		err := rlp.DecodeBytes(data.SerializedPreprepare, &value)
		if err != nil {
			return err
		}
		rs.preprepare = &value
	}

	return nil
}
