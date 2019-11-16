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
		}, newTestValidatorSet(4), nil, nil, istanbul.EmptyPreparedCertificate(), nil, nil),
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
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
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

	v = c.currentView()
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
		logger:     testLogger,
		backlogs:   make(map[istanbul.Validator]*prque.Prque),
		backlogsMu: new(sync.Mutex),
	}
	v := &istanbul.View{
		Round:    big.NewInt(10),
		Sequence: big.NewInt(10),
	}
	p := validator.New(common.BytesToAddress([]byte("12345667890")), []byte{})
	// push preprepare msg
	preprepare := &istanbul.Preprepare{
		View:     v,
		Proposal: makeBlock(1),
	}
	prepreparePayload, _ := Encode(preprepare)
	m := &istanbul.Message{
		Code:    istanbul.MsgPreprepare,
		Msg:     prepreparePayload,
		Address: p.Address(),
	}
	c.storeBacklog(m, p)
	msg := c.backlogs[p].PopItem()
	if !reflect.DeepEqual(msg, m) {
		t.Errorf("message mismatch: have %v, want %v", msg, m)
	}

	// push prepare msg
	subject := &istanbul.Subject{
		View:   v,
		Digest: common.BytesToHash([]byte("1234567890")),
	}
	subjectPayload, _ := Encode(subject)

	m = &istanbul.Message{
		Code:    istanbul.MsgPrepare,
		Msg:     subjectPayload,
		Address: p.Address(),
	}
	c.storeBacklog(m, p)
	msg = c.backlogs[p].PopItem()
	if !reflect.DeepEqual(msg, m) {
		t.Errorf("message mismatch: have %v, want %v", msg, m)
	}

	// push commit msg
	m = &istanbul.Message{
		Code:    istanbul.MsgCommit,
		Msg:     subjectPayload,
		Address: p.Address(),
	}
	c.storeBacklog(m, p)
	msg = c.backlogs[p].PopItem()
	if !reflect.DeepEqual(msg, m) {
		t.Errorf("message mismatch: have %v, want %v", msg, m)
	}

	// push roundChange msg
	rc := &istanbul.RoundChange{
		View:                v,
		PreparedCertificate: istanbul.EmptyPreparedCertificate(),
	}
	rcPayload, _ := Encode(rc)

	m = &istanbul.Message{
		Code:    istanbul.MsgRoundChange,
		Msg:     rcPayload,
		Address: p.Address(),
	}
	c.storeBacklog(m, p)
	msg = c.backlogs[p].PopItem()
	if !reflect.DeepEqual(msg, m) {
		t.Errorf("message mismatch: have %v, want %v", msg, m)
	}
}

func TestProcessFutureBacklog(t *testing.T) {
	backend := &testSystemBackend{
		events: new(event.TypeMux),
	}
	testLogger.SetHandler(elog.StdoutHandler)
	c := &core{
		logger:     testLogger,
		backlogs:   make(map[istanbul.Validator]*prque.Prque),
		backlogsMu: new(sync.Mutex),
		backend:    backend,
		current: newRoundState(&istanbul.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, newTestValidatorSet(4), nil, nil, istanbul.EmptyPreparedCertificate(), nil, nil),
		state: StateAcceptRequest,
	}
	c.subscribeEvents()
	defer c.unsubscribeEvents()

	v := &istanbul.View{
		Round:    big.NewInt(10),
		Sequence: big.NewInt(10),
	}
	p := validator.New(common.BytesToAddress([]byte("12345667890")), []byte{})
	// push a future msg
	subject := &istanbul.Subject{
		View:   v,
		Digest: common.BytesToHash([]byte("1234567890")),
	}
	subjectPayload, _ := Encode(subject)
	m := &istanbul.Message{
		Code: istanbul.MsgCommit,
		Msg:  subjectPayload,
	}
	c.storeBacklog(m, p)
	c.processBacklog()

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
			Msg:     subjectPayload,
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
		logger:     testLogger,
		backlogs:   make(map[istanbul.Validator]*prque.Prque),
		backlogsMu: new(sync.Mutex),
		backend:    backend,
		state:      State(msg.Code),
		current: newRoundState(&istanbul.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, newTestValidatorSet(4), nil, nil, istanbul.EmptyPreparedCertificate(), nil, nil),
	}
	c.subscribeEvents()
	defer c.unsubscribeEvents()

	c.storeBacklog(msg, vset.GetByIndex(0))
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
