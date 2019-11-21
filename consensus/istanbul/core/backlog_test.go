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
	"math/big"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
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
	c := &core{
		logger:  testLogger,
		state:   StateAcceptRequest,
		backend: backend,
		current: newRoundState(&istanbul.View{
			Sequence: big.NewInt(2),
			Round:    big.NewInt(2),
		}, newTestValidatorSet(4), nil, nil, istanbul.EmptyPreparedCertificate(), nil),
	}

	// invalid view format
	err := c.checkMessage(istanbul.MsgPreprepare, nil)
	if err != errInvalidMessage {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidMessage)
	}

	testStates := []State{StateAcceptRequest, StatePreprepared, StatePrepared, StateCommitted, StateWaitingForNewRound}
	testCode := []uint64{istanbul.MsgPreprepare, istanbul.MsgPrepare, istanbul.MsgCommit, istanbul.MsgRoundChange}

	// accept Commits from sequence, round matching LastSubject
	v := &istanbul.View{
		Sequence: big.NewInt(0),
		// set smaller round so that the roundchange case gets hit
		Round: big.NewInt(1),
	}
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if testCode[j] == istanbul.MsgCommit {
				if err != nil {
					t.Errorf("error mismatch: have %v, want %v", err, nil)
				}
			} else {
				if err != errOldMessage {
					t.Errorf("error mismatch: have %v, want %v", err, errOldMessage)
				}
			}
		}
	}

	// rejects Commits from sequence matching LastSubject, round not matching
	v = &istanbul.View{
		Sequence: big.NewInt(0),
		// set smaller round so that we don't accept
		Round: big.NewInt(0),
	}
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if err != errOldMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errOldMessage)
			}
		}
	}

	// rejects all other older sequences
	v = &istanbul.View{
		Sequence: big.NewInt(0),
		Round:    big.NewInt(0),
	}
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if err != errOldMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errOldMessage)
			}
		}
	}

	// future sequence
	v = &istanbul.View{
		Sequence: big.NewInt(3),
		Round:    big.NewInt(0),
	}
	vTooFuture := &istanbul.View{
		Sequence: big.NewInt(2 + acceptMaxFutureSequence.Int64() + 1),
		Round:    big.NewInt(0),
	}
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
			}
			err = c.checkMessage(testCode[j], vTooFuture)
			if err != errTooFarInTheFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errTooFarInTheFutureMessage)
			}
		}
	}

	// future round
	v = &istanbul.View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(3),
	}
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if testCode[j] == istanbul.MsgRoundChange {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
			}
		}
	}

	v = c.current.View()
	// current view, state = StateAcceptRequest
	c.state = StateAcceptRequest
	for i := 0; i < len(testCode); i++ {
		err = c.checkMessage(testCode[i], v)
		if testCode[i] == istanbul.MsgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if testCode[i] == istanbul.MsgPreprepare {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else {
			if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
			}
		}
	}

	// current view, state = StatePreprepared
	c.state = StatePreprepared
	for i := 0; i < len(testCode); i++ {
		err = c.checkMessage(testCode[i], v)
		if testCode[i] == istanbul.MsgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
	}

	// current view, state = StatePrepared
	c.state = StatePrepared
	for i := 0; i < len(testCode); i++ {
		err = c.checkMessage(testCode[i], v)
		if testCode[i] == istanbul.MsgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
	}

	// current view, state = StateCommitted
	c.state = StateCommitted
	for i := 0; i < len(testCode); i++ {
		err = c.checkMessage(testCode[i], v)
		if testCode[i] == istanbul.MsgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
	}

	// current view, state = StateWaitingForNewRound
	c.state = StateWaitingForNewRound
	for i := 0; i < len(testCode); i++ {
		err := c.checkMessage(testCode[i], v)
		if testCode[i] == istanbul.MsgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if err != errFutureMessage {
			t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
		}
	}

}

func TestStoreBacklog(t *testing.T) {
	testLogger.SetHandler(elog.StdoutHandler)
	c := &core{
		logger:            testLogger,
		backlogBySeq:      make(map[uint64]*prque.Prque),
		backlogCountByVal: make(map[common.Address]int),
		backlogsMu:        new(sync.Mutex),
	}

	v10 := &istanbul.View{
		Round:    big.NewInt(10),
		Sequence: big.NewInt(10),
	}

	v11 := &istanbul.View{
		Round:    big.NewInt(12),
		Sequence: big.NewInt(11),
	}
	p1 := validator.New(common.BytesToAddress([]byte("12345667890")), []byte{})
	p2 := validator.New(common.BytesToAddress([]byte("47324349949")), []byte{})

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
	c.storeBacklog(mPreprepare)
	msg := c.backlogBySeq[v10.Sequence.Uint64()].PopItem()
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

	c.storeBacklog(mPreprepare)
	c.storeBacklog(mPrepare)
	c.storeBacklog(mPreprepare2)

	if c.backlogCountByVal[p1.Address()] != 3 {
		t.Errorf("backlogCountByVal mismatch: have %v, want 3", c.backlogCountByVal[p1.Address()])
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
	c.storeBacklog(mCommit)
	if c.backlogCountByVal[p2.Address()] != 1 {
		t.Errorf("backlogCountByVal mismatch: have %v, want 1", c.backlogCountByVal[p2.Address()])
	}
	if c.backlogTotal != 5 {
		t.Errorf("backlogTotal mismatch: have %v, want 5", c.backlogTotal)
	}

	// Should get back v10 preprepare then commit
	msg = c.backlogBySeq[v10.Sequence.Uint64()].PopItem()
	if !reflect.DeepEqual(msg, mPreprepare) {
		t.Errorf("message mismatch: have %v, want %v", msg, mPreprepare2)
	}
	msg = c.backlogBySeq[v10.Sequence.Uint64()].PopItem()
	if !reflect.DeepEqual(msg, mCommit) {
		t.Errorf("message mismatch: have %v, want %v", msg, mCommit)

	}
	msg = c.backlogBySeq[v11.Sequence.Uint64()].PopItem()
	if !reflect.DeepEqual(msg, mPreprepare2) {
		t.Errorf("message mismatch: have %v, want %v", msg, mPreprepare2)
	}

	c.backlogTotal = 0
	delete(c.backlogCountByVal, p1.Address())
	delete(c.backlogCountByVal, p2.Address())
}

func TestProcessFutureBacklog(t *testing.T) {
	backend := &testSystemBackend{
		events: new(event.TypeMux),
	}

	valSet := newTestValidatorSet(4)
	testLogger.SetHandler(elog.StdoutHandler)
	c := &core{
		logger:            testLogger,
		backlogBySeq:      make(map[uint64]*prque.Prque),
		backlogCountByVal: make(map[common.Address]int),
		backlogsMu:        new(sync.Mutex),
		backend:           backend,
		current: newRoundState(&istanbul.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, valSet, nil, nil, istanbul.EmptyPreparedCertificate(), nil),
		state: StateAcceptRequest,
	}
	c.subscribeEvents()
	defer c.unsubscribeEvents()

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
	mFuture := &istanbul.Message{
		Code:    istanbul.MsgCommit,
		Msg:     committedSubjectPayload,
		Address: valSet.GetByIndex(0).Address(),
	}
	c.storeBacklog(mFuture)

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
	c.storeBacklog(mPast)

	backlogSeqs := c.getSortedBacklogSeqs()
	if len(backlogSeqs) != 1 || backlogSeqs[0] != v.Sequence.Uint64() {
		t.Errorf("getSortedBacklogSeqs mismatch: have %v", backlogSeqs)
	}

	c.processBacklog()

	// Check message from future remains, past expired
	if c.backlogTotal != 1 || c.backlogCountByVal[valSet.GetByIndex(1).Address()] > 0 {
		t.Errorf("backlog mismatch: %v", c.backlogCountByVal)
	}

	const timeoutDura = 2 * time.Second
	timeout := time.NewTimer(timeoutDura)
	select {
	case e, ok := <-c.events.Chan():
		if !ok {
			return
		}
		t.Errorf("unexpected events comes: %v", e)
	case <-timeout.C:
		// success
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
		testProcessBacklog(t, msgs[i])
	}
}

func testProcessBacklog(t *testing.T, msg *istanbul.Message) {
	vset := newTestValidatorSet(1)
	backend := &testSystemBackend{
		events: new(event.TypeMux),
		peers:  vset,
	}
	testLogger.SetHandler(elog.StdoutHandler)
	c := &core{
		logger:            testLogger.New("backend", "test", "id", 0),
		backlogBySeq:      make(map[uint64]*prque.Prque),
		backlogCountByVal: make(map[common.Address]int),
		backlogsMu:        new(sync.Mutex),
		backend:           backend,
		state:             State(msg.Code),
		current: newRoundState(&istanbul.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, newTestValidatorSet(4), nil, nil, istanbul.EmptyPreparedCertificate(), nil),
		valSet: vset,
	}
	c.subscribeEvents()
	defer c.unsubscribeEvents()

	msg.Address = vset.GetByIndex(0).Address()
	c.storeBacklog(msg)
	c.processBacklog()

	const timeoutDura = 2 * time.Second
	timeout := time.NewTimer(timeoutDura)
	select {
	case ev := <-c.events.Chan():
		e, ok := ev.Data.(backlogEvent)
		if !ok {
			t.Errorf("unexpected event comes: %v", reflect.TypeOf(ev.Data))
		}
		if e.msg.Code != msg.Code {
			t.Errorf("message code mismatch: have %v, want %v", e.msg.Code, msg.Code)
		}
		// success
	case <-timeout.C:
		t.Error("unexpected timeout occurs")
	}
}
