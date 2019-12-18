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
	"testing"

	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

func newTestPreprepare(v *istanbul.View) *istanbul.Preprepare {
	return &istanbul.Preprepare{
		View:     v,
		Proposal: newTestProposal(),
	}
}

func TestHandlePreprepare(t *testing.T) {
	N := uint64(4) // replica 0 is the proposer, it will send messages to others
	F := uint64(1) // F does not affect tests

	getRoundState := func(c *core) *roundStateImpl {
		return c.current.(*rsSaveDecorator).rs.(*roundStateImpl)
	}

	testCases := []struct {
		name            string
		system          func() *testSystem
		getCert         func(*testSystem) istanbul.RoundChangeCertificate
		expectedRequest istanbul.Proposal
		expectedErr     error
		existingBlock   bool
	}{
		{
			"normal case",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for _, backend := range sys.backends {
					backend.engine.(*core).Start()
				}
				return sys
			},
			func(_ *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
			},
			newTestProposal(),
			nil,
			false,
		},
		{
			"proposal sequence doesn't match message sequence",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for _, backend := range sys.backends {
					backend.engine.(*core).Start()
				}
				return sys
			},
			func(_ *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
			},
			makeBlock(3),
			errInvalidProposal,
			false,
		},
		{
			"non-proposer",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				// force remove replica 0, let replica 1 be the proposer
				sys.backends = sys.backends[1:]

				for i, backend := range sys.backends {
					backend.engine.(*core).Start()
					c := backend.engine.(*core)
					if i != 0 {
						// replica 0 is the proposer
						getRoundState(c).state = StatePreprepared
					}
				}
				return sys
			},
			func(_ *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
			},
			makeBlock(1),
			errNotFromProposer,
			false,
		},
		{
			"errOldMessage",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					backend.engine.(*core).Start()
					c := backend.engine.(*core)
					if i != 0 {
						getRoundState(c).state = StatePreprepared
						getRoundState(c).sequence = big.NewInt(10)
						getRoundState(c).round = big.NewInt(10)
					}
				}
				return sys
			},
			func(_ *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
			},
			makeBlock(1),
			errOldMessage,
			false,
		},
		{
			"test existing block",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for _, backend := range sys.backends {
					backend.engine.(*core).Start()
					c := backend.engine.(*core)
					getRoundState(c).state = StatePreprepared
					getRoundState(c).sequence = big.NewInt(10)
					getRoundState(c).round = big.NewInt(10)
				}
				return sys
			},
			func(_ *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
			},
			// In the method testbackend_test.go:HasBlockMatching(), it will return true if the proposal's block number == 5
			makeBlock(5),
			nil,
			true,
		},
		{
			"ROUND CHANGE certificate missing",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for _, backend := range sys.backends {
					backend.engine.(*core).Start()
					c := backend.engine.(*core)
					getRoundState(c).state = StatePreprepared
					getRoundState(c).round = big.NewInt(1)
				}
				return sys
			},
			func(_ *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
			},
			makeBlock(1),
			errMissingRoundChangeCertificate,
			false,
		},
		{
			"ROUND CHANGE certificate invalid, duplicate messages.",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for _, backend := range sys.backends {
					backend.engine.(*core).Start()
					c := backend.engine.(*core)
					getRoundState(c).state = StatePreprepared
					getRoundState(c).round = big.NewInt(1)
				}
				return sys
			},
			func(sys *testSystem) istanbul.RoundChangeCertificate {
				// Duplicate messages
				roundChangeCertificate := sys.getRoundChangeCertificate(t, *(sys.backends[0].engine.(*core).current.View()), istanbul.EmptyPreparedCertificate())
				roundChangeCertificate.RoundChangeMessages[1] = roundChangeCertificate.RoundChangeMessages[0]
				return roundChangeCertificate
			},
			makeBlock(1),
			errInvalidRoundChangeCertificateDuplicate,
			false,
		},
		{
			"ROUND CHANGE certificate contains PREPARED certificate with inconsistent views among the cert's messages",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for _, backend := range sys.backends {
					backend.engine.(*core).Start()
					c := backend.engine.(*core)
					getRoundState(c).state = StatePreprepared
					getRoundState(c).round = big.NewInt(1)
					getRoundState(c).sequence = big.NewInt(2)
					c.current.TransitionToPreprepared(&istanbul.Preprepare{
						View: &istanbul.View{
							Round:    big.NewInt(1),
							Sequence: big.NewInt(2),
						},
						Proposal:               makeBlock(2),
						RoundChangeCertificate: istanbul.RoundChangeCertificate{},
					})
				}
				return sys
			},
			func(sys *testSystem) istanbul.RoundChangeCertificate {
				view1 := *(sys.backends[0].engine.(*core).current.View())

				var view2 istanbul.View
				view2.Sequence = big.NewInt(view1.Sequence.Int64())
				view2.Round = big.NewInt(view1.Round.Int64() + 1)

				preparedCertificate := sys.getPreparedCertificate(t, []istanbul.View{view1, view2}, makeBlock(2))
				roundChangeCertificate := sys.getRoundChangeCertificate(t, *(sys.backends[0].engine.(*core).current.View()), preparedCertificate)
				return roundChangeCertificate
			},
			makeBlock(2),
			errInvalidPreparedCertificateInconsistentViews,
			false,
		},
		{
			"ROUND CHANGE certificate contains PREPARED certificate for a different block.",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for _, backend := range sys.backends {
					backend.engine.(*core).Start()
					c := backend.engine.(*core)
					getRoundState(c).state = StatePreprepared
					getRoundState(c).round = big.NewInt(1)
					c.current.TransitionToPreprepared(&istanbul.Preprepare{
						View: &istanbul.View{
							Round:    big.NewInt(1),
							Sequence: big.NewInt(0),
						},
						Proposal:               makeBlock(2),
						RoundChangeCertificate: istanbul.RoundChangeCertificate{},
					})
				}
				return sys
			},
			func(sys *testSystem) istanbul.RoundChangeCertificate {
				preparedCertificate := sys.getPreparedCertificate(t, []istanbul.View{*(sys.backends[0].engine.(*core).current.View())}, makeBlock(2))
				roundChangeCertificate := sys.getRoundChangeCertificate(t, *(sys.backends[0].engine.(*core).current.View()), preparedCertificate)
				return roundChangeCertificate
			},
			makeBlock(1),
			errInvalidPreparedCertificateDigestMismatch,
			false,
		},
		{
			"ROUND CHANGE certificate for N+1 round with valid PREPARED certificates",
			// Round is N+1 to match the correct proposer.
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					backend.engine.(*core).Start()
					c := backend.engine.(*core)
					getRoundState(c).round = big.NewInt(int64(N))
					if i != 0 {
						getRoundState(c).state = StateAcceptRequest
					}
				}
				return sys
			},
			func(sys *testSystem) istanbul.RoundChangeCertificate {
				preparedCertificate := sys.getPreparedCertificate(t, []istanbul.View{*(sys.backends[0].engine.(*core).current.View())}, makeBlock(1))
				roundChangeCertificate := sys.getRoundChangeCertificate(t, *(sys.backends[0].engine.(*core).current.View()), preparedCertificate)
				return roundChangeCertificate
			},
			makeBlock(1),
			nil,
			false,
		},
		{
			"ROUND CHANGE certificate for N+1 round with empty PREPARED certificates",
			// Round is N+1 to match the correct proposer.
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					backend.engine.(*core).Start()
					c := backend.engine.(*core)
					getRoundState(c).round = big.NewInt(int64(N))
					if i != 0 {
						getRoundState(c).state = StateAcceptRequest
					}
				}
				return sys
			},
			func(sys *testSystem) istanbul.RoundChangeCertificate {
				roundChangeCertificate := sys.getRoundChangeCertificate(t, *(sys.backends[0].engine.(*core).current.View()), istanbul.EmptyPreparedCertificate())
				return roundChangeCertificate
			},
			makeBlock(1),
			nil,
			false,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {

			t.Log("Running", "test", test.name)
			sys := test.system()
			closer := sys.Run(false)

			v0 := sys.backends[0]
			r0 := v0.engine.(*core)

			curView := r0.current.View()

			preprepareView := curView
			if test.existingBlock {
				preprepareView = &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(5)}
			}

			preprepare := &istanbul.Preprepare{
				View:                   preprepareView,
				Proposal:               test.expectedRequest,
				RoundChangeCertificate: test.getCert(sys),
			}

			for i, v := range sys.backends {
				// i == 0 is primary backend, it is responsible for send PRE-PREPARE messages to others.
				if i == 0 {
					continue
				}

				c := v.engine.(*core)

				m, _ := Encode(preprepare)
				// run each backends and verify handlePreprepare function.
				if err := c.handlePreprepare(&istanbul.Message{
					Code:    istanbul.MsgPreprepare,
					Msg:     m,
					Address: v0.Address(),
				}); err != nil {
					if err != test.expectedErr {
						t.Errorf("error mismatch: have %v, want %v", err, test.expectedErr)
					}
					return
				}

				if c.current.State() != StatePreprepared {
					t.Errorf("state mismatch: have %v, want %v", c.current.State(), StatePreprepared)
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
				var committedSubject *istanbul.CommittedSubject

				if decodedMsg.Code == istanbul.MsgPrepare {
					err = decodedMsg.Decode(&subject)
				} else if decodedMsg.Code == istanbul.MsgCommit {
					err = decodedMsg.Decode(&committedSubject)
					subject = committedSubject.Subject
				}

				if err != nil {
					t.Errorf("error mismatch: have %v, want nil", err)
				}

				expectedSubject := c.current.Subject()
				if test.existingBlock {
					expectedSubject = &istanbul.Subject{View: &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(5)},
						Digest: test.expectedRequest.Hash()}
				}

				if !reflect.DeepEqual(subject, expectedSubject) {
					t.Errorf("subject mismatch: have %v, want %v", subject, expectedSubject)
				}

				if expectedCode == istanbul.MsgCommit {
					srcValidator := c.current.GetValidatorByAddress(v.address)

					if err := c.verifyCommittedSeal(committedSubject, srcValidator); err != nil {
						t.Errorf("invalid seal.  verify commmited seal error: %v, subject: %v, committedSeal: %v", err, expectedSubject, committedSubject.CommittedSeal)
					}
				}
			}

			closer()
		})
	}
}
