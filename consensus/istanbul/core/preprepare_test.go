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

	testCases := []struct {
		system          *testSystem
		getCert         func(*testSystem) istanbul.RoundChangeCertificate
		expectedRequest istanbul.Proposal
		expectedErr     error
		existingBlock   bool
	}{
		{
			// normal case
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.valSet = backend.peers
					if i != 0 {
						c.state = StateAcceptRequest
					}
				}
				return sys
			}(),
			func(_ *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
			},
			newTestProposal(),
			nil,
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
			func(_ *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
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
			func(_ *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
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
			func(_ *testSystem) istanbul.RoundChangeCertificate {
				return istanbul.RoundChangeCertificate{}
			},
			makeBlock(1),
			errMissingRoundChangeCertificate,
			false,
		},
		{
			// ROUND CHANGE certificate invalid, duplicate messages.
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
			func(sys *testSystem) istanbul.RoundChangeCertificate {
				// Duplicate messages
				roundChangeCertificate := sys.getRoundChangeCertificate(t, *(sys.backends[0].engine.(*core).currentView()), istanbul.EmptyPreparedCertificate())
				roundChangeCertificate.RoundChangeMessages[1] = roundChangeCertificate.RoundChangeMessages[0]
				return roundChangeCertificate
			},
			makeBlock(1),
			errInvalidRoundChangeCertificateDuplicate,
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
					c.current.SetPreprepare(&istanbul.Preprepare{
						View: &istanbul.View{
							Round:    big.NewInt(1),
							Sequence: big.NewInt(0),
						},
						Proposal:               makeBlock(2),
						RoundChangeCertificate: istanbul.RoundChangeCertificate{},
					})
				}
				return sys
			}(),
			func(sys *testSystem) istanbul.RoundChangeCertificate {
				preparedCertificate := sys.getPreparedCertificate(t, *(sys.backends[0].engine.(*core).currentView()), makeBlock(2))
				roundChangeCertificate := sys.getRoundChangeCertificate(t, *(sys.backends[0].engine.(*core).currentView()), preparedCertificate)
				return roundChangeCertificate
			},
			makeBlock(1),
			errInvalidProposal,
			false,
		},
		{
			// ROUND CHANGE certificate for second round with valid PREPARED certificates
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.valSet = backend.peers
					c.current.SetRound(big.NewInt(1))
					if i != 0 {
						c.state = StateAcceptRequest
					}
				}
				return sys
			}(),
			func(sys *testSystem) istanbul.RoundChangeCertificate {
				preparedCertificate := sys.getPreparedCertificate(t, *(sys.backends[0].engine.(*core).currentView()), makeBlock(0))
				roundChangeCertificate := sys.getRoundChangeCertificate(t, *(sys.backends[0].engine.(*core).currentView()), preparedCertificate)
				return roundChangeCertificate
			},
			makeBlock(0),
			nil,
			false,
		},
		{
			// ROUND CHANGE certificate for second round with empty PREPARED certificates
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.valSet = backend.peers
					c.current.SetRound(big.NewInt(1))
					if i != 0 {
						c.state = StateAcceptRequest
					}
				}
				return sys
			}(),
			func(sys *testSystem) istanbul.RoundChangeCertificate {
				roundChangeCertificate := sys.getRoundChangeCertificate(t, *(sys.backends[0].engine.(*core).currentView()), istanbul.EmptyPreparedCertificate())
				return roundChangeCertificate
			},
			makeBlock(1),
			nil,
			false,
		},
	}

OUTER:
	for i, test := range testCases {
		testLogger.Info("Running handle preprepare test case", "number", i)
		test.system.Run(false)

		v0 := test.system.backends[0]
		r0 := v0.engine.(*core)

		curView := r0.currentView()

		preprepare := &istanbul.Preprepare{
			View:                   curView,
			Proposal:               test.expectedRequest,
			RoundChangeCertificate: test.getCert(test.system),
		}

		for i, v := range test.system.backends {
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
				continue OUTER
			}

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

		}
	}
}
