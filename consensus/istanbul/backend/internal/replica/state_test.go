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
	"math/big"
	"testing"
)

func noop() error {
	return nil
}

func (rs *replicaStateImpl) CheckRSDB() error {
	// load DB
	loaded, err := rs.rsdb.GetReplicaState()
	if err != nil {
		return err
	}
	if loaded == nil {
		return errors.New("Could not load rsdb")
	}
	if loaded.state != rs.state {
		return fmt.Errorf("Expected loaded state to equal rs. loaded: %v; rs: %v.", loaded.state, rs.state)
	}
	hasLoadedStart := loaded.startValidatingBlock != nil
	hasRsStart := rs.startValidatingBlock != nil
	hasLoadedStop := loaded.stopValidatingBlock != nil
	hasRsStop := rs.stopValidatingBlock != nil

	if !hasLoadedStart && !hasRsStart {
		// pass
	} else if !hasLoadedStart || !hasRsStart {
		return fmt.Errorf("Expected loaded start block to equal rs. loaded: %v; rs: %v.", loaded.startValidatingBlock, rs.startValidatingBlock)
	} else if loaded.startValidatingBlock.Cmp(rs.startValidatingBlock) != 0 {
		return fmt.Errorf("Expected loaded start block to equal rs. loaded: %v; rs: %v.", loaded.startValidatingBlock, rs.startValidatingBlock)
	}
	if !hasLoadedStop && !hasRsStop {
		// pass
	} else if !hasLoadedStop || !hasRsStop {
		return fmt.Errorf("Expected loaded stop block to equal rs. loaded: %v; rs: %v.", loaded.stopValidatingBlock, rs.stopValidatingBlock)
	} else if loaded.stopValidatingBlock.Cmp(rs.stopValidatingBlock) != 0 {
		return fmt.Errorf("Expected loaded stop bloc to equal rs. loaded: %v; rs: %v.", loaded.stopValidatingBlock, rs.stopValidatingBlock)
	}
	return nil
}

func TestIsPrimaryForSeq(t *testing.T) {
	t.Run("permanent primary", func(t *testing.T) {

		seqs := []int64{0, 1, 2, 4, 8, 16, 32, 64, 128}
		rsState, _ := NewState(false, "", noop, noop)
		rs := rsState.(*replicaStateImpl)
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)
			if err := rs.CheckRSDB(); err != nil {
				t.Errorf("expected RSDB to be the same for seq %v, err: %v", seq, err)
			}

			if !primary {
				t.Errorf("expected to be primary for seq %v", seq)
			}
		}
	})

	t.Run("permanent replica", func(t *testing.T) {
		seqs := []int64{0, 1, 2, 4, 8, 16, 32, 64, 128}
		rsState, _ := NewState(true, "", noop, noop)
		rs := rsState.(*replicaStateImpl)
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)
			if err := rs.CheckRSDB(); err != nil {
				t.Errorf("expected RSDB to be the same for seq %v, err: %v", seq, err)
			}

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
		rsState, _ := NewState(true, "", noop, noop)
		rs := rsState.(*replicaStateImpl)
		rs.SetStartValidatingBlock(big.NewInt(200))
		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)
			if err := rs.CheckRSDB(); err != nil {
				t.Errorf("expected RSDB to be the same for seq %v, err: %v", seq, err)
			}

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
			if err := rs.CheckRSDB(); err != nil {
				t.Errorf("expected RSDB to be the same for seq %v, err: %v", seq, err)
			}

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
		rsState, _ := NewState(true, "", noop, noop)
		rs := rsState.(*replicaStateImpl)
		rs.SetStartValidatingBlock(big.NewInt(200))
		rs.SetStopValidatingBlock(big.NewInt(210))

		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)
			if err := rs.CheckRSDB(); err != nil {
				t.Errorf("expected RSDB to be the same for seq %v, err: %v", seq, err)
			}

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
			if err := rs.CheckRSDB(); err != nil {
				t.Errorf("expected RSDB to be the same for seq %v, err: %v", seq, err)
			}

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
			if err := rs.CheckRSDB(); err != nil {
				t.Errorf("expected RSDB to be the same for seq %v, err: %v", seq, err)
			}

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
		rsState, _ := NewState(false, "", noop, noop)
		rs := rsState.(*replicaStateImpl)
		rs.SetStopValidatingBlock(big.NewInt(210))

		for _, seq := range seqs {
			n := big.NewInt(seq)
			primary := rs.IsPrimaryForSeq(n)
			rs.NewChainHead(n)
			if err := rs.CheckRSDB(); err != nil {
				t.Errorf("expected RSDB to be the same for seq %v, err: %v", seq, err)
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
			if err := rs.CheckRSDB(); err != nil {
				t.Errorf("expected RSDB to be the same for seq %v, err: %v", seq, err)
			}

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

	t.Run("Respects start/stop block ordering", func(t *testing.T) {
		rsState, _ := NewState(true, "", noop, noop)
		rs := rsState.(*replicaStateImpl)
		rs.state = replicaWaiting
		rs.SetStopValidatingBlock(big.NewInt(10))

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

	//start <= seq < stop
	t.Run("Respects start/stop block ordering", func(t *testing.T) {
		rsState, _ := NewState(true, "", noop, noop)
		rs := rsState.(*replicaStateImpl)
		rs.SetStartValidatingBlock(big.NewInt(10))

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
