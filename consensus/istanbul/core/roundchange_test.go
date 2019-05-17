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

// TODO(asa): Test with PREPARED certificate
func TestValidatePreparedCertificate(t *testing.T) {
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
			err := c.ValidatePreparedCertificate(test.certificate)
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
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for _, backend := range sys.backends {
					c := backend.engine.(*core)
					c.valSet = backend.peers
				}
				return sys
			}(),
			func(_ *testSystem) istanbul.PreparedCertificate {
				return istanbul.EmptyPreparedCertificate()
			},
			nil,
		},
		/*
			{
				// future message
				func() *testSystem {
					sys := NewTestSystemWithBackend(N, F)

					for i, backend := range sys.backends {
						c := backend.engine.(*core)
						c.valSet = backend.peers
						if i != 0 {
							c.state = StateAcceptRequest
							// hack: force set subject that future message can be simulated
							c.current = newTestRoundState(
								&istanbul.View{
									Round:    big.NewInt(0),
									Sequence: big.NewInt(0),
								},
								c.valSet,
							)

						} else {
							c.current.SetSequence(big.NewInt(10))
						}
					}
					return sys
				}(),
				func(_ *testSystem) istanbul.PreparedCertificate {
					return istanbul.PreparedCertificate{}
				},
				makeBlock(1),
				errFutureMessage,
				false,
			},
			{
				// non-proposer
				func() *testSystem {
					sys := NewTestSystemWithBackend(N, F)

					// force remove replica 0, let replica 1 be the proposer
					sys.backends = sys.backends[1:]

					for i, backend := range sys.backends {
						c := backend.engine.(*core)
						c.valSet = backend.peers
						if i != 0 {
							// replica 0 is the proposer
							c.state = StatePreprepared
						}
					}
					return sys
				}(),
				func(_ *testSystem) istanbul.PreparedCertificate {
					return istanbul.PreparedCertificate{}
				},
				makeBlock(1),
				errNotFromProposer,
				false,
			},
			{
				// errOldMessage
				func() *testSystem {
					sys := NewTestSystemWithBackend(N, F)

					for i, backend := range sys.backends {
						c := backend.engine.(*core)
						c.valSet = backend.peers
						if i != 0 {
							c.state = StatePreprepared
							c.current.SetSequence(big.NewInt(10))
							c.current.SetRound(big.NewInt(10))
						}
					}
					return sys
				}(),
				func(_ *testSystem) istanbul.PreparedCertificate {
					return istanbul.PreparedCertificate{}
				},
				makeBlock(1),
				errOldMessage,
				false,
			},
			{
				// ROUND CHANGE certificate missing
				func() *testSystem {
					sys := NewTestSystemWithBackend(N, F)

					for _, backend := range sys.backends {
						c := backend.engine.(*core)
						c.valSet = backend.peers
						c.state = StatePreprepared
						c.current.SetRound(big.NewInt(1))
					}
					return sys
				}(),
				func(_ *testSystem) istanbul.PreparedCertificate {
					return istanbul.PreparedCertificate{}
				},
				makeBlock(1),
				errMissingPreparedCertificate,
				false,
			},
			{
				// ROUND CHANGE certificate invalid
				func() *testSystem {
					sys := NewTestSystemWithBackend(N, F)

					for _, backend := range sys.backends {
						c := backend.engine.(*core)
						c.valSet = backend.peers
						c.state = StatePreprepared
						c.current.SetRound(big.NewInt(1))
					}
					return sys
				}(),
				func(sys *testSystem) istanbul.PreparedCertificate {
					// Duplicate messages
					preparedCertificate := sys.getPreparedCertificate(t, *(sys.backends[0].engine.(*core).currentView()), istanbul.EmptyPreparedCertificate())
					preparedCertificate.PrepareMessages[1] = preparedCertificate.PrepareMessages[0]
					return preparedCertificate
				},
				makeBlock(1),
				errInvalidPreparedCertificateDuplicate,
				false,
			},
			{
				// ROUND CHANGE certificate contains PREPARED certificate for a different block.
				func() *testSystem {
					sys := NewTestSystemWithBackend(N, F)

					for _, backend := range sys.backends {
						c := backend.engine.(*core)
						c.valSet = backend.peers
						c.state = StatePreprepared
						c.current.SetRound(big.NewInt(1))
					}
					return sys
				}(),
				func(sys *testSystem) istanbul.PreparedCertificate {
					preparedCertificate := sys.getPreparedCertificate(t, *(sys.backends[0].engine.(*core).currentView()), makeBlock(2))
					preparedCertificate := sys.getPreparedCertificate(t, *(sys.backends[0].engine.(*core).currentView()), preparedCertificate)
					return preparedCertificate
				},
				makeBlock(1),
				errInvalidProposal,
				false,
			},
		*/
	}

OUTER:
	for i, test := range testCases {
		testLogger.Info("Running handle round change test case", "number", i)
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
			if err := c.handleRoundChange(&istanbul.Message{
				Code:    istanbul.MsgRoundChange,
				Msg:     m,
				Address: v0.Address(),
			}, val); err != nil {
				if err != test.expectedErr {
					t.Errorf("error mismatch: have %v, want %v", err, test.expectedErr)
				}
				continue OUTER
			}

			/*
				if c.state != StatePreprepared {
					t.Errorf("state mismatch: have %v, want %v", c.state, StatePreprepared)
				}

				if !test.existingBlock && !reflect.DeepEqual(c.current.Subject().View, curView) {
					t.Errorf("view mismatch: have %v, want %v", c.current.Subject().View, curView)
				}

				// verify prepare messages
				decodedMsg := new(istanbul.Message)
				err := decodedMsg.FromPayload(v.sentMsgs[0], nil)
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}

				expectedCode := istanbul.MsgPrepare
				if test.existingBlock {
					expectedCode = istanbul.MsgCommit
				}
				if decodedMsg.Code != expectedCode {
					t.Errorf("message code mismatch: have %v, want %v", decodedMsg.Code, expectedCode)
				}

				var subject *istanbul.Subject
				err = decodedMsg.Decode(&subject)
				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}
				if !test.existingBlock && !reflect.DeepEqual(subject, c.current.Subject()) {
					t.Errorf("subject mismatch: have %v, want %v", subject, c.current.Subject())
				}
			*/

		}
	}
}
