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
	"github.com/ethereum/go-ethereum/log"
)

func TestCheckMessage(t *testing.T) {
	c := &core{
		state: StateAcceptRequest,
		current: newRoundState(&istanbul.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, newTestValidatorSet(4), common.Hash{}, nil, nil, nil),
	}

	// invalid view format
	err := c.checkMessage(msgPreprepare, nil)
	if err != errInvalidMessage {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidMessage)
	}

	testStates := []State{StateAcceptRequest, StatePreprepared, StatePrepared, StateCommitted}
	testCode := []uint64{msgPreprepare, msgPrepare, msgCommit, msgRoundChange}

	// future sequence
	v := &istanbul.View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(0),
	}
	vTooFuture := &istanbul.View{
		Sequence: big.NewInt(2 + acceptMaxFutureViews.Int64()),
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
		Sequence: big.NewInt(1),
		Round:    big.NewInt(1),
	}
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if testCode[j] == msgRoundChange {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
			}
		}
	}

	// current view but waiting for round change
	v = &istanbul.View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(0),
	}
	c.waitingForRoundChange = true
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(testCode[j], v)
			if testCode[j] == msgRoundChange {
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
			} else if err != errFutureMessage {
				t.Errorf("error mismatch: have %v, want %v", err, errFutureMessage)
			}
		}
	}
	c.waitingForRoundChange = false

	v = c.currentView()
	// current view, state = StateAcceptRequest
	c.state = StateAcceptRequest
	for i := 0; i < len(testCode); i++ {
		err = c.checkMessage(testCode[i], v)
		if testCode[i] == msgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if testCode[i] == msgPreprepare {
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
		if testCode[i] == msgRoundChange {
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
		if testCode[i] == msgRoundChange {
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
		if testCode[i] == msgRoundChange {
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
		} else if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
	}

}

func TestStoreBacklog(t *testing.T) {
	c := &core{
		logger:            log.New("backend", "test", "id", 0),
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
	p1 := validator.New(common.BytesToAddress([]byte("12345667890")))
	p2 := validator.New(common.BytesToAddress([]byte("47324349949")))

	// push messages

	preprepare := &istanbul.Preprepare{
		View:     v10,
		Proposal: makeBlock(1),
	}
	sPreprepare, _ := Encode(preprepare)
	mPreprepare := &message{
		Code:    msgPreprepare,
		Msg:     sPreprepare,
		Address: p1.Address(),
	}

	subject := &istanbul.Subject{
		View:   v10,
		Digest: common.BytesToHash([]byte("1234567890")),
	}
	sPrepare, _ := Encode(subject)
	mPrepare := &message{
		Code:    msgPrepare,
		Msg:     sPrepare,
		Address: p1.Address(),
	}

	// push messages
	preprepare2 := &istanbul.Preprepare{
		View:     v11,
		Proposal: makeBlock(1),
	}
	sPreprepare2, _ := Encode(preprepare2)
	mPreprepare2 := &message{
		Code:    msgPreprepare,
		Msg:     sPreprepare2,
		Address: p2.Address(),
	}

	c.storeBacklog(mPreprepare, p1)
	c.storeBacklog(mPrepare, p1)
	c.storeBacklog(mPreprepare2, p2)

	if c.backlogCountByVal[p1.Address()] != 2 {
		t.Errorf("backlogCountByVal mismatch: have %v, want 2", c.backlogCountByVal[p1.Address()])
	}
	if c.backlogCountByVal[p2.Address()] != 1 {
		t.Errorf("backlogCountByVal mismatch: have %v, want 1", c.backlogCountByVal[p2.Address()])
	}
	if c.backlogTotal != 3 {
		t.Errorf("backlogTotal mismatch: have %v, want 3", c.backlogTotal)
	}

	// Should get back v10 preprepare then prepare
	msg := c.backlogBySeq[v10.Sequence.Uint64()].PopItem()
	if !reflect.DeepEqual(msg, mPreprepare) {
		t.Errorf("message mismatch: have %v, want %v", msg, mPreprepare2)
	}
	msg = c.backlogBySeq[v10.Sequence.Uint64()].PopItem()
	if !reflect.DeepEqual(msg, mPrepare) {
		t.Errorf("message mismatch: have %v, want %v", msg, mPrepare)
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

	c := &core{
		logger:            log.New("backend", "test", "id", 0),
		backlogBySeq:      make(map[uint64]*prque.Prque),
		backlogCountByVal: make(map[common.Address]int),
		backlogsMu:        new(sync.Mutex),
		backend:           backend,
		current: newRoundState(&istanbul.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, valSet, common.Hash{}, nil, nil, nil),
		state:  StateAcceptRequest,
		valSet: valSet,
	}
	c.subscribeEvents()
	defer c.unsubscribeEvents()

	// push a future msg
	v := &istanbul.View{
		Round:    big.NewInt(10),
		Sequence: big.NewInt(10),
	}
	subject := &istanbul.Subject{
		View:   v,
		Digest: common.BytesToHash([]byte("1234567890")),
	}
	subjectPayload, _ := Encode(subject)
	mFuture := &message{
		Code:    msgCommit,
		Msg:     subjectPayload,
		Address: valSet.GetByIndex(0).Address(),
	}
	c.storeBacklog(mFuture, valSet.GetByIndex(0))

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
	mPast := &message{
		Code:    msgRoundChange,
		Msg:     subjectPayload0,
		Address: valSet.GetByIndex(1).Address(),
	}
	c.storeBacklog(mPast, valSet.GetByIndex(1))

	backlogSeqs := c.getSortedBacklogSeqs()
	if len(backlogSeqs) != 2 || backlogSeqs[0] != v0.Sequence.Uint64() || backlogSeqs[1] != v.Sequence.Uint64() {
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

	msgs := []*message{
		{
			Code: msgPreprepare,
			Msg:  prepreparePayload,
		},
		{
			Code: msgPrepare,
			Msg:  subjectPayload,
		},
		{
			Code: msgCommit,
			Msg:  subjectPayload,
		},
		{
			Code: msgRoundChange,
			Msg:  subjectPayload,
		},
	}
	for i := 0; i < len(msgs); i++ {
		testProcessBacklog(t, msgs[i])
	}
}

func testProcessBacklog(t *testing.T, msg *message) {
	vset := newTestValidatorSet(1)
	backend := &testSystemBackend{
		events: new(event.TypeMux),
		peers:  vset,
	}
	c := &core{
		logger:            log.New("backend", "test", "id", 0),
		backlogBySeq:      make(map[uint64]*prque.Prque),
		backlogCountByVal: make(map[common.Address]int),
		backlogsMu:        new(sync.Mutex),
		backend:           backend,
		state:             State(msg.Code),
		current: newRoundState(&istanbul.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, newTestValidatorSet(4), common.Hash{}, nil, nil, nil),
		valSet: vset,
	}
	c.subscribeEvents()
	defer c.unsubscribeEvents()

	msg.Address = vset.GetByIndex(0).Address()
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
