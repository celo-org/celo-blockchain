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
	"encoding/binary"
	"errors"
	"io"
	"math/big"
	"os"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
	lvlerrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

const (
	dbVersion    = 1
	dbVersionKey = "version"  // Version of the database to flush if changes
	lastViewKey  = "lastView" // Last View that we know of
)

var (
	// errFailedCreatePreparedCertificate is returned when there aren't enough PREPARE messages to create a PREPARED certificate.
	errFailedCreatePreparedCertificate = errors.New("failed to create PREPARED certficate")
)

func newRoundState(view *istanbul.View, validatorSet istanbul.ValidatorSet, proposer istanbul.Validator) RoundState {
	return &roundStateImpl{
		state:        StateAcceptRequest,
		round:        view.Round,
		desiredRound: view.Round,
		sequence:     view.Sequence,

		// data for current round
		preprepare: nil,
		prepares:   newMessageSet(validatorSet),
		commits:    newMessageSet(validatorSet),
		proposer:   proposer,

		// data saves across rounds, same sequence
		validatorSet:        validatorSet,
		parentCommits:       newMessageSet(validatorSet),
		pendingRequest:      nil,
		preparedCertificate: istanbul.EmptyPreparedCertificate(),

		mu: new(sync.RWMutex),
	}
}

func newRoundStateWithPersistence(sequence *big.Int, validatorSet istanbul.ValidatorSet, proposer istanbul.Validator, path string) (RoundState, error) {
	// create DB
	var db *leveldb.DB
	var err error
	if path == "" {
		db, err = newMemoryDB()
	} else {
		db, err = newPersistentDB(path)
	}

	if err != nil {
		return nil, err
	}

	view := &istanbul.View{
		Sequence: sequence,
		Round:    common.Big0,
	}

	lastStoredView, err := getLastStoredView(db)

	if err != nil && err != leveldb.ErrNotFound {
		db.Close()
		return nil, err
	}

	if err == leveldb.ErrNotFound || lastStoredView.Cmp(view) < 0 {
		return &roundStatePersistence{
			db:       db,
			delegate: newRoundState(view, validatorSet, proposer),
		}, nil
	}

	lastStoredRoundState, err := getStoredRoundState(db, lastStoredView)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &roundStatePersistence{
		db:       db,
		delegate: lastStoredRoundState,
	}, nil
}

// newMemoryDB creates a new in-memory node database without a persistent backend.
func newMemoryDB() (*leveldb.DB, error) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// newPersistentNodeDB creates/opens a leveldb backed persistent node database,
// also flushing its contents in case of a version mismatch.
func newPersistentDB(path string) (*leveldb.DB, error) {
	opts := &opt.Options{OpenFilesCacheCapacity: 5}
	db, err := leveldb.OpenFile(path, opts)
	if _, iscorrupted := err.(*lvlerrors.ErrCorrupted); iscorrupted {
		db, err = leveldb.RecoverFile(path, nil)
	}
	if err != nil {
		return nil, err
	}
	// The nodes contained in the cache correspond to a certain protocol version.
	// Flush all nodes if the version doesn't match.
	currentVer := make([]byte, binary.MaxVarintLen64)
	currentVer = currentVer[:binary.PutVarint(currentVer, int64(dbVersion))]

	blob, err := db.Get([]byte(dbVersionKey), nil)
	switch err {
	case leveldb.ErrNotFound:
		// Version not found (i.e. empty cache), insert it
		if err := db.Put([]byte(dbVersionKey), currentVer, nil); err != nil {
			db.Close()
			return nil, err
		}

	case nil:
		// Version present, flush if different
		if !bytes.Equal(blob, currentVer) {
			db.Close()
			if err = os.RemoveAll(path); err != nil {
				return nil, err
			}
			return newPersistentDB(path)
		}
	}
	return db, nil
}

type RoundState interface {
	// mutation functions
	StartNewRound(nextRound *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator) error
	StartNewSequence(view *istanbul.View, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator, parentCommits MessageSet) error
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

	mu *sync.RWMutex
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

	s.changeRound(nextRound, validatorSet, nextProposer)
	return nil
}

func (s *roundStateImpl) StartNewSequence(view *istanbul.View, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator, parentCommits MessageSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.validatorSet = validatorSet

	s.changeRound(view.Round, validatorSet, nextProposer)

	s.sequence = view.Sequence
	s.preparedCertificate = istanbul.EmptyPreparedCertificate()
	s.pendingRequest = nil
	s.parentCommits = parentCommits
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

type roundStateRLP struct {
	State               State
	Round               *big.Int
	DesiredRound        *big.Int
	Sequence            *big.Int
	Preprepare          *istanbul.Preprepare
	Prepares            MessageSet
	Commits             MessageSet
	Proposer            istanbul.Validator
	ValidatorSet        istanbul.ValidatorSet
	ParentCommits       MessageSet
	PendingRequest      *istanbul.Request
	PreparedCertificate istanbul.PreparedCertificate
}

// The DecodeRLP method should read one value from the given
// Stream. It is not forbidden to read less or more, but it might
// be confusing.
func (s *roundStateImpl) DecodeRLP(stream *rlp.Stream) error {
	var ss roundStateRLP
	if err := stream.Decode(&ss); err != nil {
		return err
	}

	s.state = ss.State
	s.round = ss.Round
	s.desiredRound = ss.DesiredRound
	s.sequence = ss.Sequence
	s.preprepare = ss.Preprepare
	s.prepares = ss.Prepares
	s.commits = ss.Commits
	s.proposer = ss.Proposer
	s.validatorSet = ss.ValidatorSet
	s.parentCommits = ss.ParentCommits
	s.pendingRequest = ss.PendingRequest
	s.preparedCertificate = ss.PreparedCertificate
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

	entry := roundStateRLP{
		State:               s.state,
		Round:               s.round,
		DesiredRound:        s.desiredRound,
		Sequence:            s.sequence,
		Preprepare:          s.preprepare,
		Prepares:            s.prepares,
		Commits:             s.commits,
		Proposer:            s.proposer,
		ValidatorSet:        s.validatorSet,
		ParentCommits:       s.parentCommits,
		PendingRequest:      s.pendingRequest,
		PreparedCertificate: s.preparedCertificate,
	}
	return rlp.Encode(w, entry)
}

type roundStatePersistence struct {
	delegate RoundState
	db       *leveldb.DB //the actual DB
}

// mutation functions
func (rsp *roundStatePersistence) StartNewRound(nextRound *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator) error {
	err := rsp.delegate.StartNewRound(nextRound, validatorSet, nextProposer)
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}
func (rsp *roundStatePersistence) StartNewSequence(view *istanbul.View, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator, parentCommits MessageSet) error {
	err := rsp.delegate.StartNewSequence(view, validatorSet, nextProposer, parentCommits)
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}
func (rsp *roundStatePersistence) TransitionToPreprepared(preprepare *istanbul.Preprepare) error {
	err := rsp.delegate.TransitionToPreprepared(preprepare)
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}
func (rsp *roundStatePersistence) TransitionToWaitingForNewRound(r *big.Int, nextProposer istanbul.Validator) error {
	err := rsp.delegate.TransitionToWaitingForNewRound(r, nextProposer)
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}
func (rsp *roundStatePersistence) TransitionToCommited() error {
	err := rsp.delegate.TransitionToCommited()
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}
func (rsp *roundStatePersistence) TransitionToPrepared(quorumSize int) error {
	err := rsp.delegate.TransitionToPrepared(quorumSize)
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}
func (rsp *roundStatePersistence) AddCommit(msg *istanbul.Message) error {
	err := rsp.delegate.AddCommit(msg)
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}
func (rsp *roundStatePersistence) AddPrepare(msg *istanbul.Message) error {
	err := rsp.delegate.AddPrepare(msg)
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}
func (rsp *roundStatePersistence) AddParentCommit(msg *istanbul.Message) error {
	err := rsp.delegate.AddParentCommit(msg)
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}
func (rsp *roundStatePersistence) SetPendingRequest(pendingRequest *istanbul.Request) error {
	err := rsp.delegate.SetPendingRequest(pendingRequest)
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}

// DesiredRound implements RoundState.DesiredRound
func (rsp *roundStatePersistence) DesiredRound() *big.Int { return rsp.delegate.DesiredRound() }

// State implements RoundState.State
func (rsp *roundStatePersistence) State() State { return rsp.delegate.State() }

// Proposer implements RoundState.Proposer
func (rsp *roundStatePersistence) Proposer() istanbul.Validator { return rsp.delegate.Proposer() }

// Subject implements RoundState.Subject
func (rsp *roundStatePersistence) Subject() *istanbul.Subject { return rsp.delegate.Subject() }

// Proposal implements RoundState.Proposal
func (rsp *roundStatePersistence) Proposal() istanbul.Proposal { return rsp.delegate.Proposal() }

// Round implements RoundState.Round
func (rsp *roundStatePersistence) Round() *big.Int { return rsp.delegate.Round() }

// Commits implements RoundState.Commits
func (rsp *roundStatePersistence) Commits() MessageSet { return rsp.delegate.Commits() }

// Prepares implements RoundState.Prepares
func (rsp *roundStatePersistence) Prepares() MessageSet { return rsp.delegate.Prepares() }

// ParentCommits implements RoundState.ParentCommits
func (rsp *roundStatePersistence) ParentCommits() MessageSet { return rsp.delegate.ParentCommits() }

// Sequence implements RoundState.Sequence
func (rsp *roundStatePersistence) Sequence() *big.Int { return rsp.delegate.Sequence() }

// View implements RoundState.View
func (rsp *roundStatePersistence) View() *istanbul.View { return rsp.delegate.View() }

// GetPrepareOrCommitSize implements RoundState.GetPrepareOrCommitSize
func (rsp *roundStatePersistence) GetPrepareOrCommitSize() int {
	return rsp.delegate.GetPrepareOrCommitSize()
}

// GetValidatorByAddress implements RoundState.GetValidatorByAddress
func (rsp *roundStatePersistence) GetValidatorByAddress(address common.Address) istanbul.Validator {
	return rsp.delegate.GetValidatorByAddress(address)
}

// ValidatorSet implements RoundState.ValidatorSet
func (rsp *roundStatePersistence) ValidatorSet() istanbul.ValidatorSet {
	return rsp.delegate.ValidatorSet()
}

// IsProposer implements RoundState.IsProposer
func (rsp *roundStatePersistence) IsProposer(address common.Address) bool {
	return rsp.delegate.IsProposer(address)
}

// Preprepare implements RoundState.Preprepare
func (rsp *roundStatePersistence) Preprepare() *istanbul.Preprepare {
	return rsp.delegate.Preprepare()
}

// PendingRequest implements RoundState.PendingRequest
func (rsp *roundStatePersistence) PendingRequest() *istanbul.Request {
	return rsp.delegate.PendingRequest()
}

// PreparedCertificate implements RoundState.PreparedCertificate
func (rsp *roundStatePersistence) PreparedCertificate() istanbul.PreparedCertificate {
	return rsp.delegate.PreparedCertificate()
}

func (rsp *roundStatePersistence) storeRoundState() error {
	viewKey, err := rlp.EncodeToBytes(rsp.delegate.View())
	if err != nil {
		return err
	}

	entryBytes, err := rlp.EncodeToBytes(rsp)
	if err != nil {
		return err
	}

	batch := new(leveldb.Batch)
	batch.Put([]byte(lastViewKey), viewKey)
	batch.Put(viewKey, entryBytes)

	return rsp.db.Write(batch, nil)
}

func getLastStoredView(db *leveldb.DB) (*istanbul.View, error) {
	rawEntry, err := db.Get([]byte(lastViewKey), nil)
	if err != nil {
		return nil, err
	}

	var entry istanbul.View
	if err = rlp.DecodeBytes(rawEntry, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func getStoredRoundState(db *leveldb.DB, view *istanbul.View) (RoundState, error) {
	viewKey, err := rlp.EncodeToBytes(view)
	if err != nil {
		return nil, err
	}

	rawEntry, err := db.Get(viewKey, nil)
	if err != nil {
		return nil, err
	}

	var entry roundStateImpl
	if err = rlp.DecodeBytes(rawEntry, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}
