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
	// Add messages from each validators at round 1
	for i, v := range vset.List() {
		msg := &message{
			Code:    msgRoundChange,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view, msg, v)
		if rc.msgsForRound[view.Round.Uint64()].Size() != i+1 {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.msgsForRound[view.Round.Uint64()].Size(), i+1)
		}
	}

	// Add message again from each validator. Number of stored messages shouldn't change
	for _, v := range vset.List() {
		msg := &message{
			Code:    msgRoundChange,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view, msg, v)
		if rc.msgsForRound[view.Round.Uint64()].Size() != vset.Size() {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.msgsForRound[view.Round.Uint64()].Size(), vset.Size())
		}
	}

	// Test GreatestRoundForThreshold()
	for i := 0; i < 10; i++ {
		maxRound := rc.GreatestRoundForThreshold(i)
		if i <= vset.Size() {
			if maxRound == nil || maxRound.Cmp(view.Round) != 0 {
				t.Errorf("GreatestRoundForThreshold mismatch: have %v, want %v", maxRound, view.Round)
			}
		} else if maxRound != nil {
			t.Errorf("GreatestRoundForThreshold mismatch: have %v, want nil", maxRound)
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
		rc.Add(view, msg, v)
		if rc.msgsForRound[view.Round.Uint64()].Size() != i+1 {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.msgsForRound[view.Round.Uint64()].Size(), i+1)
		}
	}

	rc.Clear(big.NewInt(2))
	if rc.msgsForRound[view.Round.Uint64()] != nil {
		t.Errorf("the change messages mismatch: have %v, want nil", rc.msgsForRound[view.Round.Uint64()])
	}

	// Test that we only store the msg with the highest round for each validator
	roundMultiplier := 2
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
			err := rc.Add(view, msg, v)
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

	// Check tail + prev pointers
	for threshold := 1; threshold < vset.Size(); threshold++ {
		r := rc.GreatestRoundForThreshold(threshold).Uint64()
		expectedR := uint64((vset.Size() - threshold) * roundMultiplier)
		if r != expectedR {
			t.Errorf("GreatestRoundForThreshold: %v want %v have %v", rc.String(), expectedR, r)
		}
	}

	// Check head + next pointers
	rms := rc.least
	var prevRms *messageSet
	expectedR := 0
	for rms != nil {
		if rms.View().Round.Uint64() != uint64(expectedR) {
			t.Errorf("Round change links invalid: %v want %v have %v", rc.String(), expectedR, rms.View().Round)
		}
		expectedR += roundMultiplier
		prevRms = rms
		rms = rms.next
		if rms != nil && rms.prev != prevRms {
			t.Errorf("Round change links invalid: %v at %v", rc.String(), rms)
		}
	}
	if prevRms.next != nil || rc.greatest != prevRms {
		t.Errorf("Round change links invalid at tail: %v at %v", rc.String(), prevRms)

	}

}
