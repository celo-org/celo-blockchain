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
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/event"
	elog "github.com/ethereum/go-ethereum/log"
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
			if testCode == istanbul.MsgRoundChange {
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
	backlog := newMsgBacklog(
		func(msg *istanbul.Message) {},
		func(msgCode uint64, msgView *istanbul.View) error { return nil },
	).(*msgBacklogImpl)

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

	// push messages
	preprepare := &istanbul.Preprepare{
		View:     v10,
		Proposal: makeBlock(10),
	}

	prepreparePayload, _ := Encode(preprepare)
	mPreprepare := &istanbul.Message{
		Code:    istanbul.MsgPreprepare,
		Msg:     prepreparePayload,
		Address: p1.Address(),
	}

	backlog.store(mPreprepare)

	msg := backlog.backlogBySeq[v10.Sequence.Uint64()].PopItem()
	if !reflect.DeepEqual(msg, mPreprepare) {
		t.Errorf("message mismatch: have %v, want %v", msg, mPreprepare)
	}

	subject := &istanbul.Subject{
		View:   v10,
		Digest: common.BytesToHash([]byte("1234567890")),
	}
	subjectPayload, _ := Encode(subject)
	mPrepare := &istanbul.Message{
		Code:    istanbul.MsgPrepare,
		Msg:     subjectPayload,
		Address: p1.Address(),
	}

	preprepare2 := &istanbul.Preprepare{
		View:     v11,
		Proposal: makeBlock(11),
	}
	preprepare2Payload, _ := Encode(preprepare2)
	mPreprepare2 := &istanbul.Message{
		Code:    istanbul.MsgPreprepare,
		Msg:     preprepare2Payload,
		Address: p2.Address(),
	}

	backlog.store(mPreprepare)
	backlog.store(mPrepare)
	backlog.store(mPreprepare2)

	if backlog.msgCountBySrc[p1.Address()] != 3 {
		t.Errorf("msgCountBySrc mismatch: have %v, want 3", backlog.msgCountBySrc[p1.Address()])
	}
	// push commit msg
	committedSubject := &istanbul.CommittedSubject{
		Subject:       subject,
		CommittedSeal: []byte{0x63, 0x65, 0x6C, 0x6F}, // celo in hex!
	}

	committedSubjectPayload, _ := Encode(committedSubject)

	mCommit := &istanbul.Message{
		Code:    istanbul.MsgCommit,
		Msg:     committedSubjectPayload,
		Address: p1.Address(),
	}

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

	backlog.msgCount = 0
	delete(backlog.msgCountBySrc, p1.Address())
	delete(backlog.msgCountBySrc, p2.Address())
}

func TestProcessFutureBacklog(t *testing.T) {
	testLogger.SetHandler(elog.StdoutHandler)

	backlog := newMsgBacklog(
		func(msg *istanbul.Message) {},
		func(msgCode uint64, msgView *istanbul.View) error { return nil },
	).(*msgBacklogImpl)

	// push a future msg
	v := &istanbul.View{
		Round:    big.NewInt(10),
		Sequence: big.NewInt(10),
	}

	committedSubject := &istanbul.CommittedSubject{
		Subject: &istanbul.Subject{
			View:   v,
			Digest: common.BytesToHash([]byte("1234567890")),
		},
		CommittedSeal: []byte{0x63, 0x65, 0x6C, 0x6F},
	}

	committedSubjectPayload, _ := Encode(committedSubject)
	// push a future msg
	valSet := newTestValidatorSet(4)
	mFuture := &istanbul.Message{
		Code:    istanbul.MsgCommit,
		Msg:     committedSubjectPayload,
		Address: valSet.GetByIndex(0).Address(),
	}
	backlog.store(mFuture)

	// push a message from the past and check we expire it
	v0 := &istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(0),
	}
	subject0 := &istanbul.Subject{
		View:   v0,
		Digest: common.BytesToHash([]byte("1234567890")),
	}
	subjectPayload0, _ := Encode(subject0)
	mPast := &istanbul.Message{
		Code:    istanbul.MsgRoundChange,
		Msg:     subjectPayload0,
		Address: valSet.GetByIndex(1).Address(),
	}
	backlog.store(mPast)

	backlogSeqs := backlog.getSortedBacklogSeqs()
	if len(backlogSeqs) != 1 || backlogSeqs[0] != v.Sequence.Uint64() {
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
	preprepare := &istanbul.Preprepare{
		View:     v,
		Proposal: makeBlock(1),
	}
	prepreparePayload, _ := Encode(preprepare)

	subject := &istanbul.Subject{
		View:   v,
		Digest: common.BytesToHash([]byte("1234567890")),
	}
	subjectPayload, _ := Encode(subject)

	committedSubject := &istanbul.CommittedSubject{
		Subject:       subject,
		CommittedSeal: []byte{0x63, 0x65, 0x6C, 0x6F},
	}
	committedSubjectPayload, _ := Encode(committedSubject)

	rc := &istanbul.RoundChange{
		View:                v,
		PreparedCertificate: istanbul.EmptyPreparedCertificate(),
	}
	rcPayload, _ := Encode(rc)

	address := common.BytesToAddress([]byte("0xce10ce10"))

	msgs := []*istanbul.Message{
		{
			Code:    istanbul.MsgPreprepare,
			Msg:     prepreparePayload,
			Address: address,
		},
		{
			Code:    istanbul.MsgPrepare,
			Msg:     subjectPayload,
			Address: address,
		},
		{
			Code:    istanbul.MsgCommit,
			Msg:     committedSubjectPayload,
			Address: address,
		},
		{
			Code:    istanbul.MsgRoundChange,
			Msg:     rcPayload,
			Address: address,
		},
	}
	for i := 0; i < len(msgs); i++ {
		t.Run(fmt.Sprintf("Msg with code %d", msgs[i].Code), func(t *testing.T) {
			testProcessBacklog(t, msgs[i])
		})
	}
}

func testProcessBacklog(t *testing.T, msg *istanbul.Message) {

	testLogger.SetHandler(elog.StdoutHandler)

	processedMsgs := make(chan uint64, 100)
	registerCall := func(msg *istanbul.Message) {
		processedMsgs <- msg.Code
		// we expect only one msg
		close(processedMsgs)
	}

	backlog := newMsgBacklog(
		registerCall,
		func(msgCode uint64, msgView *istanbul.View) error { return nil },
	).(*msgBacklogImpl)

	v := &istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(1),
	}

	msg.Address = common.Address{50}
	backlog.store(msg)

	backlog.updateState(v, State(msg.Code))

	timeout := time.NewTimer(1 * time.Second)

	select {
	case got := <-processedMsgs:
		if got != msg.Code {
			t.Errorf("Expected different msg: have: %v, want: %v", got, msg.Code)
		}
	case <-timeout.C:
		t.Errorf("No Message was processed")
	}

}
