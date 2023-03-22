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
	"io"
	"math/big"
	"os"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/task"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/syndtr/goleveldb/leveldb"
	lvlerrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	dbVersion    = 2
	dbVersionKey = "version"  // Version of the database to flush if changes
	lastViewKey  = "lastView" // Last View that we know of
	rsKey        = "rs"       // Database Key Pefix for RoundState
	rcvdKey      = "rcvd"     // Database Key Prefix for rcvd messages from the RoundState (split saving)
)

type RoundStateDB interface {
	GetLastView() (*istanbul.View, error)
	// GetOldestValidView returns the oldest valid view that can be stored on the db
	// it might or might not be present on the db
	GetOldestValidView() (*istanbul.View, error)
	GetRoundStateFor(view *istanbul.View) (RoundState, error)
	UpdateLastRoundState(rs RoundState) error
	UpdateLastRcvd(rs RoundState) error
	Close() error
}

// RoundStateDBOptions are the options for a RoundStateDB instance
type RoundStateDBOptions struct {
	withGarbageCollector   bool
	garbageCollectorPeriod time.Duration
	sequencesToSave        uint64
}

type roundStateDBImpl struct {
	db                   *leveldb.DB
	stopGarbageCollector task.StopFn
	opts                 RoundStateDBOptions
	logger               log.Logger
}

var defaultRoundStateDBOptions = RoundStateDBOptions{
	withGarbageCollector:   true,
	sequencesToSave:        100,
	garbageCollectorPeriod: 2 * time.Minute,
}

func coerceOptions(opts *RoundStateDBOptions) RoundStateDBOptions {
	if opts == nil {
		return defaultRoundStateDBOptions
	}

	options := *opts
	if options.sequencesToSave == 0 {
		options.sequencesToSave = defaultRoundStateDBOptions.sequencesToSave
	}
	if options.withGarbageCollector && options.garbageCollectorPeriod == 0 {
		options.garbageCollectorPeriod = defaultRoundStateDBOptions.garbageCollectorPeriod
	}
	return options
}

func newRoundStateDB(path string, opts *RoundStateDBOptions) (RoundStateDB, error) {
	logger := log.New("func", "newRoundStateDB", "type", "roundStateDB", "rsdb_path", path)

	logger.Info("Open roundstate db")
	var db *leveldb.DB
	var err error
	if path == "" {
		db, err = newMemoryDB()
	} else {
		db, err = newPersistentDB(path)
	}

	if err != nil {
		logger.Error("Failed to open roundstate db", "err", err)
		return nil, err
	}

	rsdb := &roundStateDBImpl{
		db:     db,
		opts:   coerceOptions(opts),
		logger: logger,
	}

	if rsdb.opts.withGarbageCollector {
		rsdb.stopGarbageCollector = task.RunTaskRepeateadly(rsdb.garbageCollectEntries, task.NewDefaultTicker(rsdb.opts.garbageCollectorPeriod))
	}

	return rsdb, nil
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

type rcvd struct {
	Prepares      MessageSet
	Commits       MessageSet
	ParentCommits MessageSet
}

type rcvdRLP struct {
	Prep []byte
	Comm []byte
	Parc []byte
}

func (r *rcvd) ToRLP() (*rcvdRLP, error) {
	serializedParentCommits, err := r.ParentCommits.Serialize()
	if err != nil {
		return nil, err
	}
	serializedPrepares, err := r.Prepares.Serialize()
	if err != nil {
		return nil, err
	}
	serializedCommits, err := r.Commits.Serialize()
	if err != nil {
		return nil, err
	}
	return &rcvdRLP{
		Prep: serializedPrepares,
		Comm: serializedCommits,
		Parc: serializedParentCommits,
	}, nil
}

func (r *rcvd) FromRLP(r2 *rcvdRLP) error {
	var err error
	r.Prepares, err = deserializeMessageSet(r2.Prep)
	if err != nil {
		return err
	}
	r.ParentCommits, err = deserializeMessageSet(r2.Parc)
	if err != nil {
		return err
	}
	r.Commits, err = deserializeMessageSet(r2.Comm)
	if err != nil {
		return err
	}
	return nil
}

func (r *rcvdRLP) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{r.Prep, r.Comm, r.Parc})
}

func (r *rcvdRLP) DecodeRLP(s *rlp.Stream) error {
	var d struct {
		Prepares      []byte
		Commits       []byte
		ParentCommits []byte
	}
	if err := s.Decode(&d); err != nil {
		return err
	}
	r.Prep = d.Prepares
	r.Comm = d.Commits
	r.Parc = d.ParentCommits
	return nil
}

func rcvdFromRoundState(rs RoundState) *rcvd {
	r := rcvd{}
	r.Prepares = rs.Prepares()
	r.Commits = rs.Commits()
	r.ParentCommits = rs.ParentCommits()
	return &r
}

// UpdateLastRcvd will update to the db only the rcvd messages (prepares, commits, parentCommits).
// this is used to hold signed messages as proof for possible future slashes.
// While UpdateLastRoundState stores the messages as well, this is called from
// consensus when a message is received and approved, where we need fast updates
// to the db, and we want to avoid re-saving the block in these cases.
//
// Possible future improvements are to completely refactor the RoundState & Impl to
// include a fine-grained journaling system.
func (rsdb *roundStateDBImpl) UpdateLastRcvd(rs RoundState) error {
	logger := rsdb.logger.New("func", "UpdateLastRcvd")
	rcvdViewKey := rcvdView2Key(rs.View())

	r := rcvdFromRoundState(rs)
	rRLP, err := r.ToRLP()
	if err != nil {
		return err
	}
	entryBytes, err := rlp.EncodeToBytes(&rRLP)
	if err != nil {
		logger.Error("Failed to save rcvd messages from roundState", "reason", "rlp encoding", "err", err)
		return err
	}
	batch := new(leveldb.Batch)
	batch.Put(rcvdViewKey, entryBytes)
	err = rsdb.db.Write(batch, nil)
	if err != nil {
		logger.Error("Failed to save rcvd messages from roundState", "reason", "levelDB write", "err", err, "func")
	}

	return err
}

// storeRoundState will store the currentRoundState in a Map<view, roundState> schema.
func (rsdb *roundStateDBImpl) UpdateLastRoundState(rs RoundState) error {
	// We store the roundState for each view; since we'll need this
	// information to allow the node to have evidence to show that
	// a validator did a "valid" double signing
	logger := rsdb.logger.New("func", "UpdateLastRoundState")
	viewKey := view2Key(rs.View())

	entryBytes, err := rlp.EncodeToBytes(rs)
	if err != nil {
		logger.Error("Failed to save roundState", "reason", "rlp encoding", "err", err)
		return err
	}

	batch := new(leveldb.Batch)
	batch.Put([]byte(lastViewKey), viewKey)
	batch.Put(viewKey, entryBytes)

	err = rsdb.db.Write(batch, nil)
	if err != nil {
		logger.Error("Failed to save roundState", "reason", "levelDB write", "err", err, "func")
	}

	return err
}

func (rsdb *roundStateDBImpl) GetLastView() (*istanbul.View, error) {
	rawEntry, err := rsdb.db.Get([]byte(lastViewKey), nil)
	if err != nil {
		return nil, err
	}

	return key2View(rawEntry), nil
}

func (rsdb *roundStateDBImpl) GetOldestValidView() (*istanbul.View, error) {
	lastView, err := rsdb.GetLastView()
	// If nothing stored all views are valid
	if err == leveldb.ErrNotFound {
		return &istanbul.View{Sequence: common.Big0, Round: common.Big0}, nil
	} else if err != nil {
		return nil, err
	}

	oldestValidSequence := new(big.Int).Sub(lastView.Sequence, new(big.Int).SetUint64(rsdb.opts.sequencesToSave))
	if oldestValidSequence.Cmp(common.Big0) < 0 {
		oldestValidSequence = common.Big0
	}

	return &istanbul.View{Sequence: oldestValidSequence, Round: common.Big0}, nil
}

func (rsdb *roundStateDBImpl) GetRoundStateFor(view *istanbul.View) (RoundState, error) {
	viewKey := view2Key(view)
	rawEntry, err := rsdb.db.Get(viewKey, nil)
	if err != nil {
		return nil, err
	}

	var entry roundStateImpl
	if err = rlp.DecodeBytes(rawEntry, &entry); err != nil {
		return nil, err
	}
	// Check if rcvd is stored
	rcvdViewKey := rcvdView2Key(view)
	rawRcvd, err := rsdb.db.Get(rcvdViewKey, nil)
	// No rcvd, return the roundstate as found
	if err == leveldb.ErrNotFound {
		return &entry, nil
	}
	// Unknown error. Return the roundstate as found, but log the err
	if err != nil {
		return nil, err
	}
	var r *rcvdRLP = &rcvdRLP{}
	if err = rlp.DecodeBytes(rawRcvd, &r); err != nil {
		return nil, err
	}
	// Transform into rcvd
	var res *rcvd = &rcvd{}
	err = res.FromRLP(r)
	if err != nil {
		return nil, err
	}
	return merged(&entry, res), nil
}

func merged(rs *roundStateImpl, r *rcvd) *roundStateImpl {
	rs.prepares.AddAll(r.Prepares.Values())
	rs.commits.AddAll(r.Commits.Values())
	rs.parentCommits.AddAll(r.ParentCommits.Values())
	return rs
}

func (rsdb *roundStateDBImpl) Close() error {
	if rsdb.opts.withGarbageCollector {
		rsdb.stopGarbageCollector()
	}
	return rsdb.db.Close()
}

func (rsdb *roundStateDBImpl) garbageCollectEntries() {
	logger := rsdb.logger.New("func", "garbageCollectEntries")

	oldestValidView, err := rsdb.GetOldestValidView()
	if err != nil {
		logger.Error("Aborting RoundStateDB GarbageCollect: Failed to fetch oldestValidView", "err", err)
		return
	}

	logger.Debug("Pruning entries from old views", "oldestValidView", oldestValidView)
	count, err := rsdb.deleteEntriesOlderThan(oldestValidView)
	if err != nil {
		logger.Error("Aborting RoundStateDB GarbageCollect: Failed to remove entries", "entries_removed", count, "err", err)
		return
	}

	logger.Debug("Finished RoundStateDB GarbageCollect", "removed_entries", count)
}

func (rsdb *roundStateDBImpl) deleteEntriesOlderThan(lastView *istanbul.View) (int, error) {
	fromViewKey := view2Key(&istanbul.View{Sequence: common.Big0, Round: common.Big0})
	toViewKey := view2Key(lastView)

	count, err := rsdb.deleteIteratorEntries(&util.Range{Start: fromViewKey, Limit: toViewKey})
	if err != nil {
		return count, err
	}

	fromRcvdKey := rcvdView2Key(&istanbul.View{Sequence: common.Big0, Round: common.Big0})
	toRcvdKey := rcvdView2Key(lastView)
	rcvdCount, err := rsdb.deleteIteratorEntries(&util.Range{Start: fromRcvdKey, Limit: toRcvdKey})
	if err != nil {
		return count + rcvdCount, err
	}
	return count + rcvdCount, nil
}

func (rsdb *roundStateDBImpl) deleteIteratorEntries(rang *util.Range) (int, error) {
	iter := rsdb.db.NewIterator(rang, nil)
	defer iter.Release()
	counter := 0
	for iter.Next() {
		rawKey := iter.Key()
		err := rsdb.db.Delete(rawKey, nil)
		if err != nil {
			return counter, err
		}
		counter++
	}
	return counter, nil
}

// view2Key will encode a view in binary format
// so that the binary format maintains the sort order for the view,
// using the rsKey prefix
func view2Key(view *istanbul.View) []byte {
	return prefixView2Key(rsKey, view)
}

// rcvdView2Key will encode a view in binary format
// so that the binary format maintains the sort order for the view,
// using the rcvdKey prefix
func rcvdView2Key(view *istanbul.View) []byte {
	return prefixView2Key(rcvdKey, view)
}

func prefixView2Key(prefix string, view *istanbul.View) []byte {
	// leveldb sorts entries by key
	// keys are sorted with their binary representation, so we need a binary representation
	// that mantains the key order
	// The key format is [ prefix . BigEndian(Sequence) . BigEndian(Round)]
	// We use BigEndian so to maintain order in binary format
	// And we want to sort by (seq, round); since seq had higher precedence than round
	buff := make([]byte, len(prefix)+16)

	copy(buff, prefix)
	// TODO (mcortesi) Support Seq/Round bigger than 64bits
	binary.BigEndian.PutUint64(buff[len(prefix):], view.Sequence.Uint64())
	binary.BigEndian.PutUint64(buff[len(prefix)+8:], view.Round.Uint64())

	return buff
}

func key2View(key []byte) *istanbul.View {
	prefixLen := len([]byte(rsKey))
	seq := binary.BigEndian.Uint64(key[prefixLen : prefixLen+8])
	round := binary.BigEndian.Uint64(key[prefixLen+8:])
	return &istanbul.View{
		Sequence: new(big.Int).SetUint64(seq),
		Round:    new(big.Int).SetUint64(round),
	}
}
