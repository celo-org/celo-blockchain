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

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/rlp"
)

func mockSub() *istanbul.Subject {
	view := &istanbul.View{
		Round:    new(big.Int),
		Sequence: new(big.Int),
	}

	return &istanbul.Subject{
		View:   view,
		Digest: common.BytesToHash([]byte("1234567890")),
	}
}

func mockMsg(rawSub []byte, valIndex uint64, valSet istanbul.ValidatorSet) *istanbul.Message {
	return &istanbul.Message{
		Code:    istanbul.MsgPrepare,
		Msg:     rawSub,
		Address: valSet.GetByIndex(valIndex).Address(),
	}
}

func TestMessageSetWithSubject(t *testing.T) {
	valSet := newTestValidatorSet(4)

	ms := newMessageSet(valSet)

	rawSub, err := rlp.EncodeToBytes(mockSub())
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	msg := mockMsg(rawSub, 0, valSet)

	err = ms.Add(msg)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	err = ms.Add(msg)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if ms.Size() != 1 {
		t.Errorf("the size of message set mismatch: have %v, want 1", ms.Size())
	}
}

func TestAddAll(t *testing.T) {
	valSet := newTestValidatorSet(16)

	ms := newMessageSet(valSet)

	rawSub, err := rlp.EncodeToBytes(mockSub())
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	ms.Add(mockMsg(rawSub, 0, valSet))
	ms.Add(mockMsg(rawSub, 1, valSet))
	ms.Add(mockMsg(rawSub, 2, valSet))
	ms.Add(mockMsg(rawSub, 3, valSet))
	ms.Add(mockMsg(rawSub, 4, valSet))

	if ms.Size() != 5 {
		t.Errorf("the size of message set mismatch: have %v, want 5", ms.Size())
	}

	msgs := []*istanbul.Message{
		mockMsg(rawSub, 3, valSet),
		mockMsg(rawSub, 5, valSet),
		mockMsg(rawSub, 6, valSet),
		mockMsg(rawSub, 7, valSet),
	}

	added := ms.AddAll(msgs)

	if ms.Size() != 8 {
		t.Errorf("the size of message set mismatch: have %v, want 8", ms.Size())
	}

	if added != 3 {
		t.Errorf("added amount to message set mismatch: have %v, want 3", added)
	}
}
