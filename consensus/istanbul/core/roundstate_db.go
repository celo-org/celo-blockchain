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
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/task"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/syndtr/goleveldb/leveldb"
	lvlerrors "github.com/syndtr/goleveldb/leveldb/errors"
	// "github.com/syndtr/goleveldb/leveldb/filter"
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

	// degradationWarnInterval specifies how often warning should be printed if the
	// leveldb database cannot keep up with requested writes.
	degradationWarnInterval = time.Minute

	// minCache is the minimum amount of memory in megabytes to allocate to leveldb
	// read and write caching, split half and half.
	minCache = 16

	// minHandles is the minimum number of files handles to allocate to the open
	// database files.
	minHandles = 16

	// metricsGatheringInterval specifies the interval to retrieve leveldb database
	// compaction, io and pause stats to report to the user.
	metricsGatheringInterval = 3 * time.Second
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

	compTimeMeter      metrics.Meter // Meter for measuring the total time spent in database compaction
	compReadMeter      metrics.Meter // Meter for measuring the data read during compaction
	compWriteMeter     metrics.Meter // Meter for measuring the data written during compaction
	writeDelayNMeter   metrics.Meter // Meter for measuring the write delay number due to database compaction
	writeDelayMeter    metrics.Meter // Meter for measuring the write delay duration due to database compaction
	diskSizeGauge      metrics.Gauge // Gauge for tracking the size of all the levels in the database
	diskReadMeter      metrics.Meter // Meter for measuring the effective amount of data read
	diskWriteMeter     metrics.Meter // Meter for measuring the effective amount of data written
	memCompGauge       metrics.Gauge // Gauge for tracking the number of memory compaction
	level0CompGauge    metrics.Gauge // Gauge for tracking the number of table compaction in level0
	nonlevel0CompGauge metrics.Gauge // Gauge for tracking the number of table compaction in non0 level
	seekCompGauge      metrics.Gauge // Gauge for tracking the number of table compaction caused by read opt
	rsRLPMeter         metrics.Meter // Meter for measuring the size of rs RLP-encoded data
	rsRLPEncTimer      metrics.Timer // Timer measuring time required for rs RLP encoding
	rsDbSaveTimer      metrics.Timer // Timer measuring rs DB write latency
	rcvdRLPMeter       metrics.Meter // Meter for measuring the size of rcvd RLP-encoded data
	rcvdRLPEncTimer    metrics.Timer // Timer measuring time required for rcvd RLP encoding
	rcvdDbSaveTimer    metrics.Timer // Timer measuring rcvd DB write latency

	quitLock sync.Mutex      // Mutex protecting the quit channel access
	quitChan chan chan error // Quit channel to stop the metrics collection before closing the database

	logger log.Logger
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

	namespace := "consensus/istanbul/roundstate/db/"
	rsdb.compTimeMeter = metrics.NewRegisteredMeter(namespace+"compact/time", nil)
	rsdb.compReadMeter = metrics.NewRegisteredMeter(namespace+"compact/input", nil)
	rsdb.compWriteMeter = metrics.NewRegisteredMeter(namespace+"compact/output", nil)
	rsdb.diskSizeGauge = metrics.NewRegisteredGauge(namespace+"disk/size", nil)
	rsdb.diskReadMeter = metrics.NewRegisteredMeter(namespace+"disk/read", nil)
	rsdb.diskWriteMeter = metrics.NewRegisteredMeter(namespace+"disk/write", nil)
	rsdb.writeDelayMeter = metrics.NewRegisteredMeter(namespace+"compact/writedelay/duration", nil)
	rsdb.writeDelayNMeter = metrics.NewRegisteredMeter(namespace+"compact/writedelay/counter", nil)
	rsdb.memCompGauge = metrics.NewRegisteredGauge(namespace+"compact/memory", nil)
	rsdb.level0CompGauge = metrics.NewRegisteredGauge(namespace+"compact/level0", nil)
	rsdb.nonlevel0CompGauge = metrics.NewRegisteredGauge(namespace+"compact/nonlevel0", nil)
	rsdb.seekCompGauge = metrics.NewRegisteredGauge(namespace+"compact/seek", nil)
	rsdb.rsRLPMeter = metrics.NewRegisteredMeter(namespace+"rs/rlp/encoding/size", nil)
	rsdb.rsRLPEncTimer = metrics.NewRegisteredTimer(namespace+"rs/rlp/encoding/duration", nil)
	rsdb.rsDbSaveTimer = metrics.NewRegisteredTimer(namespace+"rs/db/save/time", nil)
	rsdb.rcvdRLPMeter = metrics.NewRegisteredMeter(namespace+"rcvd/rlp/encoding/size", nil)
	rsdb.rcvdRLPEncTimer = metrics.NewRegisteredTimer(namespace+"rcvd/rlp/encoding/duration", nil)
	rsdb.rcvdDbSaveTimer = metrics.NewRegisteredTimer(namespace+"rcvd/db/save/time", nil)

	// Start up the metrics gathering and return
	go rsdb.meter(metricsGatheringInterval)

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
	opts := &opt.Options{
		BlockSize:          128 * opt.KiB,
		BlockCacheCapacity: 256 * opt.MiB,
		OpenFilesCacheCapacity: 5, //120,
		WriteBuffer:            128 * opt.MiB,
		CompactionTableSize: 10 * opt.MiB,
		// CompactionTotalSize: 100 * opt.MiB,
		// CompactionTotalSizeMultiplier: 50,
		// CompactionL0Trigger: 16,
	}
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
	// Encode and measure time
	before := time.Now()
	entryBytes, err := rlp.EncodeToBytes(&rRLP)
	rsdb.rcvdRLPEncTimer.UpdateSince(before)

	if err != nil {
		logger.Error("Failed to save rcvd messages from roundState", "reason", "rlp encoding", "err", err)
		return err
	}

	rsdb.rcvdRLPMeter.Mark(int64(len(entryBytes)))

	before = time.Now()
	batch := new(leveldb.Batch)
	batch.Put(rcvdViewKey, entryBytes)
	err = rsdb.db.Write(batch, nil)
	if err != nil {
		logger.Error("Failed to save rcvd messages from roundState", "reason", "levelDB write", "err", err, "func")
	}
	rsdb.rcvdDbSaveTimer.UpdateSince(before)

	return err
}

// storeRoundState will store the currentRoundState in a Map<view, roundState> schema.
func (rsdb *roundStateDBImpl) UpdateLastRoundState(rs RoundState) error {
	// We store the roundState for each view; since we'll need this
	// information to allow the node to have evidence to show that
	// a validator did a "valid" double signing
	logger := rsdb.logger.New("func", "UpdateLastRoundState")
	viewKey := view2Key(rs.View())

	before := time.Now()
	entryBytes, err := rlp.EncodeToBytes(rs)
	rsdb.rsRLPEncTimer.UpdateSince(before)

	if err != nil {
		logger.Error("Failed to save roundState", "reason", "rlp encoding", "err", err)
		return err
	}

	rsdb.rsRLPMeter.Mark(int64(len(entryBytes)))

	before = time.Now()
	batch := new(leveldb.Batch)
	batch.Put([]byte(lastViewKey), viewKey)
	batch.Put(viewKey, entryBytes)

	err = rsdb.db.Write(batch, nil)
	if err != nil {
		logger.Error("Failed to save roundState", "reason", "levelDB write", "err", err, "func")
	}
	rsdb.rsDbSaveTimer.UpdateSince(before)

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
	// Close stops the metrics collection, flushes any pending data to disk and closes
	// all io accesses to the underlying key-value store.

	rsdb.quitLock.Lock()
	defer rsdb.quitLock.Unlock()

	if rsdb.quitChan != nil {
		errc := make(chan error)
		rsdb.quitChan <- errc
		if err := <-errc; err != nil {
			rsdb.logger.Error("Metrics collection failed", "err", err)
		}
		rsdb.quitChan = nil
	}

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

// meter periodically retrieves internal leveldb counters and reports them to
// the metrics subsystem.
//
// This is how a LevelDB stats table looks like (currently):
//
//	Compactions
//	 Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
//	-------+------------+---------------+---------------+---------------+---------------
//	   0   |          0 |       0.00000 |       1.27969 |       0.00000 |      12.31098
//	   1   |         85 |     109.27913 |      28.09293 |     213.92493 |     214.26294
//	   2   |        523 |    1000.37159 |       7.26059 |      66.86342 |      66.77884
//	   3   |        570 |    1113.18458 |       0.00000 |       0.00000 |       0.00000
//
// This is how the write delay look like (currently):
// DelayN:5 Delay:406.604657ms Paused: false
//
// This is how the iostats look like (currently):
// Read(MB):3895.04860 Write(MB):3654.64712
func (rsdb *roundStateDBImpl) meter(refresh time.Duration) {
	// Create the counters to store current and previous compaction values
	compactions := make([][]float64, 2)
	for i := 0; i < 2; i++ {
		compactions[i] = make([]float64, 4)
	}
	// Create storage for iostats.
	var iostats [2]float64

	// Create storage and warning log tracer for write delay.
	var (
		delaystats      [2]int64
		lastWritePaused time.Time
	)

	var (
		errc chan error
		merr error
	)

	timer := time.NewTimer(refresh)
	defer timer.Stop()

	// Iterate ad infinitum and collect the stats
	for i := 1; errc == nil && merr == nil; i++ {
		// Retrieve the database stats
		stats, err := rsdb.db.GetProperty("leveldb.stats")
		if err != nil {
			rsdb.logger.Error("Failed to read database stats", "err", err)
			merr = err
			continue
		}
		// Find the compaction table, skip the header
		lines := strings.Split(stats, "\n")
		for len(lines) > 0 && strings.TrimSpace(lines[0]) != "Compactions" {
			lines = lines[1:]
		}
		if len(lines) <= 3 {
			rsdb.logger.Error("Compaction leveldbTable not found")
			merr = lvlerrors.New("compaction leveldbTable not found")
			continue
		}
		lines = lines[3:]

		// Iterate over all the leveldbTable rows, and accumulate the entries
		for j := 0; j < len(compactions[i%2]); j++ {
			compactions[i%2][j] = 0
		}
		for _, line := range lines {
			parts := strings.Split(line, "|")
			if len(parts) != 6 {
				break
			}
			for idx, counter := range parts[2:] {
				value, err := strconv.ParseFloat(strings.TrimSpace(counter), 64)
				if err != nil {
					rsdb.logger.Error("Compaction entry parsing failed", "err", err)
					merr = err
					continue
				}
				compactions[i%2][idx] += value
			}
		}
		// Update all the requested meters
		if rsdb.diskSizeGauge != nil {
			rsdb.diskSizeGauge.Update(int64(compactions[i%2][0] * 1024 * 1024))
		}
		if rsdb.compTimeMeter != nil {
			rsdb.compTimeMeter.Mark(int64((compactions[i%2][1] - compactions[(i-1)%2][1]) * 1000 * 1000 * 1000))
		}
		if rsdb.compReadMeter != nil {
			rsdb.compReadMeter.Mark(int64((compactions[i%2][2] - compactions[(i-1)%2][2]) * 1024 * 1024))
		}
		if rsdb.compWriteMeter != nil {
			rsdb.compWriteMeter.Mark(int64((compactions[i%2][3] - compactions[(i-1)%2][3]) * 1024 * 1024))
		}
		// Retrieve the write delay statistic
		writedelay, err := rsdb.db.GetProperty("leveldb.writedelay")
		if err != nil {
			rsdb.logger.Error("Failed to read database write delay statistic", "err", err)
			merr = err
			continue
		}
		var (
			delayN        int64
			delayDuration string
			duration      time.Duration
			paused        bool
		)
		if n, err := fmt.Sscanf(writedelay, "DelayN:%d Delay:%s Paused:%t", &delayN, &delayDuration, &paused); n != 3 || err != nil {
			rsdb.logger.Error("Write delay statistic not found")
			merr = err
			continue
		}
		duration, err = time.ParseDuration(delayDuration)
		if err != nil {
			rsdb.logger.Error("Failed to parse delay duration", "err", err)
			merr = err
			continue
		}
		if rsdb.writeDelayNMeter != nil {
			rsdb.writeDelayNMeter.Mark(delayN - delaystats[0])
		}
		if rsdb.writeDelayMeter != nil {
			rsdb.writeDelayMeter.Mark(duration.Nanoseconds() - delaystats[1])
		}
		// If a warning that db is performing compaction has been displayed, any subsequent
		// warnings will be withheld for one minute not to overwhelm the user.
		if paused && delayN-delaystats[0] == 0 && duration.Nanoseconds()-delaystats[1] == 0 &&
			time.Now().After(lastWritePaused.Add(degradationWarnInterval)) {
			rsdb.logger.Warn("Database compacting, degraded performance")
			lastWritePaused = time.Now()
		}
		delaystats[0], delaystats[1] = delayN, duration.Nanoseconds()

		// Retrieve the database iostats.
		ioStats, err := rsdb.db.GetProperty("leveldb.iostats")
		if err != nil {
			rsdb.logger.Error("Failed to read database iostats", "err", err)
			merr = err
			continue
		}
		var nRead, nWrite float64
		parts := strings.Split(ioStats, " ")
		if len(parts) < 2 {
			rsdb.logger.Error("Bad syntax of ioStats", "ioStats", ioStats)
			merr = fmt.Errorf("bad syntax of ioStats %s", ioStats)
			continue
		}
		if n, err := fmt.Sscanf(parts[0], "Read(MB):%f", &nRead); n != 1 || err != nil {
			rsdb.logger.Error("Bad syntax of read entry", "entry", parts[0])
			merr = err
			continue
		}
		if n, err := fmt.Sscanf(parts[1], "Write(MB):%f", &nWrite); n != 1 || err != nil {
			rsdb.logger.Error("Bad syntax of write entry", "entry", parts[1])
			merr = err
			continue
		}
		if rsdb.diskReadMeter != nil {
			rsdb.diskReadMeter.Mark(int64((nRead - iostats[0]) * 1024 * 1024))
		}
		if rsdb.diskWriteMeter != nil {
			rsdb.diskWriteMeter.Mark(int64((nWrite - iostats[1]) * 1024 * 1024))
		}
		iostats[0], iostats[1] = nRead, nWrite

		compCount, err := rsdb.db.GetProperty("leveldb.compcount")
		if err != nil {
			rsdb.logger.Error("Failed to read database iostats", "err", err)
			merr = err
			continue
		}

		var (
			memComp       uint32
			level0Comp    uint32
			nonLevel0Comp uint32
			seekComp      uint32
		)
		if n, err := fmt.Sscanf(compCount, "MemComp:%d Level0Comp:%d NonLevel0Comp:%d SeekComp:%d", &memComp, &level0Comp, &nonLevel0Comp, &seekComp); n != 4 || err != nil {
			rsdb.logger.Error("Compaction count statistic not found")
			merr = err
			continue
		}
		rsdb.memCompGauge.Update(int64(memComp))
		rsdb.level0CompGauge.Update(int64(level0Comp))
		rsdb.nonlevel0CompGauge.Update(int64(nonLevel0Comp))
		rsdb.seekCompGauge.Update(int64(seekComp))

		// Sleep a bit, then repeat the stats collection
		select {
		case errc = <-rsdb.quitChan:
			// Quit requesting, stop hammering the database
		case <-timer.C:
			timer.Reset(refresh)
			// Timeout, gather a new set of stats
		}
	}

	if errc == nil {
		errc = <-rsdb.quitChan
	}
	errc <- merr
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
