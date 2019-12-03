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
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
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

// createOrRestoreRoundState will obtain the last saved RoundState and use it if it's newer than the given Sequence,
// if not it will create a new one with the information given.
func createOrRestoreRoundState(sequence *big.Int, validatorSet istanbul.ValidatorSet, proposer istanbul.Validator, path string) (RoundState, error) {
	logger := log.New("func", "createOrRestoreRoundState")

	var db *leveldb.DB
	var err error
	if path == "" {
		logger.Info("Open roundstate db", "path", "inmemory")
		db, err = newMemoryDB()
	} else {
		logger.Info("Open roundstate db", "path", path)
		db, err = newPersistentDB(path)
	}

	if err != nil {
		logger.Error("Failed to open roundstate db", "err", err)
		return nil, err
	}

	view := &istanbul.View{
		Sequence: sequence,
		Round:    common.Big0,
	}

	lastStoredView, err := getLastStoredView(db)

	if err != nil && err != leveldb.ErrNotFound {
		logger.Error("Failed to fetch lastStoredView", "err", err)
		db.Close()
		return nil, err
	}

	if err == leveldb.ErrNotFound || lastStoredView.Cmp(view) < 0 {
		if err == leveldb.ErrNotFound {
			logger.Info("Creating new RoundState", "reason", "No storedView found")
		} else {
			logger.Info("Creating new RoundState", "reason", "old view", "stored_view", lastStoredView, "requested_view", view)
		}
		return &roundStatePersistence{
			db:       db,
			delegate: newRoundState(view, validatorSet, proposer),
		}, nil
	}

	logger.Info("Retrieving stored RoundState", "stored_view", lastStoredView, "requested_view", view)
	lastStoredRoundState, err := getStoredRoundState(db, lastStoredView)
	if err != nil {
		logger.Error("Failed to fetch lastStoredRoundState", "err", err)
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

type roundStatePersistence struct {
	delegate RoundState
	db       *leveldb.DB //the actual DB
}

func (rsp *roundStatePersistence) Close() error {
	return rsp.db.Close()
}

// mutation functions
func (rsp *roundStatePersistence) StartNewRound(nextRound *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator) error {
	err := rsp.delegate.StartNewRound(nextRound, validatorSet, nextProposer)
	if err != nil {
		return err
	}

	return rsp.storeRoundState()
}
func (rsp *roundStatePersistence) StartNewSequence(nextSequence *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator, parentCommits MessageSet) error {
	err := rsp.delegate.StartNewSequence(nextSequence, validatorSet, nextProposer, parentCommits)
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
func (rsp *roundStatePersistence) TransitionToCommitted() error {
	err := rsp.delegate.TransitionToCommitted()
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

// storeRoundState will store the currentRoundState in a Map<view, roundState> schema.
func (rsp *roundStatePersistence) storeRoundState() error {
	// We store the roundState for each view; since we'll need this
	// information to allow the node to have evidence to show that
	// a validator did a "valid" double signing

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
