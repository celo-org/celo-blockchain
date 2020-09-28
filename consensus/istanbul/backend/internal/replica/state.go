// Copyright 2020 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package replica

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	lvlerrors "github.com/syndtr/goleveldb/leveldb/errors"
)

type state uint64

// Different possible states for validators wrt replica/primary
// Set start & stop block to the range [start, stop)
// Permanant primary / replica when the node will not change state in the future
// primaryInRange when inside the range [start, stop)
// replicaWaiting when before the startValidatingBlock
const (
	primaryPermanent state = iota
	primaryInRange
	replicaPermanent
	replicaWaiting
)

func (s state) String() string {
	switch s {
	case primaryPermanent:
		return "Primary"
	case primaryInRange:
		return "Primary in given range"
	case replicaPermanent:
		return "Replica"
	case replicaWaiting:
		return "Replica waiting to start"
	}
	return "Unknown"
}

type State interface {
	// SetStartValidatingBlock
	SetStartValidatingBlock(blockNumber *big.Int) error
	SetStopValidatingBlock(blockNumber *big.Int) error
	NewChainHead(blockNumber *big.Int) error
	MakeReplica() error
	MakePrimary() error
	Close() error

	// view functions
	IsPrimary() bool
	IsPrimaryForSeq(blockNumber *big.Int) bool
	Summary() *ReplicaStateSummary
}

// ReplicaState stores info on this node being a primary or replica
type replicaStateImpl struct {
	state                state
	startValidatingBlock *big.Int
	stopValidatingBlock  *big.Int

	rsdb *ReplicaStateDB
	mu   *sync.RWMutex

	startFn func() error
	stopFn  func() error
}

// NewState creates a replicaState in the given replica state and opens or creates the replica state DB at `path`.
func NewState(isReplica bool, path string, startFn, stopFn func() error) State {
	db, err := OpenReplicaStateDB(path)
	if err != nil {
		log.Crit("Can't open ReplicaStateDB", "err", err, "dbpath", path)
	}
	rs, err := db.GetReplicaState()
	// First startup
	if err == lvlerrors.ErrNotFound {
		var state state
		if isReplica {
			state = replicaPermanent
		} else {
			state = primaryPermanent
		}
		rs = &replicaStateImpl{
			state: state,
			mu:    new(sync.RWMutex),
		}
	} else if err != nil {
		log.Warn("Can't read ReplicaStateDB at startup", "err", err, "dbpath", path)
	}
	rs.rsdb = db
	rs.startFn = startFn
	rs.stopFn = stopFn
	db.StoreReplicaState(rs)
	return rs
}

// Close closes the replica state database
func (rs *replicaStateImpl) Close() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.rsdb.Close()
}

// NewChainHead updates replica state and starts/stops the core if needed
func (rs *replicaStateImpl) NewChainHead(blockNumber *big.Int) error {
	logger := log.New("func", "NewChainHead", "seq", blockNumber)
	switch rs.state {
	case primaryInRange:
		if blockNumber.Cmp(rs.stopValidatingBlock) >= 0 {
			logger.Info("About to stop validating")
			if err := rs.stopFn(); err != nil {
				logger.Warn("Error stopping core", "err", err)
				return err
			}
			defer rs.rsdb.StoreReplicaState(rs)
			rs.state = replicaPermanent
			rs.startValidatingBlock = nil
			rs.stopValidatingBlock = nil
		}
	case replicaWaiting:
		if blockNumber.Cmp(rs.startValidatingBlock) >= 0 {
			if err := rs.startFn(); err != nil {
				logger.Warn("Error starting core", "err", err)
				return err
			}
			defer rs.rsdb.StoreReplicaState(rs)
			if rs.stopValidatingBlock == nil {
				logger.Info("Switching to primary (permanent)")
				rs.state = primaryPermanent
				rs.startValidatingBlock = nil
				rs.stopValidatingBlock = nil
			} else {
				logger.Info("Switching to primary in range")
				rs.state = primaryInRange
			}
		}
	default:
		// pass
	}
	return nil
}

// SetStartValidatingBlock sets the start block in the range [start, stop)
func (rs *replicaStateImpl) SetStartValidatingBlock(blockNumber *big.Int) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	defer rs.rsdb.StoreReplicaState(rs)

	if blockNumber.Cmp(common.Big0) <= 0 {
		return errors.New("blockNumber must be > 0")
	}
	if rs.stopValidatingBlock != nil && blockNumber.Cmp(rs.stopValidatingBlock) >= 0 {
		return errors.New("Start block number should be less than the stop block number")
	}

	switch rs.state {
	case replicaPermanent:
		rs.state = replicaWaiting
	case replicaWaiting:
		// pass. Changed start block while waiting to start.
	default:
		return fmt.Errorf("Can't change set start validating block when primary (%v)", rs.state)
	}
	rs.startValidatingBlock = blockNumber

	return nil
}

// SetStopValidatingBlock sets the stop block in the range [start, stop)
func (rs *replicaStateImpl) SetStopValidatingBlock(blockNumber *big.Int) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	defer rs.rsdb.StoreReplicaState(rs)

	if blockNumber.Cmp(common.Big0) <= 0 {
		return errors.New("blockNumber must be > 0")
	}
	if rs.startValidatingBlock != nil && !(blockNumber.Cmp(rs.startValidatingBlock) > 0) {
		return errors.New("Stop block number should be greater than the start block number")
	}

	switch rs.state {
	case primaryPermanent:
		rs.state = primaryInRange
	case primaryInRange:
		// pass. Changes stop block while waiting to stop.
	case replicaPermanent:
		return errors.New("Can't change stop validating block when permanent replica")
	case replicaWaiting:
		// pass. Changed stop block while waiting to start.
	}
	rs.stopValidatingBlock = blockNumber

	return nil
}

// MakeReplica makes this node a replica & clears all start/stop blocks.
func (rs *replicaStateImpl) MakeReplica() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	defer rs.rsdb.StoreReplicaState(rs)

	if rs.state == primaryPermanent || rs.state == primaryInRange {
		if err := rs.stopFn(); err != nil {
			return err
		}
	}
	rs.startValidatingBlock = nil
	rs.stopValidatingBlock = nil
	rs.state = replicaPermanent
	return nil
}

// MakePrimary makes this node a primary & clears all start/stop blocks.
func (rs *replicaStateImpl) MakePrimary() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	defer rs.rsdb.StoreReplicaState(rs)

	if rs.state == replicaPermanent || rs.state == replicaWaiting {
		if err := rs.startFn(); err != nil {
			return err
		}
	}
	rs.startValidatingBlock = nil
	rs.stopValidatingBlock = nil
	rs.state = primaryPermanent
	return nil
}

// IsPrimary determines is this node is the primary validator.
func (rs *replicaStateImpl) IsPrimary() bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.state == primaryPermanent || rs.state == primaryInRange
}

// IsPrimaryForSeq determines is this node is the primary validator.
// If start/stop checking is enabled (via a call to start/stop at block)
// determine if start <= seq < stop. If not enabled, check if this was
// set up with replica mode.
func (rs *replicaStateImpl) IsPrimaryForSeq(seq *big.Int) bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	switch rs.state {
	case primaryPermanent:
		return true
	case replicaPermanent:
		return false
	case replicaWaiting:
		return seq.Cmp(rs.startValidatingBlock) >= 0
	case primaryInRange:
		return seq.Cmp(rs.stopValidatingBlock) < 0
	}
	return false
}

type ReplicaStateSummary struct {
	State                string   `json:"state"`
	IsPrimary            bool     `json:"isPrimary"`
	StartValidatingBlock *big.Int `json:"startValidatingBlock"`
	StopValidatingBlock  *big.Int `json:"stopValidatingBlock"`
}

func (rs *replicaStateImpl) Summary() *ReplicaStateSummary {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	// String explanation of replica state
	var state string
	switch rs.state {
	case primaryPermanent:
		state = "Primary"
	case primaryInRange:
		state = "Primary in given range"
	case replicaPermanent:
		state = "Replica"
	case replicaWaiting:
		state = "Replica waiting to start"
	}

	summary := &ReplicaStateSummary{
		State:                state,
		IsPrimary:            rs.state == primaryPermanent || rs.state == primaryInRange,
		StartValidatingBlock: rs.startValidatingBlock,
		StopValidatingBlock:  rs.stopValidatingBlock,
	}

	return summary
}

type replicaStateRLP struct {
	State                state
	StartValidatingBlock *big.Int
	StopValidatingBlock  *big.Int
}

// EncodeRLP should write the RLP encoding of its receiver to w.
// If the implementation is a pointer method, it may also be
// called for nil pointers.
//
// Implementations should generate valid RLP. The data written is
// not verified at the moment, but a future version might. It is
// recommended to write only a single value but writing multiple
// values or no value at all is also permitted.
func (rs *replicaStateImpl) EncodeRLP(w io.Writer) error {
	entry := replicaStateRLP{
		State:                rs.state,
		StartValidatingBlock: rs.startValidatingBlock,
		StopValidatingBlock:  rs.stopValidatingBlock,
	}
	return rlp.Encode(w, entry)
}

// The DecodeRLP method should read one value from the given
// Stream. It is not forbidden to read less or more, but it might
// be confusing.
func (rs *replicaStateImpl) DecodeRLP(stream *rlp.Stream) error {
	var data replicaStateRLP
	err := stream.Decode(&data)
	if err != nil {
		return err
	}
	log.Warn("decode replica state RLP", "startValidatingBlock", data.StartValidatingBlock, "stopValidatingBlock", data.StopValidatingBlock)

	rs.mu = new(sync.RWMutex)
	rs.state = data.State
	if data.StartValidatingBlock.Cmp(common.Big0) == 0 {
		rs.startValidatingBlock = nil
	} else {
		rs.startValidatingBlock = data.StartValidatingBlock

	}
	if data.StopValidatingBlock.Cmp(common.Big0) == 0 {
		rs.stopValidatingBlock = nil
	} else {
		rs.stopValidatingBlock = data.StopValidatingBlock
	}

	return nil
}
