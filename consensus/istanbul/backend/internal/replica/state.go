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
	"io"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

type State interface {
	// mutation functions
	SetStartValidatingBlock(blockNumber *big.Int) error
	SetStopValidatingBlock(blockNumber *big.Int) error
	MakeReplica()
	MakePrimary()
	UpdateReplicaState(newSeq *big.Int)

	// view functions
	IsPrimaryForSeq(seq *big.Int) bool
	Summary() *ReplicaStateSummary
}

// ReplicaState stores info on this node being a primary or replica
type replicaStateImpl struct {
	isReplica            bool // Overridden by start/stop blocks if start/stop is enabled.
	enabled              bool
	startValidatingBlock *big.Int
	stopValidatingBlock  *big.Int

	rsdb *ReplicaStateDB
	mu   *sync.RWMutex
}

func NewState(isReplica bool, path string) State {
	db, err := OpenReplicaStateDB(path)
	if err != nil {
		log.Crit("Can't open ReplicaStateDB", "err", err, "dbpath", path)
	}
	return &replicaStateImpl{
		isReplica: isReplica,
		mu:        new(sync.RWMutex),
		rsdb:      db,
	}
}

func (rs *replicaStateImpl) SetStartValidatingBlock(blockNumber *big.Int) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	defer rs.rsdb.StoreReplicaState(rs)

	if blockNumber == nil {
		rs.startValidatingBlock = nil
		return nil
	}

	if rs.stopValidatingBlock != nil && !(blockNumber.Cmp(rs.stopValidatingBlock) < 0) {
		return errors.New("Start block number should be less than the stop block number")
	}

	rs.enabled = true
	rs.startValidatingBlock = blockNumber
	return nil
}

func (rs *replicaStateImpl) SetStopValidatingBlock(blockNumber *big.Int) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	defer rs.rsdb.StoreReplicaState(rs)

	if blockNumber == nil {
		rs.stopValidatingBlock = nil
		return nil
	}

	if rs.startValidatingBlock != nil && !(blockNumber.Cmp(rs.startValidatingBlock) > 0) {
		return errors.New("Stop block number should be greater than the start block number")
	}

	rs.enabled = true
	rs.stopValidatingBlock = blockNumber
	return nil
}

func (rs *replicaStateImpl) MakeReplica() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.makeReplica()
}

// Must be called w/ rs.mu held
func (rs *replicaStateImpl) makeReplica() {
	defer rs.rsdb.StoreReplicaState(rs)

	rs.isReplica = true
	rs.enabled = false
	rs.startValidatingBlock = nil
	rs.stopValidatingBlock = nil
}

func (rs *replicaStateImpl) MakePrimary() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.makePrimary()
}

// Must be called w/ rs.mu held
func (rs *replicaStateImpl) makePrimary() {
	defer rs.rsdb.StoreReplicaState(rs)

	rs.isReplica = false
	rs.enabled = false
	rs.startValidatingBlock = nil
	rs.stopValidatingBlock = nil
}

// UpdateReplicaState will clear start/stop state and transition the validator
// to being a replica/primary if it enters/leaves a start-stop range.
func (rs *replicaStateImpl) UpdateReplicaState(newSeq *big.Int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	// Transition to Primary/Replica
	if !rs.enabled {
		return
	}
	// start <= seq w/ no stop -> primary
	if rs.startValidatingBlock != nil && newSeq.Cmp(rs.startValidatingBlock) > 0 {
		if rs.stopValidatingBlock == nil {
			rs.makePrimary()
		}
	}
	// start <= stop <= seq -> replica
	if rs.stopValidatingBlock != nil && newSeq.Cmp(rs.stopValidatingBlock) > 0 {
		rs.makeReplica()
	}

}

// IsPrimaryForSeq determines is this node is the primary validator.
// If start/stop checking is enabled (via a call to start/stop at block)
// determine if start <= seq < stop. If not enabled, check if this was
// set up with replica mode.
func (rs *replicaStateImpl) IsPrimaryForSeq(seq *big.Int) bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if !rs.enabled {
		return !rs.isReplica
	}
	// Return start <= seq < stop with start/stop at +-inf if nil
	if rs.startValidatingBlock != nil && seq.Cmp(rs.startValidatingBlock) < 0 {
		return false
	}
	if rs.stopValidatingBlock != nil && seq.Cmp(rs.stopValidatingBlock) >= 0 {
		return false
	}
	return true
}

type ReplicaStateSummary struct {
	State                string   `json:"state"`
	Enabled              bool     `json:"enabled"`
	IsReplica            bool     `json:"isReplica"`
	StartValidatingBlock *big.Int `json:"startValidatingBlock"`
	StopValidatingBlock  *big.Int `json:"stopValidatingBlock"`
}

func (rs *replicaStateImpl) Summary() *ReplicaStateSummary {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	// String explanation of replica state
	var state string
	if rs.isReplica && !rs.enabled {
		state = "Replica"
	} else if rs.isReplica && rs.enabled {
		state = "Replica waiting to start"
	} else if !rs.isReplica && !rs.enabled {
		state = "Primary"
	} else if !rs.isReplica && rs.enabled {
		state = "Primary in given range"
	}

	summary := &ReplicaStateSummary{
		State:                state,
		IsReplica:            rs.isReplica,
		Enabled:              rs.enabled,
		StartValidatingBlock: rs.startValidatingBlock,
		StopValidatingBlock:  rs.stopValidatingBlock,
	}

	return summary
}

type replicaStateRLP struct {
	IsReplica            bool
	Enabled              bool
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
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	entry := replicaStateRLP{
		IsReplica:            rs.isReplica,
		Enabled:              rs.enabled,
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

	rs.mu = new(sync.RWMutex)
	rs.isReplica = data.IsReplica
	rs.enabled = data.Enabled
	rs.startValidatingBlock = data.StartValidatingBlock
	rs.stopValidatingBlock = data.StopValidatingBlock

	return nil
}
