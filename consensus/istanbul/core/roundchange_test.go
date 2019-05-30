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
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
)

func TestRoundChangeSet(t *testing.T) {
	vset := validator.NewSet(generateValidators(4), istanbul.RoundRobin)
	rc := newRoundChangeSet(vset)

	view := &istanbul.View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(1),
	}
	r := &istanbul.Subject{
		View:   view,
		Digest: common.Hash{},
	}
	m, _ := Encode(r)

	// Test Add()
	// Add message from all validators
	for i, v := range vset.List() {
		msg := &message{
			Code:    msgRoundChange,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view.Round, msg, v)
		if rc.msgsForRound[view.Round.Uint64()].Size() != i+1 {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.msgsForRound[view.Round.Uint64()].Size(), i+1)
		}
	}

	// Add message again from all validators, but the size should be the same
	for _, v := range vset.List() {
		msg := &message{
			Code:    msgRoundChange,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view.Round, msg, v)
		if rc.msgsForRound[view.Round.Uint64()].Size() != vset.Size() {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.msgsForRound[view.Round.Uint64()].Size(), vset.Size())
		}
	}

	// Test MaxRound()
	for i := 0; i < 10; i++ {
		maxRound := rc.MaxRound(i)
		if i <= vset.Size() {
			if maxRound == nil || maxRound.Cmp(view.Round) != 0 {
				t.Errorf("MaxRound mismatch: have %v, want %v", maxRound, view.Round)
			}
		} else if maxRound != nil {
			t.Errorf("MaxRound mismatch: have %v, want nil", maxRound)
		}
	}

	// Test Clear()
	for i := int64(0); i < 2; i++ {
		rc.Clear(big.NewInt(i))
		if rc.msgsForRound[view.Round.Uint64()].Size() != vset.Size() {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.msgsForRound[view.Round.Uint64()].Size(), vset.Size())
		}
	}
	rc.Clear(big.NewInt(2))
	if rc.msgsForRound[view.Round.Uint64()] != nil {
		t.Errorf("the change messages mismatch: have %v, want nil", rc.msgsForRound[view.Round.Uint64()])
	}

	// Test Add()
	// Add message from all validators
	for i, v := range vset.List() {
		msg := &message{
			Code:    msgRoundChange,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view.Round, msg, v)
		if rc.msgsForRound[view.Round.Uint64()].Size() != i+1 {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.msgsForRound[view.Round.Uint64()].Size(), i+1)
		}
	}

	rc.Clear(big.NewInt(2))
	if rc.msgsForRound[view.Round.Uint64()] != nil {
		t.Errorf("the change messages mismatch: have %v, want nil", rc.msgsForRound[view.Round.Uint64()])
	}

	// Test that we only store the msg with the highest round for each validator
	roundMultiplier := 1
	for j := 1; j <= roundMultiplier; j++ {
		for i, v := range vset.List() {
			view := &istanbul.View{
				Sequence: big.NewInt(1),
				Round:    big.NewInt(int64(i * j)),
			}
			r := &istanbul.Subject{
				View:   view,
				Digest: common.Hash{},
			}
			m, _ := Encode(r)
			msg := &message{
				Code:    msgRoundChange,
				Msg:     m,
				Address: v.Address(),
			}
			err := rc.Add(view.Round, msg, v)
			if err != nil {
				t.Errorf("Round change message: unexpected error %v", err)
			}
		}
	}

	for i, v := range vset.List() {
		lookingForValAtRound := uint64(roundMultiplier * i)
		if rc.msgsForRound[lookingForValAtRound].Size() != 1 {
			t.Errorf("Round change messages at unexpected rounds: %v", rc.msgsForRound)
		}
		if rc.latestRoundForVal[v.Address()] != lookingForValAtRound {
			t.Errorf("Round change messages at unexpected rounds: for %v want %v have %v",
				i, rc.latestRoundForVal[v.Address()], lookingForValAtRound)
		}
	}

	for threshold := 1; threshold < vset.Size(); threshold++ {
		r := rc.MaxRound(threshold).Uint64()
		expectedR := uint64((vset.Size() - threshold) * roundMultiplier)
		if r != expectedR {
			t.Errorf("MaxRound: %v want %v have %v", rc.String(), expectedR, r)
		}
	}
}

func (ts *testSystem) distributeIstMsgs(t *testing.T, sys *testSystem, istMsgDistribution map[uint64]map[int]bool) {
	for {
		select {
		case <-ts.quit:
			return
		case event := <-ts.queuedMessage:
			msg := new(message)
			if err := msg.FromPayload(event.Payload, nil); err != nil {
				t.Errorf("Could not decode payload")
			}

			targets := istMsgDistribution[msg.Code]
			for index, b := range sys.backends {
				if targets[index] || msg.Address == b.address {
					go b.EventMux().Post(event)
				}
			}
		}
	}
}

var gossip = map[int]bool{
	0: true,
	1: true,
	2: true,
	3: true,
}

var noGossip = map[int]bool{
	0: false,
	1: false,
	2: false,
	3: false,
}

func TestRoundChangeWithLock(t *testing.T) {
	sys := NewTestSystemWithBackend(4, 1)

	for _, b := range sys.backends {
		b.engine.Start() // start Istanbul core
	}

	newBlocks := sys.backends[3].EventMux().Subscribe(istanbul.FinalCommittedEvent{})
	defer newBlocks.Unsubscribe()

	istMsgDistribution := map[uint64]map[int]bool{}

	istMsgDistribution[msgPreprepare] = gossip
	istMsgDistribution[msgPrepare] = gossip
	istMsgDistribution[msgCommit] = gossip
	istMsgDistribution[msgRoundChange] = gossip

	go sys.distributeIstMsgs(t, sys, istMsgDistribution)

	// Start the first preprepare
	sys.backends[0].NewRequest(makeBlock(1))

	// Received the first block which will setup the round change timeout
	<-newBlocks.Chan()
	// Do not propagate any prepares
	// Shame be upon me for modifying shared memory between two goroutines
	// but this seemed much easier than to setup channels
	istMsgDistribution[msgCommit] = noGossip
	sys.backends[0].NewRequest(makeBlock(2))

	// By now we should have sent prepares
	<-time.After(2 * time.Second)
	istMsgDistribution[msgCommit] = gossip

	// Eventually we should get a block again
	select {
	case <-time.After(time.Duration(istanbul.DefaultConfig.RequestTimeout) * time.Millisecond):
		t.Error("Never finalized block")
	case _, ok := <-newBlocks.Chan():
		if !ok {
			t.Error("Error reading block")
		}
	}

	// Manually open and close b/c hijacking sys.listen
	for _, b := range sys.backends {
		b.engine.Stop() // start Istanbul core
	}
	close(sys.quit)
}
