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

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
)

func TestRoundChangeSet(t *testing.T) {
	vals, _, _ := generateValidators(4)
	vset := validator.NewSet(vals)
	rc := newRoundChangeSet(vset)

	view := &istanbul.View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(1),
	}
	r := &istanbul.Subject{
		View:   view,
		Digest: common.Hash{},
	}

	// Test Add()
	// Add message from all validators
	for i, v := range vset.List() {
		rc.Add(view.Round, istanbul.NewMessage(r, v.Address()))
		if rc.msgsForRound[view.Round.Uint64()].Size() != i+1 {
			t.Errorf("the size of round change messages mismatch: have %v, want %v", rc.msgsForRound[view.Round.Uint64()].Size(), i+1)
		}
	}

	// Add message again from all validators, but the size should be the same
	for _, v := range vset.List() {
		rc.Add(view.Round, istanbul.NewMessage(r, v.Address()))
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
		rc.Add(view.Round, istanbul.NewMessage(r, v.Address()))
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
				Round:    big.NewInt(int64((i + 1) * j)),
			}
			r := &istanbul.Subject{
				View:   view,
				Digest: common.Hash{},
			}
			m, _ := Encode(r)
			msg := &istanbul.Message{
				Code:    istanbul.MsgRoundChange,
				Msg:     m,
				Address: v.Address(),
			}
			err := rc.Add(view.Round, msg)
			if err != nil {
				t.Errorf("Round change message: unexpected error %v", err)
			}
		}
	}

	for i, v := range vset.List() {
		lookingForValAtRound := uint64(roundMultiplier * (i + 1))
		if rc.msgsForRound[lookingForValAtRound].Size() != 1 {
			t.Errorf("Round change messages at unexpected rounds: %v", rc.msgsForRound)
		}
		if rc.latestRoundForVal[v.Address()] != lookingForValAtRound {
			t.Errorf("Round change messages at unexpected rounds: for %v want %v have %v",
				i+1, rc.latestRoundForVal[v.Address()], lookingForValAtRound)
		}
	}

	for threshold := 1; threshold <= vset.Size(); threshold++ {
		r := rc.MaxRound(threshold).Uint64()
		expectedR := uint64((vset.Size() - threshold + 1) * roundMultiplier)
		if r != expectedR {
			t.Errorf("MaxRound: %v want %v have %v", rc.String(), expectedR, r)
		}
	}

	// Test getCertificate
	for r := 1; r < vset.Size(); r += roundMultiplier {
		expectedMsgsAtRound := vset.Size() - r + 1
		for quorum := 1; quorum < 10; quorum++ {
			cert, err := rc.getCertificate(big.NewInt(int64(r)), quorum)
			if expectedMsgsAtRound < quorum {
				// Expecting fewer than quorum.
				if err != errFailedCreateRoundChangeCertificate || len(cert.RoundChangeMessages) != 0 {
					t.Errorf("problem in getCertificate r=%v q=%v expMsgs=%v - want 0 have %v err=%v -- %v -- %v", r, quorum, expectedMsgsAtRound, len(cert.RoundChangeMessages), err, cert, rc)
				}
			} else {
				// Number msgs available at this round is >= quorum. Expecting a cert with =quorum RC messages.
				if err != nil || len(cert.RoundChangeMessages) != quorum {
					t.Errorf("problem in getCertificate r=%v q=%v expMsgs=%v - want %v have %v -- %v -- %v", r, quorum, quorum, expectedMsgsAtRound, len(cert.RoundChangeMessages), cert, rc)
				}
			}
		}
	}
}

func TestHandleRoundChangeCertificate(t *testing.T) {
	N := uint64(4) // replica 0 is the proposer, it will send messages to others
	F := uint64(1)
	view := istanbul.View{
		Round:    big.NewInt(1),
		Sequence: big.NewInt(1),
	}

	testCases := []struct {
		name           string
		getCertificate func(*testing.T, *testSystem) istanbul.RoundChangeCertificate
		expectedErr    error
	}{
		{
			"Valid round change certificate without PREPARED certificate",
			func(t *testing.T, sys *testSystem) istanbul.RoundChangeCertificate {
				return sys.getRoundChangeCertificate(t, []istanbul.View{view}, istanbul.EmptyPreparedCertificate())
			},
			nil,
		},
		{
			"Valid round change certificate with PREPARED certificate",
			func(t *testing.T, sys *testSystem) istanbul.RoundChangeCertificate {
				return sys.getRoundChangeCertificate(t, []istanbul.View{view}, sys.getPreparedCertificate(t, []istanbul.View{view}, makeBlock(0)))
			},
			nil,
		},
		{
			"Invalid round change certificate, duplicate message",
			func(t *testing.T, sys *testSystem) istanbul.RoundChangeCertificate {
				roundChangeCertificate := sys.getRoundChangeCertificate(t, []istanbul.View{view}, istanbul.EmptyPreparedCertificate())
				roundChangeCertificate.RoundChangeMessages[1] = roundChangeCertificate.RoundChangeMessages[0]
				return roundChangeCertificate
			},
			errInvalidRoundChangeCertificateDuplicate,
		},
		{
			"Empty certificate",
			func(t *testing.T, sys *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
			},
			errInvalidRoundChangeCertificateNumMsgs,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			sys := NewTestSystemWithBackend(N, F)
			for i, backend := range sys.backends {
				c := backend.engine.(*core)
				c.Start()
				certificate := test.getCertificate(t, sys)
				subject := istanbul.Subject{
					View:   &view,
					Digest: makeBlock(0).Hash(),
				}
				err := c.handleRoundChangeCertificate(subject, certificate)

				if err != test.expectedErr {
					t.Errorf("error mismatch for test case %v: have %v, want %v", i, err, test.expectedErr)
				}
				if err == nil && c.current.View().Cmp(&view) != 0 {
					t.Errorf("view mismatch for test case %v: have %v, want %v", i, c.current.View(), view)
				}
			}
			for _, backend := range sys.backends {
				backend.engine.Stop()
			}
			close(sys.quit)
		})
	}
}

func TestHandleRoundChange(t *testing.T) {
	N := uint64(4) // replica 0 is the proposer, it will send messages to others
	F := uint64(1) // F does not affect tests

	buildEmptyCertificate := func(_ *testing.T, _ *testSystem) istanbul.PreparedCertificate {
		return istanbul.EmptyPreparedCertificate()
	}

	noopPrepare := func(_ *testSystem) {}

	testCases := []struct {
		name          string
		prepareSystem func(*testSystem)
		getCert       func(*testing.T, *testSystem) istanbul.PreparedCertificate
		expectedErr   error
	}{
		{
			"normal case",
			noopPrepare,
			buildEmptyCertificate,
			nil,
		},
		{
			"normal case with valid prepared certificate",
			noopPrepare,
			func(t *testing.T, sys *testSystem) istanbul.PreparedCertificate {
				return sys.getPreparedCertificate(t, []istanbul.View{*sys.backends[0].engine.(*core).current.View()}, makeBlock(1))
			},
			nil,
		},
		{
			"normal case with invalid prepared certificate",
			noopPrepare,
			func(t *testing.T, sys *testSystem) istanbul.PreparedCertificate {
				preparedCert := sys.getPreparedCertificate(t, []istanbul.View{*sys.backends[0].engine.(*core).current.View()}, makeBlock(1))
				preparedCert.PrepareMessages[0] = preparedCert.PrepareMessages[1]
				return preparedCert
			},
			errInvalidPreparedCertificateDuplicate,
		},
		{
			"valid message for future round",
			func(sys *testSystem) {
				sys.backends[0].engine.(*core).current.(*rsSaveDecorator).rs.(*roundStateImpl).round = big.NewInt(10)
			},
			func(t *testing.T, _ *testSystem) istanbul.PreparedCertificate {
				return istanbul.EmptyPreparedCertificate()
			},
			nil,
		},
		{
			"invalid message for future sequence",
			func(sys *testSystem) {
				sys.backends[0].engine.(*core).current.(*rsSaveDecorator).rs.(*roundStateImpl).sequence = big.NewInt(10)
			},
			buildEmptyCertificate,
			errFutureMessage,
		},
		{
			"invalid message for previous round",
			func(sys *testSystem) {
				sys.backends[0].engine.(*core).current.(*rsSaveDecorator).rs.(*roundStateImpl).round = big.NewInt(0)
			},
			buildEmptyCertificate,
			nil,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			sys := NewTestSystemWithBackend(N, F)

			closer := sys.Run(false)
			defer closer()

			for _, v := range sys.backends {
				v.engine.(*core).Start()
			}
			test.prepareSystem(sys)

			v0 := sys.backends[0]
			r0 := v0.engine.(*core)

			curView := r0.current.View()
			nextView := &istanbul.View{
				Round:    new(big.Int).Add(curView.Round, common.Big1),
				Sequence: curView.Sequence,
			}

			msg := istanbul.NewMessage(&istanbul.RoundChange{
				View:                nextView,
				PreparedCertificate: test.getCert(t, sys),
			}, v0.Address())

			for i, v := range sys.backends {
				// i == 0 is primary backend, it is responsible for send ROUND CHANGE messages to others.
				if i == 0 {
					continue
				}

				c := v.engine.(*core)

				// run each backends and verify handlePreprepare function.
				err := c.handleRoundChange(msg)
				if err != test.expectedErr {
					t.Errorf("error mismatch: have %v, want %v", err, test.expectedErr)
				}
				return
			}
		})
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
					testLogger.Info("not sending msg", "index", index, "from", msg.Address, "to", b.address, "code", msg.Code)
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

var sendToFPlus1 = map[int]bool{
	0: false,
	1: false,
	2: true,
	3: true,
}
var noGossip = map[int]bool{
	0: false,
	1: false,
	2: false,
	3: false,
}

// This tests the liveness issue present in the initial implementation of Istanbul, described in
// more detail here: https://arxiv.org/pdf/1901.07160.pdf
// To test this, a block is proposed, for which 2F + 1 PREPARE messages are sent to F nodes.
// In the original implementation, these F nodes would lock onto that block, and eventually everyone would
// round change. If the next proposer was byzantine, they could send a PREPREPARE with a different block,
// get the remaining 2F non-byzantine nodes to lock onto that new block, causing a deadlock.
// In the new implementation, the PREPREPARE will include a ROUND CHANGE certificate,
// and all nodes will accept the newly proposed block.
func TestCommitsBlocksAfterRoundChange(t *testing.T) {
	sys := NewTestSystemWithBackend(4, 1)

	for _, b := range sys.backends {
		b.engine.Start() // start Istanbul core
		block := makeBlock(1)
		b.NewRequest(block)
	}

	newBlocks := sys.backends[0].EventMux().Subscribe(istanbul.FinalCommittedEvent{})
	defer newBlocks.Unsubscribe()

	timeout := sys.backends[3].EventMux().Subscribe(timeoutAndMoveToNextRoundEvent{})
	defer timeout.Unsubscribe()

	istMsgDistribution := map[uint64]map[int]bool{}

	// Allow everyone to see the initial proposal
	// Send all PREPARE messages to F nodes.
	// Send COMMIT messages (we don't expect these to be sent in the first round anyway).
	// Send ROUND CHANGE messages to the remaining 2F + 1 nodes.
	istMsgDistribution[istanbul.MsgPreprepare] = gossip
	istMsgDistribution[istanbul.MsgPrepare] = sendToF
	istMsgDistribution[istanbul.MsgCommit] = gossip
	istMsgDistribution[istanbul.MsgRoundChange] = sendTo2FPlus1

	go sys.distributeIstMsgs(t, sys, istMsgDistribution)

	<-timeout.Chan()

	// Turn PREPAREs back on for round 1.
	testLogger.Info("Turn PREPAREs back on for round 1")
	istMsgDistribution[istanbul.MsgPrepare] = gossip

	// Eventually we should get a block again
	select {
	case <-time.After(2 * time.Second):
		t.Error("Did not finalize a block within 2 secs")
	case _, ok := <-newBlocks.Chan():
		if !ok {
			t.Error("Error reading block")
		}
		// Wait for all backends to finalize the block.
		<-time.After(1 * time.Second)
		testLogger.Info("Expected all backends to finalize")
		expectedCommitted, _ := sys.backends[0].GetCurrentHeadBlockAndAuthor()
		for i, b := range sys.backends {
			committed, _ := b.GetCurrentHeadBlockAndAuthor()
			// We don't expect any particular block to be committed here. We do expect them to be consistent.
			if committed.Number().Cmp(common.Big1) != 0 {
				t.Errorf("Backend %v got committed block with unexpected number: expected %v, got %v", i, 1, committed.Number())
			}
			if expectedCommitted.Hash() != committed.Hash() {
				t.Errorf("Backend %v got committed block with unexpected hash: expected %v, got %v", i, expectedCommitted.Hash(), committed.Hash())
			}
		}
	}

	// Manually open and close b/c hijacking sys.listen
	for _, b := range sys.backends {
		b.engine.Stop() // stop Istanbul core
	}
	close(sys.quit)
}

// This tests that when F+1 nodes receive 2F+1 PREPARE messages for a particular proposal, the
// system enforces that as the only valid proposal for this sequence.
func TestPreparedCertificatePersistsThroughRoundChanges(t *testing.T) {
	t.Skip("This test is not working as expected, and was actually committing in the first round")
	sys := NewTestSystemWithBackend(4, 1)

	for _, b := range sys.backends {
		b.engine.Start() // start Istanbul core
		block := makeBlock(1)
		b.NewRequest(block)
	}

	newBlocks := sys.backends[3].EventMux().Subscribe(istanbul.FinalCommittedEvent{})
	defer newBlocks.Unsubscribe()

	timeout := sys.backends[3].EventMux().Subscribe(timeoutAndMoveToNextRoundEvent{})
	defer timeout.Unsubscribe()

	istMsgDistribution := map[uint64]map[int]bool{}

	// Send PREPARE messages to F + 1 nodes so we guarantee a PREPARED certificate in the ROUND CHANGE certificate..
	istMsgDistribution[istanbul.MsgPreprepare] = gossip
	istMsgDistribution[istanbul.MsgPrepare] = sendToFPlus1
	istMsgDistribution[istanbul.MsgCommit] = gossip
	istMsgDistribution[istanbul.MsgRoundChange] = gossip

	go sys.distributeIstMsgs(t, sys, istMsgDistribution)

	// Turn PREPARE messages off for round 1 to force reuse of the PREPARED certificate.
	<-time.After(1 * time.Second)
	istMsgDistribution[istanbul.MsgPrepare] = noGossip

	// Wait for round 1 to start.
	<-timeout.Chan()
	// Turn PREPARE messages back on in time for round 2.
	<-time.After(1 * time.Second)
	istMsgDistribution[istanbul.MsgPrepare] = gossip

	// Wait for round 2 to start.
	<-timeout.Chan()

	select {
	case <-timeout.Chan():
		t.Error("Did not finalize a block in round 2.")
	case _, ok := <-newBlocks.Chan():
		if !ok {
			t.Error("Error reading block")
		}
		// Wait for all backends to finalize the block.
		<-time.After(2 * time.Second)
		for i, b := range sys.backends {
			committed, _ := b.GetCurrentHeadBlockAndAuthor()
			// We expect to commit the block proposed by the first proposer.
			expectedCommitted := makeBlock(1)
			if committed.Number().Cmp(common.Big1) != 0 {
				t.Errorf("Backend %v got committed block with unexpected number: expected %v, got %v", i, 1, committed.Number())
			}
			if expectedCommitted.Hash() != committed.Hash() {
				t.Errorf("Backend %v got committed block with unexpected hash: expected %v, got %v", i, expectedCommitted.Hash(), committed.Hash())
			}
		}
	}

	// Manually open and close b/c hijacking sys.listen
	for _, b := range sys.backends {
		b.engine.Stop() // stop Istanbul core
	}
	close(sys.quit)
}

// Test periodic round changes at high rounds
func TestPeriodicRoundChanges(t *testing.T) {
	sys := NewTestSystemWithBackend(4, 1)

	for _, b := range sys.backends {
		b.engine.Start() // start Istanbul core
		block := makeBlock(1)
		b.NewRequest(block)
	}

	newBlocks := sys.backends[3].EventMux().Subscribe(istanbul.FinalCommittedEvent{})
	defer newBlocks.Unsubscribe()

	timeoutMoveToNextRound := sys.backends[3].EventMux().Subscribe(timeoutAndMoveToNextRoundEvent{})
	defer timeoutMoveToNextRound.Unsubscribe()

	timeoutResendRC := sys.backends[3].EventMux().Subscribe(resendRoundChangeEvent{})
	defer timeoutResendRC.Unsubscribe()

	istMsgDistribution := map[uint64]map[int]bool{}
	istMsgDistribution[istanbul.MsgPreprepare] = noGossip
	istMsgDistribution[istanbul.MsgPrepare] = noGossip
	istMsgDistribution[istanbul.MsgCommit] = noGossip
	istMsgDistribution[istanbul.MsgRoundChange] = noGossip

	go sys.distributeIstMsgs(t, sys, istMsgDistribution)

	for _, b := range sys.backends {
		b.engine.(*core).waitForDesiredRound(big.NewInt(5))
	}

	// Expect at least one repeat RC before move to next round.
	timeoutResends := 0
loop:
	for {
		select {
		case <-timeoutResendRC.Chan():
			testLogger.Info("Got timeoutResendRC")
			timeoutResends++
		case <-timeoutMoveToNextRound.Chan():
			if timeoutResends == 0 {
				t.Errorf("No Repeat events before moving to next round")
			}
			break loop
		}
	}

	istMsgDistribution[istanbul.MsgPreprepare] = gossip
	istMsgDistribution[istanbul.MsgPrepare] = gossip
	istMsgDistribution[istanbul.MsgCommit] = gossip
	istMsgDistribution[istanbul.MsgRoundChange] = gossip

	// Make sure we finalize block in next two rounds.
	roundTimeouts := 0
loop2:
	for {
		select {
		case <-timeoutMoveToNextRound.Chan():
			roundTimeouts++
			if roundTimeouts > 1 {
				t.Error("Did not finalize a block.")
			}
		case _, ok := <-newBlocks.Chan():
			if !ok {
				t.Error("Error reading block")
			}
			// Wait for all backends to finalize the block.
			<-time.After(2 * time.Second)
			for i, b := range sys.backends {
				committed, _ := b.GetCurrentHeadBlockAndAuthor()
				// We expect to commit the block proposed by proposer 6 mod 4 = 2.
				expectedCommitted := makeBlock(1)
				if committed.Number().Cmp(common.Big1) != 0 {
					t.Errorf("Backend %v got committed block with unexpected number: expected %v, got %v", i, 1, committed.Number())
				}
				if expectedCommitted.Hash() != committed.Hash() {
					t.Errorf("Backend %v got committed block with unexpected hash: expected %v, got %v", i, expectedCommitted.Hash(), committed.Hash())
				}
			}
			break loop2
		}
	}

	// Manually open and close b/c hijacking sys.listen
	for _, b := range sys.backends {
		b.engine.Stop() // stop Istanbul core
	}
	close(sys.quit)
}
