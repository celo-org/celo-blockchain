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
	"math/big"
	"sync"
	"testing"
)

func noop() error {
	return nil
}

func TestIsPrimaryForSeq(t *testing.T) {
	rsdb, _ := OpenReplicaStateDB("")

	t.Run("permanent primary", func(t *testing.T) {

		seqs := []int64{0, 1, 2, 4, 8, 16, 32, 64, 128}
		rs := &replicaStateImpl{
			state:   primaryPermanent,
			mu:      new(sync.RWMutex),
			rsdb:    rsdb,
			startFn: noop,
			stopFn:  noop,
		}
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)

			if !primary {
				t.Errorf("expected to be primary for seq %v", seq)
			}
		}
	})

	t.Run("permanent replica", func(t *testing.T) {
		seqs := []int64{0, 1, 2, 4, 8, 16, 32, 64, 128}
		rs := &replicaStateImpl{
			state:   replicaPermanent,
			mu:      new(sync.RWMutex),
			rsdb:    rsdb,
			startFn: noop,
			stopFn:  noop,
		}
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)

			if primary {
				t.Errorf("expected to be replica for seq %v", seq)
			}
			primary = rs.IsPrimary()
			if primary {
				t.Errorf("expected to be replica for seq %v", seq)
			}
		}
	})

	t.Run("replica waiting", func(t *testing.T) {
		seqs := []int64{1, 2, 4, 8, 16, 32, 64, 128}
		rs := &replicaStateImpl{
			state:                replicaWaiting,
			startValidatingBlock: big.NewInt(200),
			mu:                   new(sync.RWMutex),
			rsdb:                 rsdb,
			startFn:              noop,
			stopFn:               noop,
		}
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)
			if primary {
				t.Errorf("expected to be replica for seq %v", seq)
			}
			primary = rs.IsPrimary()
			if primary {
				t.Errorf("expected to be replica for seq %v", seq)
			}
		}
		seqs = []int64{200, 205, 210}
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)

			if !primary {
				t.Errorf("expected to be primary for seq %v", seq)
			}
			primary = rs.IsPrimary()
			if !primary {
				t.Errorf("expected to be primary for seq %v", seq)
			}
		}

	})

	t.Run("replica waiting to primary in range to permanent replica", func(t *testing.T) {
		seqs := []int64{1, 2, 4, 8, 16, 32, 64, 128}
		rs := &replicaStateImpl{
			state:                replicaWaiting,
			stopValidatingBlock:  big.NewInt(210),
			startValidatingBlock: big.NewInt(200),
			mu:                   new(sync.RWMutex),
			rsdb:                 rsdb,
			startFn:              noop,
			stopFn:               noop,
		}
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)

			if primary {
				t.Errorf("expected to be replica for seq %v", seq)
			}
			primary = rs.IsPrimary()
			if primary {
				t.Errorf("expected to be replica for seq %v", seq)
			}
		}
		seqs = []int64{200, 205, 209}
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)
			if rs.state != primaryInRange {
				t.Errorf("expected rs.state to be %v, got %v", primaryInRange, rs.state)
			}

			if !primary {
				t.Errorf("expected to be primary for seq %v", seq)
			}
			primary = rs.IsPrimary()
			if !primary {
				t.Errorf("expected to be primary for seq %v", seq)
			}
		}
		seqs = []int64{210, 211, 220}
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)

			if primary {
				t.Errorf("expected to be replica for seq %v", seq)
			}
			primary = rs.IsPrimary()
			if primary {
				t.Errorf("expected to be replica for seq %v", seq)
			}
		}
	})

	t.Run("primary in range to permanent replica", func(t *testing.T) {
		seqs := []int64{1, 2, 4, 8, 16, 32, 64, 128, 209}
		rs := &replicaStateImpl{
			state:               primaryInRange,
			stopValidatingBlock: big.NewInt(210),
			mu:                  new(sync.RWMutex),
			rsdb:                rsdb,
			startFn:             noop,
			stopFn:              noop,
		}
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)

			if !primary {
				t.Errorf("expected to be primary for seq %v", seq)
			}
			primary = rs.IsPrimary()
			if !primary {
				t.Errorf("expected to be primary for seq %v", seq)
			}
		}

		seqs = []int64{210, 211, 220}
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)

			if primary {
				t.Errorf("expected to be replica for seq %v", seq)
			}
			primary = rs.IsPrimary()
			if primary {
				t.Errorf("expected to be replica for seq %v", seq)
			}
		}
	})

}

func TestSetStartValidatingBlock(t *testing.T) {
	rsdb, _ := OpenReplicaStateDB("")

	t.Run("Respects start/stop block ordering", func(t *testing.T) {
		rs := &replicaStateImpl{
			state:               replicaWaiting,
			stopValidatingBlock: big.NewInt(10),
			mu:                  new(sync.RWMutex),
			rsdb:                rsdb,
		}
		err := rs.SetStartValidatingBlock(big.NewInt(11))
		if err == nil {
			t.Errorf("error mismatch: have %v, want %v", err, errors.New("Start block number should be less than the stop block number"))
		}
		err = rs.SetStartValidatingBlock(big.NewInt(9))
		if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}

	})

}

func TestSetStopValidatingBlock(t *testing.T) {
	rsdb, _ := OpenReplicaStateDB("")

	//start <= seq < stop
	t.Run("Respects start/stop block ordering", func(t *testing.T) {
		rs := &replicaStateImpl{
			state:                replicaWaiting,
			startValidatingBlock: big.NewInt(10),
			mu:                   new(sync.RWMutex),
			rsdb:                 rsdb,
		}
		err := rs.SetStopValidatingBlock(big.NewInt(9))
		if err == nil {
			t.Errorf("error mismatch: have %v, want %v", err, errors.New("Stop block number should be greater than the start block number"))
		}
		err = rs.SetStopValidatingBlock(big.NewInt(10))
		if err == nil {
			t.Errorf("error mismatch: have %v, want %v", err, errors.New("Stop block number should be greater than the start block number"))
		}
		err = rs.SetStopValidatingBlock(big.NewInt(11))
		if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}

	})

}
