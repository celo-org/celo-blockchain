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

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/algorithm"
	"github.com/stretchr/testify/require"
)

func TestVerifyPreparedCertificate(t *testing.T) {
	N := uint64(4) // replica 0 is the proposer, it will send messages to others
	F := uint64(1)
	sys := NewTestSystemWithBackend(N, F)
	view := istanbul.View{
		Round:    big.NewInt(0),
		Sequence: big.NewInt(1),
	}
	proposal := makeBlock(0)

	for _, b := range sys.backends {
		b.engine.Start() // start Istanbul core
	}

	testCases := []struct {
		name         string
		certificate  istanbul.PreparedCertificate
		expectedErr  error
		expectedView *istanbul.View
	}{
		{
			"Valid PREPARED certificate",
			sys.getPreparedCertificate(t, []istanbul.View{view}, proposal),
			nil,
			&view,
		},
		{
			"Invalid PREPARED certificate, duplicate message",
			func() istanbul.PreparedCertificate {
				preparedCertificate := sys.getPreparedCertificate(t, []istanbul.View{view}, proposal)
				preparedCertificate.PrepareOrCommitMessages[1] = preparedCertificate.PrepareOrCommitMessages[0]
				return preparedCertificate
			}(),
			errInvalidPreparedCertificateDuplicate,
			nil,
		},
		{
			"Invalid PREPARED certificate, future message",
			func() istanbul.PreparedCertificate {
				futureView := istanbul.View{
					Round:    big.NewInt(0),
					Sequence: big.NewInt(10),
				}
				preparedCertificate := sys.getPreparedCertificate(t, []istanbul.View{futureView}, proposal)
				return preparedCertificate
			}(),
			errInvalidPreparedCertificateMsgView,
			nil,
		},
		{
			"Invalid PREPARED certificate, includes preprepare message",
			func() istanbul.PreparedCertificate {
				preparedCertificate := sys.getPreparedCertificate(t, []istanbul.View{view}, proposal)
				testInvalidMsg, _ := sys.backends[0].getRoundChangeMessage(view, sys.getPreparedCertificate(t, []istanbul.View{view}, proposal))
				preparedCertificate.PrepareOrCommitMessages[0] = testInvalidMsg
				return preparedCertificate
			}(),
			errInvalidPreparedCertificateMsgCode,
			nil,
		},
		{
			"Invalid PREPARED certificate, hash mismatch",
			func() istanbul.PreparedCertificate {
				preparedCertificate := sys.getPreparedCertificate(t, []istanbul.View{view}, proposal)
				preparedCertificate.PrepareOrCommitMessages[1] = preparedCertificate.PrepareOrCommitMessages[0]
				preparedCertificate.Proposal = makeBlock(1)
				return preparedCertificate
			}(),
			errInvalidPreparedCertificateDigestMismatch,
			nil,
		},
		{
			"Invalid PREPARED certificate, view inconsistencies",
			func() istanbul.PreparedCertificate {
				var view2 istanbul.View
				view2.Sequence = big.NewInt(view.Sequence.Int64())
				view2.Round = big.NewInt(view.Round.Int64() + 1)
				preparedCertificate := sys.getPreparedCertificate(t, []istanbul.View{view, view2}, proposal)
				return preparedCertificate
			}(),
			errInvalidPreparedCertificateInconsistentViews,
			nil,
		},
		{
			"Empty certificate",
			istanbul.EmptyPreparedCertificate(),
			errInvalidPreparedCertificateNumMsgs,
			nil,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			for _, backend := range sys.backends {
				c := backend.engine.(*core)
				var expectedRound uint64
				if test.expectedView != nil {
					expectedRound = test.expectedView.Round.Uint64()
				}
				view, err := c.verifyPreparedCertificate(test.certificate, expectedRound)
				if err != test.expectedErr {
					t.Errorf("error mismatch: have %v, want %v", err, test.expectedErr)
				}
				if err == nil {
					if view.Cmp(test.expectedView) != 0 {
						t.Errorf("view mismatch: have %v, want %v", view, test.expectedView)
					}
					view, err := c.getViewFromVerifiedPreparedCertificate(test.certificate)
					if err != nil {
						t.Errorf("error mismatch: have %v, want nil", err)
					}
					if view.Cmp(test.expectedView) != 0 {
						t.Errorf("view mismatch: have %v, want %v", view, test.expectedView)
					}
				}
			}
		})
	}
}

func TestHandlePrepare(t *testing.T) {
	N := uint64(4)
	F := uint64(1)

	proposal := newTestProposal()
	expectedSubject := &istanbul.Subject{
		View: &istanbul.View{
			Round:    big.NewInt(0),
			Sequence: proposal.Number(),
		},
		Digest: proposal.Hash(),
	}

	testCases := []struct {
		name        string
		system      *testSystem
		expectedErr error
	}{
		{
			"normal case",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)

					c.current = newTestRoundState(
						&istanbul.View{
							Round:    big.NewInt(0),
							Sequence: big.NewInt(1),
						},
						backend.peers,
					)

					if i == 0 {
						// replica 0 is the proposer
						c.current.(*roundStateImpl).state = StatePreprepared
					}
					c.algo = algorithm.NewAlgorithm(NewRoundStateOracle(c.current, c))
				}
				return sys
			}(),
			nil,
		},
		{
			"normal case with prepared certificate",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)
				preparedCert := sys.getPreparedCertificate(
					t,
					[]istanbul.View{
						{
							Round:    big.NewInt(0),
							Sequence: big.NewInt(1),
						},
					},
					proposal)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.current = newTestRoundState(
						&istanbul.View{
							Round:    big.NewInt(0),
							Sequence: big.NewInt(1),
						},
						backend.peers,
					)
					c.current.(*roundStateImpl).preparedCertificate = preparedCert

					if i == 0 {
						// replica 0 is the proposer
						c.current.(*roundStateImpl).state = StatePreprepared
					}
					c.algo = algorithm.NewAlgorithm(NewRoundStateOracle(c.current, c))
				}
				return sys
			}(),
			nil,
		},
		{
			"Inconsistent subject due to prepared certificate",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)
				preparedCert := sys.getPreparedCertificate(
					t,
					[]istanbul.View{
						{
							Round:    big.NewInt(0),
							Sequence: big.NewInt(10),
						},
					},
					proposal)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.current = newTestRoundState(
						&istanbul.View{
							Round:    big.NewInt(0),
							Sequence: big.NewInt(1),
						},
						backend.peers,
					)
					c.current.(*roundStateImpl).preparedCertificate = preparedCert

					if i == 0 {
						// replica 0 is the proposer
						c.current.(*roundStateImpl).state = StatePreprepared
					}
					c.algo = algorithm.NewAlgorithm(NewRoundStateOracle(c.current, c))
				}
				return sys
			}(),
			errInconsistentSubject,
		},
		{
			"less than 2F+1",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				// save less than 2*F+1 replica
				sys.backends = sys.backends[2*int(F)+1:]

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.current = newTestRoundState(
						expectedSubject.View,
						backend.peers,
					)

					if i == 0 {
						// replica 0 is the proposer
						c.current.(*roundStateImpl).state = StatePreprepared
					}
					c.algo = algorithm.NewAlgorithm(NewRoundStateOracle(c.current, c))
				}
				return sys
			}(),
			nil,
		},
		// TODO: double send message
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {

			test.system.Run(false)

			v0 := test.system.backends[0]
			r0 := v0.engine.(*core)

			for _, v := range test.system.backends {
				msg := istanbul.NewPrepareMessage(v.engine.(*core).current.Subject(), v.Address())
				err := msg.Sign(v.Sign)
				require.NoError(t, err)
				payload, err := msg.Payload()
				require.NoError(t, err)
				err = r0.handleMsg(payload)
				if err != nil {
					if err != test.expectedErr {
						t.Errorf("error mismatch: have %v, want %v", err, test.expectedErr)
					}
					return
				}
			}

			// prepared is normal case
			if r0.current.State() != StatePrepared {
				// There are not enough PREPARE messages in core
				if r0.current.State() != StatePreprepared {
					t.Errorf("state mismatch: have %v, want %v", r0.current.State(), StatePreprepared)
				}
				if r0.current.Prepares().Size() >= r0.current.ValidatorSet().MinQuorumSize() {
					t.Errorf("the size of PREPARE messages should be less than %v", 2*r0.current.ValidatorSet().MinQuorumSize()+1)
				}

				return
			}

			// core should have MinQuorumSize PREPARE messages
			if r0.current.Prepares().Size() < r0.current.ValidatorSet().MinQuorumSize() {
				t.Errorf("the size of PREPARE messages should be greater than or equal to MinQuorumSize: size %v", r0.current.Prepares().Size())
			}

			// a message will be delivered to backend if 2F+1
			if int64(len(v0.sentMsgs)) != 1 {
				t.Errorf("the Send() should be called once: times %v", len(test.system.backends[0].sentMsgs))
			}

			// verify COMMIT messages
			decodedMsg := new(istanbul.Message)
			err := decodedMsg.FromPayload(v0.sentMsgs[0], nil)
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}

			if decodedMsg.Code != istanbul.MsgCommit {
				t.Errorf("message code mismatch: have %v, want %v", decodedMsg.Code, istanbul.MsgCommit)
			}
			subject := decodedMsg.Commit().Subject
			if !reflect.DeepEqual(subject, expectedSubject) {
				t.Errorf("subject mismatch: have %v, want %v", subject, expectedSubject)
			}
		})
	}
}

// benchMarkHandlePrepare benchmarks handling a prepare message
func BenchmarkHandlePrepare(b *testing.B) {
	N := uint64(2)
	F := uint64(1) // F does not affect tests

	sys := NewMutedTestSystemWithBackend(N, F)

	for i, backend := range sys.backends {
		c := backend.engine.(*core)
		// Start core so that algorithm.Algorithm is created
		err := c.Start()
		require.NoError(b, err)

		c.current = newTestRoundState(
			&istanbul.View{
				Round:    big.NewInt(0),
				Sequence: big.NewInt(1),
			},
			backend.peers,
		)

		if i == 0 {
			// replica 0 is the proposer
			c.current.(*roundStateImpl).state = StatePreprepared
		}
	}

	shutdown := sys.Run(false)
	defer shutdown()

	v0 := sys.backends[0]
	c := v0.engine.(*core)
	m, _ := Encode(v0.engine.(*core).current.Subject())
	msg := istanbul.Message{
		Code:    istanbul.MsgPrepare,
		Msg:     m,
		Address: v0.Address(),
	}
	err := msg.Sign(v0.Sign)
	require.NoError(b, err)
	payload, err := msg.Payload()
	require.NoError(b, err)

	// benchmarked portion
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := c.handleMsg(payload)
		if err != nil {
			b.Errorf("Error handling the pre-prepare message. err: %v", err)
		}
	}
}
