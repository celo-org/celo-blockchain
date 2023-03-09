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
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/task"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
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
)

type RoundStateDB interface {
	GetLastView() (*istanbul.View, error)
	// GetOldestValidView returns the oldest valid view that can be stored on the db
	// it might or might not be present on the db
	GetOldestValidView() (*istanbul.View, error)
	GetRoundStateFor(view *istanbul.View) (RoundState, error)
	UpdateLastRoundState(rs RoundState) error
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

	roundStateRLPMeter metrics.Meter
	rlpTimer           metrics.Timer
	dbSaveTimer        metrics.Timer
	logger             log.Logger
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
		db:                 db,
		opts:               coerceOptions(opts),
		roundStateRLPMeter: metrics.NewRegisteredMeter("consensus/istanbul/core/roundstate/rlp/size", nil),
		rlpTimer:           metrics.NewRegisteredTimer("consensus/istanbul/core/roundstate/rlp/time", nil),
		dbSaveTimer:        metrics.NewRegisteredTimer("consensus/istanbul/core/roundstate/db/save/time", nil),
		logger:             logger,
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

// storeRoundState will store the currentRoundState in a Map<view, roundState> schema.
func (rsdb *roundStateDBImpl) UpdateLastRoundState(rs RoundState) error {
	// We store the roundState for each view; since we'll need this
	// information to allow the node to have evidence to show that
	// a validator did a "valid" double signing
	logger := rsdb.logger.New("func", "UpdateLastRoundState")
	viewKey := view2Key(rs.View())

	// Encode and measure time
	before := time.Now()
	entryBytes, err := rlp.EncodeToBytes(rs)
	rsdb.rlpTimer.UpdateSince(before)

	if err != nil {
		logger.Error("Failed to save roundState", "reason", "rlp encoding", "err", err)
		return err
	}
	rsdb.roundStateRLPMeter.Mark(int64(len(entryBytes)))

	before = time.Now()
	batch := new(leveldb.Batch)
	batch.Put([]byte(lastViewKey), viewKey)
	batch.Put(viewKey, entryBytes)

	err = rsdb.db.Write(batch, nil)
	if err != nil {
		logger.Error("Failed to save roundState", "reason", "levelDB write", "err", err, "func")
	}
	rsdb.dbSaveTimer.UpdateSince(before)

	stats, _ := rsdb.db.GetProperty("leveldb.stats")
	logger.Warn(stats)

	compcount, _ := rsdb.db.GetProperty("leveldb.compcount")
	logger.Warn(compcount)

	iostats, _ := rsdb.db.GetProperty("leveldb.iostats")
	logger.Warn(iostats)

	writedelay, _ := rsdb.db.GetProperty("leveldb.writedelay")
	logger.Warn(writedelay)

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
	return &entry, nil
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

	iter := rsdb.db.NewIterator(&util.Range{Start: fromViewKey, Limit: toViewKey}, nil)
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
// so that the binary format maintains the sort order for the view
func view2Key(view *istanbul.View) []byte {
	// leveldb sorts entries by key
	// keys are sorted with their binary representation, so we need a binary representation
	// that mantains the key order
	// The key format is [ prefix . BigEndian(Sequence) . BigEndian(Round)]
	// We use BigEndian so to maintain order in binary format
	// And we want to sort by (seq, round); since seq had higher precedence than round
	prefix := []byte(rsKey)
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
