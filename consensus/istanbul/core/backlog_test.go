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
	"fmt"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/event"
	elog "github.com/celo-org/celo-blockchain/log"
)

func TestCheckMessage(t *testing.T) {
	testLogger.SetHandler(elog.StdoutHandler)
	backend := &testSystemBackend{
		events: new(event.TypeMux),
	}
	valSet := newTestValidatorSet(4)
	c := &core{
		logger:  testLogger,
		backend: backend,
		current: newRoundState(&istanbul.View{
			Sequence: big.NewInt(2),
			Round:    big.NewInt(2),
		}, valSet, valSet.GetByIndex(0)),
	}

	t.Run("invalid view format", func(t *testing.T) {
		err := c.checkMessage(istanbul.MsgPreprepare, nil)
		if err != errInvalidMessage {
			t.Errorf("error mismatch: have %v, want %v", err, errInvalidMessage)
		}
	})

	testStates := []State{StateAcceptRequest, StatePreprepared, StatePrepared, StateCommitted, StateWaitingForNewRound}
	testCodes := []uint64{istanbul.MsgPreprepare, istanbul.MsgPrepare, istanbul.MsgCommit, istanbul.MsgRoundChange}

	// accept Commits from sequence, round matching LastSubject
	t.Run("Rejects all other older rounds", func(t *testing.T) {
		v := &istanbul.View{
			Sequence: big.NewInt(2),
			Round:    big.NewInt(1),
		}
		for _, testState := range testStates {
			for _, testCode := range testCodes {
				c.current.(*roundStateImpl).state = testState
				err := c.checkMessage(testCode, v)

				if err != errOldMessage {
					t.Errorf("error mismatch: have %v, want %v", err, errOldMessage)
				}

			}
		}
	})

	t.Run("Rejects all other older sequences", func(t *testing.T) {
		v := &istanbul.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		}
		for _, testState := range testStates {
			for _, testCode := range testCodes {
				c.current.(*roundStateImpl).state = testState
				err := c.checkMessage(testCode, v)
				if err != errOldMessage {
					t.Errorf("error mismatch: have %v, want %v", err, errOldMessage)
				}
			}
		}
	})

	t.Run("Future sequence", func(t *testing.T) {
		v := &istanbul.View{
			Sequence: big.NewInt(3),
			Round:    big.NewInt(0),
		}
		for _, testState := range testStates {
			for _, testCode := range testCodes {
				c.current.(*roundStateImpl).state = testState
				err := c.checkMessage(testCode, v)
				if err != errFutureMessage {
					t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
				}
			}
		}
	})

	t.Run("future round", func(t *testing.T) {
		v := &istanbul.View{
			Sequence: big.NewInt(2),
			Round:    big.NewInt(3),
		}
		for _, testState := range testStates {
			for _, testCode := range testCodes {
				c.current.(*roundStateImpl).state = testState
				err := c.checkMessage(testCode, v)
				if testCode == istanbul.MsgRoundChange {
					if err != nil {
						t.Errorf("error mismatch: have %v, want nil", err)
					}
				} else if err != errFutureMessage {
					t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
				}
			}
		}
	})

	t.Run("current view, state = StateAcceptRequest", func(t *testing.T) {
		v := c.current.View()
		c.current.(*roundStateImpl).state = StateAcceptRequest

		for _, testCode := range testCodes {
			err := c.checkMessage(testCode, v)
			if testCode == istanbul.MsgRoundChange {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else if testCode == istanbul.MsgPreprepare {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else {
				if err != errFutureMessage {
					t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
				}
			}
		}
	})

	t.Run("current view, state = StatePreprepared", func(t *testing.T) {
		v := c.current.View()
		c.current.(*roundStateImpl).state = StatePreprepared
		for _, testCode := range testCodes {
			err := c.checkMessage(testCode, v)
			if testCode == istanbul.MsgRoundChange {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		}
	})

	t.Run("current view, state = StatePrepared", func(t *testing.T) {
		v := c.current.View()
		c.current.(*roundStateImpl).state = StatePrepared
		for _, testCode := range testCodes {
			err := c.checkMessage(testCode, v)
			if testCode == istanbul.MsgRoundChange {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		}
	})

	t.Run("current view, state = StateCommited", func(t *testing.T) {
		v := c.current.View()
		c.current.(*roundStateImpl).state = StateCommitted
		for _, testCode := range testCodes {
			err := c.checkMessage(testCode, v)
			if testCode == istanbul.MsgRoundChange {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		}
	})

	t.Run("current view, state = StateWaitingForNewRound", func(t *testing.T) {
		v := c.current.View()
		c.current.(*roundStateImpl).state = StateWaitingForNewRound
		for _, testCode := range testCodes {
			err := c.checkMessage(testCode, v)
			if testCode == istanbul.MsgRoundChange || testCode == istanbul.MsgPreprepare {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
			}
		}
	})

}

func TestStoreBacklog(t *testing.T) {
	testLogger.SetHandler(elog.StdoutHandler)
	backlog := newMsgBacklog(nil).(*msgBacklogImpl)
	defer backlog.clearBacklogForSeq(12)

	v10 := &istanbul.View{
		Round:    big.NewInt(10),
		Sequence: big.NewInt(10),
	}

	v11 := &istanbul.View{
		Round:    big.NewInt(12),
		Sequence: big.NewInt(11),
	}
	p1 := validator.New(common.BytesToAddress([]byte("12345667890")), blscrypto.SerializedPublicKey{})
	p2 := validator.New(common.BytesToAddress([]byte("47324349949")), blscrypto.SerializedPublicKey{})

	mPreprepare := istanbul.NewPreprepareMessage(
		&istanbul.Preprepare{View: v10, Proposal: makeBlock(10)},
		p1.Address(),
	)
	backlog.store(mPreprepare)

	msg := backlog.backlogBySeq[v10.Sequence.Uint64()].PopItem()
	if !reflect.DeepEqual(msg, mPreprepare) {
		t.Errorf("message mismatch: have %v, want %v", msg, mPreprepare)
	}

	mPrepare := istanbul.NewPrepareMessage(
		&istanbul.Subject{View: v10, Digest: common.BytesToHash([]byte("1234567890"))},
		p1.Address(),
	)
	mPreprepare2 := istanbul.NewPreprepareMessage(
		&istanbul.Preprepare{View: v11, Proposal: makeBlock(11)},
		p2.Address(),
	)

	backlog.store(mPreprepare)
	backlog.store(mPrepare)
	backlog.store(mPreprepare2)

	if backlog.msgCountBySrc[p1.Address()] != 3 {
		t.Errorf("msgCountBySrc mismatch: have %v, want 3", backlog.msgCountBySrc[p1.Address()])
	}

	mCommit := istanbul.NewCommitMessage(
		&istanbul.CommittedSubject{Subject: mPrepare.Prepare(), CommittedSeal: []byte{0x63, 0x65, 0x6C, 0x6F}}, // celo in hex!
		p1.Address(),
	)

	backlog.store(mCommit)
	if backlog.msgCountBySrc[p2.Address()] != 1 {
		t.Errorf("msgCountBySrc mismatch: have %v, want 1", backlog.msgCountBySrc[p2.Address()])
	}
	if backlog.msgCount != 5 {
		t.Errorf("msgCount mismatch: have %v, want 5", backlog.msgCount)
	}

	// Should get back v10 preprepare then commit
	msg = backlog.backlogBySeq[v10.Sequence.Uint64()].PopItem()
	if !reflect.DeepEqual(msg, mPreprepare) {
		t.Errorf("message mismatch: have %v, want %v", msg, mPreprepare2)
	}
	msg = backlog.backlogBySeq[v10.Sequence.Uint64()].PopItem()
	if !reflect.DeepEqual(msg, mCommit) {
		t.Errorf("message mismatch: have %v, want %v", msg, mCommit)

	}
	msg = backlog.backlogBySeq[v11.Sequence.Uint64()].PopItem()
	if !reflect.DeepEqual(msg, mPreprepare2) {
		t.Errorf("message mismatch: have %v, want %v", msg, mPreprepare2)
	}
}

func TestClearBacklogForSequence(t *testing.T) {
	testLogger.SetHandler(elog.StdoutHandler)

	processed := false
	backlog := newMsgBacklog(nil).(*msgBacklogImpl)

	// The backlog's state is sequence number 1, round 0.  Store future messages with sequence number 2
	p1 := validator.New(common.BytesToAddress([]byte("12345667890")), blscrypto.SerializedPublicKey{})

	mPreprepare := istanbul.NewPreprepareMessage(
		&istanbul.Preprepare{
			View:     &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(2)},
			Proposal: makeBlock(2),
		},
		p1.Address(),
	)

	numMsgs := 20
	for i := 0; i < numMsgs; i++ {
		backlog.store(mPreprepare)
	}

	// Sanity check that storing the messages worked
	if backlog.msgCount != numMsgs {
		t.Errorf("initial message count mismatch: have %d, want %d", backlog.msgCount, numMsgs)
	}
	// Try clearing a different sequence number, there should be no effect
	backlog.clearBacklogForSeq(3)
	if backlog.msgCount != numMsgs {
		t.Errorf("middle message count mismatch: have %d, want %d", backlog.msgCount, numMsgs)
	}
	// Clear the messages with the right sequence number, should empty the backlog
	backlog.clearBacklogForSeq(2)
	if backlog.msgCount > 0 {
		t.Errorf("backlog was not empty: msgCount %d", backlog.msgCount)
	}
	// The processor should not be called with the messages when clearBacklogForSeq() is called
	if processed {
		t.Errorf("backlog messages were processed during clearing")
	}
}

func TestProcessFutureBacklog(t *testing.T) {
	testLogger.SetHandler(elog.StdoutHandler)

	backlog := newMsgBacklog(nil).(*msgBacklogImpl)
	defer backlog.clearBacklogForSeq(12)

	futureSequence := big.NewInt(10)
	oldSequence := big.NewInt(0)

	// push a future msg
	valSet := newTestValidatorSet(4)
	mFuture := istanbul.NewCommitMessage(
		&istanbul.CommittedSubject{
			Subject: &istanbul.Subject{
				View:   &istanbul.View{Round: big.NewInt(10), Sequence: futureSequence},
				Digest: common.BytesToHash([]byte("1234567890")),
			},
			CommittedSeal: []byte{0x63, 0x65, 0x6C, 0x6F},
		},
		valSet.GetByIndex(0).Address())

	backlog.store(mFuture)

	// push a message from the past and check we expire it
	mPast := istanbul.NewRoundChangeMessage(&istanbul.RoundChange{
		View: &istanbul.View{
			Round:    big.NewInt(0),
			Sequence: oldSequence,
		},
		PreparedCertificate: istanbul.PreparedCertificate{
			Proposal: makeBlock(0),
		},
	}, valSet.GetByIndex(1).Address())

	backlog.store(mPast)

	// Should prune old messages
	backlog.processBacklog()

	backlogSeqs := backlog.getSortedBacklogSeqs()
	if len(backlogSeqs) != 1 || backlogSeqs[0] != futureSequence.Uint64() {
		t.Errorf("getSortedBacklogSeqs mismatch: have %v", backlogSeqs)
	}

	backlog.updateState(&istanbul.View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(0),
	}, StateAcceptRequest)

	// Check message from future remains, past expired
	if backlog.msgCount != 1 || backlog.msgCountBySrc[valSet.GetByIndex(1).Address()] > 0 {
		t.Errorf("backlog mismatch: %v", backlog.msgCountBySrc)
	}
}

func TestProcessBacklog(t *testing.T) {
	v := &istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(1),
	}

	subject := &istanbul.Subject{
		View:   v,
		Digest: common.BytesToHash([]byte("1234567890")),
	}
	address := common.BytesToAddress([]byte("0xce10ce10"))

	msgs := []*istanbul.Message{
		istanbul.NewPreprepareMessage(
			&istanbul.Preprepare{View: v, Proposal: makeBlock(1)},
			address,
		),
		istanbul.NewPrepareMessage(subject, address),
		istanbul.NewCommitMessage(
			&istanbul.CommittedSubject{Subject: subject, CommittedSeal: []byte{0x63, 0x65, 0x6C, 0x6F}},
			address,
		),
		istanbul.NewRoundChangeMessage(
			&istanbul.RoundChange{View: v, PreparedCertificate: istanbul.EmptyPreparedCertificate()},
			address,
		),
	}
	for i := 0; i < len(msgs); i++ {
		t.Run(fmt.Sprintf("Msg with code %d", msgs[i].Code), func(t *testing.T) {
			testProcessBacklog(t, msgs[i])
		})
	}
}

func testProcessBacklog(t *testing.T, msg *istanbul.Message) {
	vset := newTestValidatorSet(1)
	backend := &testSystemBackend{
		events: new(event.TypeMux),
		peers:  vset,
	}
	testLogger.SetHandler(elog.StdoutHandler)
	valSet := newTestValidatorSet(4)
	c := &core{
		logger:  testLogger,
		backend: backend,
		current: newRoundState(&istanbul.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, valSet, valSet.GetByIndex(0)),
	}
	backlog := newMsgBacklog(c).(*msgBacklogImpl)
	c.current.(*roundStateImpl).state = State(msg.Code)
	c.subscribeEvents()
	defer c.unsubscribeEvents()

	v := &istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(1),
	}

	msg.Address = common.Address{50}
	backlog.store(msg)

	backlog.updateState(v, State(msg.Code))

	timeout := time.NewTimer(1 * time.Second)

	select {
	case got := <-c.events.Chan():
		if got.Data.(backlogEvent).msg != msg {
			// if got != msg.Code {
			t.Errorf("Expected different msg: have: %v, want: %v", got, msg.Code)
		}
	case <-timeout.C:
		t.Errorf("No Message was processed")
	}

}
