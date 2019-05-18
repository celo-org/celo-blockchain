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
	addresses, _ := generateValidators(4)
	vset := validator.NewSet(addresses, istanbul.RoundRobin)
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
		msg := &istanbul.Message{
			Code:    istanbul.MsgRoundChange,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view.Round, msg, nil)
		if rc.roundChanges[view.Round.Uint64()].Size() != i+1 {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.roundChanges[view.Round.Uint64()].Size(), i+1)
		}
	}

	// Add message again from all validators, but the size should be the same
	for _, v := range vset.List() {
		msg := &istanbul.Message{
			Code:    istanbul.MsgRoundChange,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view.Round, msg, nil)
		if rc.roundChanges[view.Round.Uint64()].Size() != vset.Size() {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.roundChanges[view.Round.Uint64()].Size(), vset.Size())
		}
	}

	// Test MaxRound()
	for i := 0; i < 10; i++ {
		maxRound := rc.MaxRound(i)
		if i <= vset.Size() {
			if maxRound == nil || maxRound.Cmp(view.Round) != 0 {
				t.Errorf("max round mismatch: have %v, want %v", maxRound, view.Round)
			}
		} else if maxRound != nil {
			t.Errorf("max round mismatch: have %v, want nil", maxRound)
		}
	}

	// Test Clear()
	for i := int64(0); i < 2; i++ {
		rc.Clear(big.NewInt(i))
		if rc.roundChanges[view.Round.Uint64()].Size() != vset.Size() {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.roundChanges[view.Round.Uint64()].Size(), vset.Size())
		}
	}
	rc.Clear(big.NewInt(2))
	if rc.roundChanges[view.Round.Uint64()] != nil {
		t.Errorf("the change messages mismatch: have %v, want nil", rc.roundChanges[view.Round.Uint64()])
	}
}

func TestVerifyPreparedCertificate(t *testing.T) {
	N := uint64(4) // replica 0 is the proposer, it will send messages to others
	F := uint64(1)
	sys := NewTestSystemWithBackend(N, F)
	view := istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(1),
	}

	testCases := []struct {
		certificate istanbul.PreparedCertificate
		expectedErr error
	}{
		{
			// Valid PREPARED certificate
			sys.getPreparedCertificate(t, view, makeBlock(0)),
			nil,
		},
		{
			// Invalid PREPARED certificate, duplicate message
			func() istanbul.PreparedCertificate {
				preparedCertificate := sys.getPreparedCertificate(t, view, makeBlock(0))
				preparedCertificate.PrepareMessages[1] = preparedCertificate.PrepareMessages[0]
				return preparedCertificate
			}(),
			errInvalidPreparedCertificateDuplicate,
		},
		{
			// Invalid PREPARED certificate, hash mismatch
			func() istanbul.PreparedCertificate {
				preparedCertificate := sys.getPreparedCertificate(t, view, makeBlock(0))
				preparedCertificate.PrepareMessages[1] = preparedCertificate.PrepareMessages[0]
				preparedCertificate.Proposal = makeBlock(1)
				return preparedCertificate
			}(),
			errInvalidPreparedCertificateDigestMismatch,
		},
		{
			// Empty certificate
			istanbul.EmptyPreparedCertificate(),
			errInvalidPreparedCertificateNumMsgs,
		},
	}
	for _, test := range testCases {
		for _, backend := range sys.backends {
			c := backend.engine.(*core)
			err := c.validatePreparedCertificate(test.certificate)
			if err != test.expectedErr {
				t.Errorf("error mismatch: have %v, want %v", err, test.expectedErr)
			}
		}
	}
}

func TestHandleRoundChange(t *testing.T) {
	N := uint64(4) // replica 0 is the proposer, it will send messages to others
	F := uint64(1) // F does not affect tests

	testCases := []struct {
		system      *testSystem
		getCert     func(*testSystem) istanbul.PreparedCertificate
		expectedErr error
	}{
		{
			// normal case
			NewTestSystemWithBackend(N, F),
			func(_ *testSystem) istanbul.PreparedCertificate {
				return istanbul.EmptyPreparedCertificate()
			},
			nil,
		},
		{
			// normal case with valid prepared certificate
			NewTestSystemWithBackend(N, F),
			func(sys *testSystem) istanbul.PreparedCertificate {
				return sys.getPreparedCertificate(t, *sys.backends[0].engine.(*core).currentView(), makeBlock(1))
			},
			nil,
		},
		{
			// normal case with invalid prepared certificate
			NewTestSystemWithBackend(N, F),
			func(sys *testSystem) istanbul.PreparedCertificate {
				preparedCert := sys.getPreparedCertificate(t, *sys.backends[0].engine.(*core).currentView(), makeBlock(1))
				preparedCert.PrepareMessages[0] = preparedCert.PrepareMessages[1]
				return preparedCert
			},
			errInvalidPreparedCertificateDuplicate,
		},
		{
			// valid message for future round
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)
				sys.backends[0].engine.(*core).current.SetRound(big.NewInt(10))
				return sys
			}(),
			func(_ *testSystem) istanbul.PreparedCertificate {
				return istanbul.EmptyPreparedCertificate()
			},
			errIgnored,
		},
		{
			// invalid message for future sequence
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)
				sys.backends[0].engine.(*core).current.SetSequence(big.NewInt(10))
				return sys
			}(),
			func(_ *testSystem) istanbul.PreparedCertificate {
				return istanbul.EmptyPreparedCertificate()
			},
			errFutureMessage,
		},
		{
			// TODO(asa): This doesn't seem to be running
			// invalid message for previous round
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)
				sys.backends[0].engine.(*core).current.SetRound(big.NewInt(0))
				return sys
			}(),
			func(_ *testSystem) istanbul.PreparedCertificate {
				return istanbul.EmptyPreparedCertificate()
			},
			nil,
		},
	}

OUTER:
	for _, test := range testCases {
		test.system.Run(false)

		v0 := test.system.backends[0]
		r0 := v0.engine.(*core)

		curView := r0.currentView()

		roundChange := &istanbul.RoundChange{
			View:                curView,
			PreparedCertificate: test.getCert(test.system),
		}

		for i, v := range test.system.backends {
			// i == 0 is primary backend, it is responsible for send ROUND CHANGE messages to others.
			if i == 0 {
				continue
			}

			c := v.engine.(*core)

			m, _ := Encode(roundChange)
			_, val := r0.valSet.GetByAddress(v0.Address())
			// run each backends and verify handlePreprepare function.
			err := c.handleRoundChange(&istanbul.Message{
				Code:    istanbul.MsgRoundChange,
				Msg:     m,
				Address: v0.Address(),
			}, val)
			if err != test.expectedErr {
				t.Errorf("error mismatch: have %v, want %v", err, test.expectedErr)
			}
			continue OUTER
		}
	}
}

func (ts *testSystem) distributeIstMsgs(t *testing.T, sys *testSystem, istMsgDistribution map[uint64]map[int]bool) {
	for {
		select {
		case <-ts.quit:
			return
		case event := <-ts.queuedMessage:
			msg := new(istanbul.Message)
			if err := msg.FromPayload(event.Payload, nil); err != nil {
				t.Errorf("Could not decode payload")
			}

			targets := istMsgDistribution[msg.Code]
			for index, b := range sys.backends {
				if targets[index] || msg.Address == b.address {
					go b.EventMux().Post(event)
				} else {
					testLogger.Info("ignoring message with code", "code", msg.Code)
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

var sendTo2FPlus1 = map[int]bool{
	0: true,
	1: true,
	2: true,
	3: false,
}

var sendToF = map[int]bool{
	0: false,
	1: false,
	2: false,
	3: true,
}

var noGossip = map[int]bool{
	0: false,
	1: false,
	2: false,
	3: false,
}

// This tests the liveness issue present in the initial implementation of Istanbul, descibed in
// more detail here: https://arxiv.org/pdf/1901.07160.pdf
// To test this, a block is proposed, for which 2F + 1 PREPARE messages are sent to F nodes.
// In the original implementation, these F nodes would lock onto that block, and eventually everyone would
// round change. If the next proposer was byzantine, they could send a PRE-PREPARED with a different block,
// get the remaining 2F non-byzantine nodes to lock onto that new block, causing a deadlock.
// In the new implementation, the PRE-PREPARE will include a ROUND CHANGE certificate,
// and the original F nodes will accept the newly proposed block.
func TestCommitsBlocksAfterRoundChange(t *testing.T) {
	// Issue was that we weren't timing out because startNewRound was thinking we were on the same round.
	sys := NewTestSystemWithBackendAndCurrentRoundState(4, 1, func(vset istanbul.ValidatorSet) *roundState { return nil })

	for i, b := range sys.backends {
		b.engine.Start() // start Istanbul core
		block := makeBlockWithDifficulty(1, int64(i))
		sys.backends[i].NewRequest(block)
	}

	newBlocks := sys.backends[3].EventMux().Subscribe(istanbul.FinalCommittedEvent{})
	defer newBlocks.Unsubscribe()

	istMsgDistribution := map[uint64]map[int]bool{}

	// Allow everyone to see the initial proposal
	// Send all PREPARE messages to F nodes.
	// Do not send COMMIT messages (we don't expect these to be sent anyway).
	// Send ROUND CHANGE messages to the remaining 2F + 1 nodes.
	istMsgDistribution[istanbul.MsgPreprepare] = gossip
	istMsgDistribution[istanbul.MsgPrepare] = sendToF
	istMsgDistribution[istanbul.MsgCommit] = gossip
	istMsgDistribution[istanbul.MsgRoundChange] = gossip

	go sys.distributeIstMsgs(t, sys, istMsgDistribution)

	// By now we should have sent prepares
	<-time.After(1 * time.Second)
	istMsgDistribution[istanbul.MsgPrepare] = gossip

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
