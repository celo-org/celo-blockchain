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
	"math"
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that handleMsg handles messages as expected. I.E. correctly rejects
// messages that are too old/new/invalid and not rejecting valid messages.
// Note that due to the way rlp decoding works nil pointers are not possible in
// a decoded object, thus we do not attempt to check messages with missing
// values in this test.
func TestHandleMsg(t *testing.T) {
	N := uint64(4)
	F := uint64(1)

	config := *istanbul.DefaultConfig
	config.ProposerPolicy = istanbul.RoundRobin
	config.RoundStateDBPath = ""
	config.RequestTimeout = math.MaxUint64 // We don't want the rounds changing unless we set them
	sys := newTestSystemWithBackendConfigured(N, F, config)

	closer := sys.Run(true)
	defer closer()

	b := sys.backends[0]
	c := b.engine.(*core)
	block1 := makeBlock(1)

	// Sanity check valid preprepare is handled without error
	currentSequence := c.current.Sequence()
	desiredRound := c.current.DesiredRound()
	m := istanbul.NewPreprepareMessage(&istanbul.Preprepare{
		View: &istanbul.View{
			Sequence: currentSequence,
			Round:    desiredRound,
		},
		Proposal: block1,
	}, b.address)

	err := m.Sign(b.Sign)
	require.NoError(t, err)
	payload, err := m.Payload()
	require.NoError(t, err)
	err = c.handleMsg(payload)
	require.NoError(t, err)

	// check simplifies signing and encoding the message, calling
	// handleMsg and checking the result.
	check := func(t *testing.T, m *istanbul.Message, c *core, expectError bool) {
		err := m.Sign(b.Sign)
		require.NoError(t, err)
		payload, err := m.Payload()
		require.NoError(t, err)
		err = c.handleMsg(payload)
		if expectError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}

	checkErr := func(t *testing.T, m *istanbul.Message, c *core) {
		check(t, m, c, true)
	}
	checkNoErr := func(t *testing.T, m *istanbul.Message, c *core) {
		check(t, m, c, false)
	}

	testStates := []State{StateAcceptRequest, StatePreprepared, StatePrepared, StateCommitted, StateWaitingForNewRound}
	t.Run("Rejects messages with invalid code", func(t *testing.T) {
		for _, testState := range testStates {
			c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = testState
			m = istanbul.NewPrepareMessage(&istanbul.Subject{
				View: &istanbul.View{
					Sequence: currentSequence,
					Round:    desiredRound,
				},
				Digest: block1.Hash(),
			}, b.address)
			m.Code = 99
			checkErr(t, m, c)
		}
	})

	t.Run("Rejects non consensus messages", func(t *testing.T) {
		for _, testState := range testStates {
			c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = testState
			m = istanbul.NewPrepareMessage(&istanbul.Subject{
				View: &istanbul.View{
					Sequence: currentSequence,
					Round:    desiredRound,
				},
				Digest: block1.Hash(),
			}, b.address)
			p, err := m.Payload()
			require.NoError(t, err)
			m = istanbul.NewForwardMessage(&istanbul.ForwardMessage{
				Code:          m.Code,
				Msg:           p,
				DestAddresses: []common.Address{{}},
			}, b.address)
			checkErr(t, m, c)
		}
	})

	t.Run("Commit for last subject", func(t *testing.T) {
		v := &istanbul.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(1),
		}
		for _, testState := range testStates {
			c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = testState

			sub := &istanbul.Subject{
				View:   v,
				Digest: common.Hash{},
			}
			committedSeal, err := c.generateCommittedSeal(sub)
			require.NoError(t, err)
			m = istanbul.NewCommitMessage(&istanbul.CommittedSubject{
				Subject:       sub,
				CommittedSeal: committedSeal[:],
			}, b.Address())
			checkErr(t, m, c)
		}
	})

	checkErrMsgTypes := func(msgView *istanbul.View, c *core) {
		m := istanbul.NewPreprepareMessage(&istanbul.Preprepare{
			View:     msgView,
			Proposal: block1,
		}, b.address)
		check(t, m, c, true)

		m = istanbul.NewPrepareMessage(&istanbul.Subject{
			View:   msgView,
			Digest: block1.Hash(),
		}, b.Address())
		check(t, m, c, true)

		sub := &istanbul.Subject{
			View:   msgView,
			Digest: block1.Hash(),
		}
		committedSeal, err := c.generateCommittedSeal(sub)
		require.NoError(t, err)
		m = istanbul.NewCommitMessage(&istanbul.CommittedSubject{
			Subject:       sub,
			CommittedSeal: committedSeal[:],
		}, b.Address())
		check(t, m, c, true)
		m = istanbul.NewRoundChangeMessage(&istanbul.RoundChange{
			View:                msgView,
			PreparedCertificate: istanbul.EmptyPreparedCertificate(),
		}, b.Address())
		check(t, m, c, true)
	}

	// Set the current sequence and round such that we can send old messages.
	c.current.(*rsSaveDecorator).rs.(*roundStateImpl).round = big.NewInt(2)
	c.current.(*rsSaveDecorator).rs.(*roundStateImpl).desiredRound = big.NewInt(2)
	c.current.(*rsSaveDecorator).rs.(*roundStateImpl).sequence = big.NewInt(2)

	t.Run("Rejects older sequences", func(t *testing.T) {
		v := &istanbul.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(2),
		}
		for _, testState := range testStates {
			c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = testState
			checkErrMsgTypes(v, c)
		}
	})

	t.Run("Rejects older rounds", func(t *testing.T) {
		v := &istanbul.View{
			Sequence: big.NewInt(2),
			Round:    big.NewInt(1),
		}
		for _, testState := range testStates {
			c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = testState
			checkErrMsgTypes(v, c)
		}
	})

	t.Run("Rejects future sequences", func(t *testing.T) {
		v := &istanbul.View{
			Sequence: big.NewInt(3),
			Round:    big.NewInt(2),
		}
		for _, testState := range testStates {
			c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = testState
			checkErrMsgTypes(v, c)
		}
	})

	t.Run("Future rounds", func(t *testing.T) {
		v := &istanbul.View{
			Sequence: big.NewInt(2),
			Round:    big.NewInt(3),
		}
		for _, testState := range testStates {
			// All message types except for round changes are rejected if they
			// have a future round.

			c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = testState
			m := istanbul.NewPreprepareMessage(&istanbul.Preprepare{
				View:     v,
				Proposal: block1,
			}, b.address)
			checkErr(t, m, c)

			m = istanbul.NewPrepareMessage(&istanbul.Subject{
				View:   v,
				Digest: block1.Hash(),
			}, b.Address())
			checkErr(t, m, c)

			sub := &istanbul.Subject{
				View:   v,
				Digest: block1.Hash(),
			}
			committedSeal, err := c.generateCommittedSeal(sub)
			require.NoError(t, err)
			m = istanbul.NewCommitMessage(&istanbul.CommittedSubject{
				Subject:       sub,
				CommittedSeal: committedSeal[:],
			}, b.Address())
			checkErr(t, m, c)

			m = istanbul.NewRoundChangeMessage(&istanbul.RoundChange{
				View:                v,
				PreparedCertificate: istanbul.EmptyPreparedCertificate(),
			}, b.Address())
			checkNoErr(t, m, c)
		}
	})

	// Reset the system to avoid conflicts from previously processed messages.
	config = *istanbul.DefaultConfig
	config.ProposerPolicy = istanbul.RoundRobin
	config.RoundStateDBPath = ""
	config.RequestTimeout = math.MaxUint64 // Avoid round changes
	sys = newTestSystemWithBackendConfigured(N, F, config)

	closer = sys.Run(true)
	defer closer()

	b = sys.backends[0]
	c = b.engine.(*core)

	currentView := &istanbul.View{
		Sequence: c.current.Sequence(),
		Round:    c.current.DesiredRound(),
	}
	t.Run("Current view state AcceptRequest or WaitingForNewRound", func(t *testing.T) {
		for _, testState := range []State{StateAcceptRequest, StateWaitingForNewRound} {
			// Only preprepares and round changes should be accepted.

			c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = testState
			m := istanbul.NewPreprepareMessage(&istanbul.Preprepare{
				View:     currentView,
				Proposal: block1,
			}, b.address)
			err := m.Sign(b.Sign)
			require.NoError(t, err)
			payload, err := m.Payload()
			require.NoError(t, err)
			err = c.handleMsg(payload)
			require.NoError(t, err)
			// Reset state since the previous message changes it
			c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = testState

			m = istanbul.NewPrepareMessage(&istanbul.Subject{
				View:   currentView,
				Digest: block1.Hash(),
			}, b.Address())
			checkErr(t, m, c)

			sub := &istanbul.Subject{
				View:   currentView,
				Digest: block1.Hash(),
			}
			committedSeal, err := c.generateCommittedSeal(sub)
			require.NoError(t, err)
			m = istanbul.NewCommitMessage(&istanbul.CommittedSubject{
				Subject:       sub,
				CommittedSeal: committedSeal[:],
			}, b.Address())
			checkErr(t, m, c)

			m = istanbul.NewRoundChangeMessage(&istanbul.RoundChange{
				View:                currentView,
				PreparedCertificate: istanbul.EmptyPreparedCertificate(),
			}, b.Address())
			checkNoErr(t, m, c)
		}
	})

	t.Run("Current view state Preprepared, Prepared or Committed", func(t *testing.T) {
		for _, testState := range []State{StatePreprepared, StatePrepared, StateCommitted} {
			// All messages should be accepted
			c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = testState
			m := istanbul.NewPreprepareMessage(&istanbul.Preprepare{
				View:     currentView,
				Proposal: block1,
			}, b.address)
			err := m.Sign(b.Sign)
			require.NoError(t, err)
			payload, err := m.Payload()
			require.NoError(t, err)
			err = c.handleMsg(payload)
			require.NoError(t, err)

			m = istanbul.NewPrepareMessage(&istanbul.Subject{
				View:   currentView,
				Digest: block1.Hash(),
			}, b.Address())
			checkNoErr(t, m, c)

			sub := &istanbul.Subject{
				View:   currentView,
				Digest: block1.Hash(),
			}
			committedSeal, err := c.generateCommittedSeal(sub)
			require.NoError(t, err)
			m = istanbul.NewCommitMessage(&istanbul.CommittedSubject{
				Subject:       sub,
				CommittedSeal: committedSeal[:],
			}, b.Address())
			checkNoErr(t, m, c)

			m = istanbul.NewRoundChangeMessage(&istanbul.RoundChange{
				View:                currentView,
				PreparedCertificate: istanbul.EmptyPreparedCertificate(),
			}, b.Address())
			checkNoErr(t, m, c)
		}
	})

	// Reset the system to avoid conflicts from previously processed messages.
	config = *istanbul.DefaultConfig
	config.ProposerPolicy = istanbul.RoundRobin
	config.RoundStateDBPath = ""
	config.RequestTimeout = math.MaxUint64 // Avoid round changes
	sys = newTestSystemWithBackendConfigured(N, F, config)

	closer = sys.Run(true)
	defer closer()

	b = sys.backends[0]
	c = b.engine.(*core)

	c.current.Sequence().SetInt64(1)
	c.current.DesiredRound().SetInt64(0)
	c.current.Round().SetInt64(0)
	c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = StateAcceptRequest

	t.Run("Commit for previous sequence", func(t *testing.T) {

		// fake commit to set the last subject
		b.Commit(block1, types.IstanbulAggregatedSeal{}, types.IstanbulEpochValidatorSetSeal{}, nil)

		// Set the sequence forward
		c.current.Sequence().SetInt64(2)
		c.current.DesiredRound().SetInt64(0)
		c.current.Round().SetInt64(0)
		c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = StatePrepared

		sub := &istanbul.Subject{
			View: &istanbul.View{
				Sequence: big.NewInt(1),
				// testSystemBackend.LastSubject() returns a hardcoded round of 1 for the last subject.
				// so we set Round to 1
				Round: big.NewInt(1),
			},
			Digest: block1.Hash(),
		}
		committedSeal, err := c.generateCommittedSeal(sub)
		require.NoError(t, err)
		m = istanbul.NewCommitMessage(&istanbul.CommittedSubject{
			Subject:       sub,
			CommittedSeal: committedSeal[:],
		}, b.Address())
		checkNoErr(t, m, c)
	})

}

// notice: the normal case have been tested in integration tests.
func TestMalformedMessageDecoding(t *testing.T) {
	N := uint64(4)
	F := uint64(1)
	sys := NewTestSystemWithBackend(N, F)

	closer := sys.Run(true)
	defer closer()

	v0 := sys.backends[0]
	r0 := v0.engine.(*core)

	m := istanbul.NewPrepareMessage(&istanbul.Subject{
		View: &istanbul.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Digest: common.BytesToHash([]byte("1234567890")),
	}, v0.Address())

	// Prepare message but preprepare message code
	m.Code = istanbul.MsgPreprepare

	payload, err := m.Payload()
	require.NoError(t, err)

	msg := &istanbul.Message{}
	err = msg.FromPayload(payload, r0.validateFn)
	assert.Error(t, err)

	m = istanbul.NewPreprepareMessage(&istanbul.Preprepare{
		View: &istanbul.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Proposal: makeBlock(1),
	}, v0.Address())

	// Preprepare message but prepare message code
	m.Code = istanbul.MsgPrepare

	payload, err = m.Payload()
	require.NoError(t, err)

	msg = &istanbul.Message{}
	err = msg.FromPayload(payload, r0.validateFn)
	assert.Error(t, err)

	m = istanbul.NewPreprepareMessage(&istanbul.Preprepare{
		View: &istanbul.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Proposal: makeBlock(2),
	}, v0.Address())

	// Preprepare message but commit message code
	m.Code = istanbul.MsgCommit

	payload, err = m.Payload()
	require.NoError(t, err)

	msg = &istanbul.Message{}
	err = msg.FromPayload(payload, r0.validateFn)
	assert.Error(t, err)

	m = istanbul.NewPreprepareMessage(&istanbul.Preprepare{
		View: &istanbul.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Proposal: makeBlock(3),
	}, v0.Address())

	// invalid message code. message code is not exists in list
	m.Code = uint64(99)

	payload, err = m.Payload()
	require.NoError(t, err)

	msg = &istanbul.Message{}
	err = msg.FromPayload(payload, r0.validateFn)
	assert.Error(t, err)

	// check fails with garbage message
	err = r0.handleMsg([]byte{1})
	assert.Error(t, err)
}

func BenchmarkHandleMsg(b *testing.B) {
	N := uint64(2)
	F := uint64(1) // F does not affect tests

	sys := NewMutedTestSystemWithBackend(N, F)
	// sys := NewTestSystemWithBackend(N, F)

	for i, backend := range sys.backends {
		c := backend.engine.(*core)

		c.current = newTestRoundState(
			&istanbul.View{
				Round:    big.NewInt(0),
				Sequence: big.NewInt(1),
			},
			backend.peers,
		)

		if i == 0 {
			// replica 0 is the proposer
			c.current.(*roundStateImpl).state = StatePreprepared
		}
	}

	sys.Run(false)

	v0 := sys.backends[0]
	v1 := sys.backends[1]
	c := v0.engine.(*core)
	sub := v0.engine.(*core).current.Subject()

	payload, _ := Encode(sub)
	msg := istanbul.Message{
		Code: 1000,
		Msg:  payload,
	}

	msg, _ = v1.finalizeAndReturnMessage(&msg)
	payload, _ = c.finalizeMessage(&msg)

	// benchmarked portion
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := c.handleMsg(payload)
		if err != errInvalidMessage {
			b.Errorf("error mismatch: have %v, want errInvalidMessage", err)
		}
	}
}

// newInitializedTestSystem creates a test system
// It optionally creates a round state db in a temporary directory
func newInitializedTestSystem(b *testing.B, useRoundStateDB bool) *testSystem {
	N := uint64(2)
	F := uint64(1) // F does not affect tests

	sys := NewMutedTestSystemWithBackend(N, F)
	// sys := NewTestSystemWithBackend(N, F)

	for i, backend := range sys.backends {
		c := backend.engine.(*core)

		if useRoundStateDB {
			rsdb, err := newRoundStateDB(b.TempDir(), nil)
			if err != nil {
				b.Errorf("Failed to create rsdb: %v", err)
			}

			c.current = withSavingDecorator(rsdb, newTestRoundState(
				&istanbul.View{
					Round:    big.NewInt(0),
					Sequence: big.NewInt(1),
				},
				backend.peers,
			))

			if i == 0 {
				// replica 0 is the proposer
				c.current.(*rsSaveDecorator).rs.(*roundStateImpl).state = StatePreprepared
			}
		} else {
			c.current = newTestRoundState(
				&istanbul.View{
					Round:    big.NewInt(0),
					Sequence: big.NewInt(1),
				},
				backend.peers,
			)
			if i == 0 {
				// replica 0 is the proposer
				c.current.(*roundStateImpl).state = StatePreprepared
			}
		}
	}

	sys.Run(false)
	return sys
}

func BenchmarkE2EHandleCommit(b *testing.B) {
	sys := newInitializedTestSystem(b, false)
	bemchmarkE2EHandleCommit(b, sys)
}

func BenchmarkE2EHandleCommitWithSave(b *testing.B) {
	sys := newInitializedTestSystem(b, true)
	bemchmarkE2EHandleCommit(b, sys)

}

func bemchmarkE2EHandleCommit(b *testing.B, sys *testSystem) {

	v0 := sys.backends[0]
	v1 := sys.backends[1]
	c := v0.engine.(*core)
	subject := v0.engine.(*core).current.Subject()

	committedSeal, err := v0.engine.(*core).generateCommittedSeal(subject)
	if err != nil {
		b.Errorf("Got error: %v", err)
	}
	committedSubject := &istanbul.CommittedSubject{
		Subject:       subject,
		CommittedSeal: committedSeal[:],
	}
	payload, err := Encode(committedSubject)
	if err != nil {
		b.Errorf("Got error: %v", err)
	}

	msg := istanbul.Message{
		Code: istanbul.MsgCommit,
		Msg:  payload,
	}

	msg, err = v1.finalizeAndReturnMessage(&msg)
	if err != nil {
		b.Errorf("Got error: %v", err)
	}
	payload, _ = c.finalizeMessage(&msg)
	if err != nil {
		b.Errorf("Got error: %v", err)
	}

	// benchmarked portion
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := c.handleMsg(payload)
		if err != nil {
			b.Errorf("error mismatch: have %v, want nil", err)
		}
	}
}

func BenchmarkE2EHandlePrepareWithSave(b *testing.B) {
	sys := newInitializedTestSystem(b, true)
	benchmarkE2EHandlePrepare(b, sys)
}

func BenchmarkE2EHandlePrepare(b *testing.B) {
	sys := newInitializedTestSystem(b, false)
	benchmarkE2EHandlePrepare(b, sys)

}

func benchmarkE2EHandlePrepare(b *testing.B, sys *testSystem) {
	v0 := sys.backends[0]
	v1 := sys.backends[1]
	c := v0.engine.(*core)
	sub := v0.engine.(*core).current.Subject()

	payload, _ := Encode(sub)
	msg := istanbul.Message{
		Code: istanbul.MsgPrepare,
		Msg:  payload,
	}

	msg, _ = v1.finalizeAndReturnMessage(&msg)
	payload, _ = c.finalizeMessage(&msg)

	// benchmarked portion
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := c.handleMsg(payload)
		if err != nil {
			b.Errorf("error mismatch: have %v, want nil", err)
		}
	}
}
