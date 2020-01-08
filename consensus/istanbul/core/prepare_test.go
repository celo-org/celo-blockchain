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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/crypto"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
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
				view, err := c.verifyPreparedCertificate(test.certificate)
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
				}
				return sys
			}(),
			errInconsistentSubject,
		},
		{
			"future message",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					if i == 0 {
						// replica 0 is the proposer
						c.current = newTestRoundState(
							expectedSubject.View,
							backend.peers,
						)
						c.current.(*roundStateImpl).state = StatePreprepared
					} else {
						c.current = newTestRoundState(
							&istanbul.View{
								Round:    big.NewInt(2),
								Sequence: big.NewInt(3),
							},
							backend.peers,
						)
					}
				}
				return sys
			}(),
			errFutureMessage,
		},
		{
			"subject not match",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					if i == 0 {
						// replica 0 is the proposer
						c.current = newTestRoundState(
							expectedSubject.View,
							backend.peers,
						)
						c.current.(*roundStateImpl).state = StatePreprepared
					} else {
						c.current = newTestRoundState(
							&istanbul.View{
								Round:    big.NewInt(0),
								Sequence: big.NewInt(0),
							},
							backend.peers,
						)
					}
				}
				return sys
			}(),
			errOldMessage,
		},
		{
			"subject not match",
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					if i == 0 {
						// replica 0 is the proposer
						c.current = newTestRoundState(
							expectedSubject.View,
							backend.peers,
						)
						c.current.(*roundStateImpl).state = StatePreprepared
					} else {
						c.current = newTestRoundState(
							&istanbul.View{
								Round:    big.NewInt(0),
								Sequence: big.NewInt(1)},
							backend.peers,
						)
					}
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

			for i, v := range test.system.backends {
				validator := r0.current.ValidatorSet().GetByIndex(uint64(i))
				m, _ := Encode(v.engine.(*core).current.Subject())
				err := r0.handlePrepare(&istanbul.Message{
					Code:    istanbul.MsgPrepare,
					Msg:     m,
					Address: validator.Address(),
				})
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
			var m *istanbul.CommittedSubject
			err = decodedMsg.Decode(&m)
			if err != nil {
				t.Errorf("error mismatch: have %v, want nil", err)
			}
			if !reflect.DeepEqual(m.Subject, expectedSubject) {
				t.Errorf("subject mismatch: have %v, want %v", m, expectedSubject)
			}
		})
	}
}

// round is not checked for now
func TestVerifyPrepare(t *testing.T) {

	// for log purpose
	privateKey, _ := crypto.GenerateKey()
	blsPrivateKey, _ := blscrypto.ECDSAToBLS(privateKey)
	blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
	peer := validator.New(getPublicKeyAddress(privateKey), blsPublicKey)
	valSet := validator.NewSet([]istanbul.ValidatorData{
		{
			peer.Address(),
			blsPublicKey,
		},
	})

	sys := NewTestSystemWithBackend(uint64(1), uint64(0))

	testCases := []struct {
		expected error

		prepare    *istanbul.Subject
		roundState RoundState
	}{
		{
			// normal case
			expected: nil,
			prepare: &istanbul.Subject{
				View:   &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
				Digest: newTestProposal().Hash(),
			},
			roundState: newTestRoundState(
				&istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
				valSet,
			),
		},
		{
			// old message
			expected: errInconsistentSubject,
			prepare: &istanbul.Subject{
				View:   &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
				Digest: newTestProposal().Hash(),
			},
			roundState: newTestRoundState(
				&istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(1)},
				valSet,
			),
		},
		{
			// different digest
			expected: errInconsistentSubject,
			prepare: &istanbul.Subject{
				View:   &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
				Digest: common.BytesToHash([]byte("1234567890")),
			},
			roundState: newTestRoundState(
				&istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(1)},
				valSet,
			),
		},
		{
			// malicious package(lack of sequence)
			expected: errInconsistentSubject,
			prepare: &istanbul.Subject{
				View:   &istanbul.View{Round: big.NewInt(0), Sequence: nil},
				Digest: newTestProposal().Hash(),
			},
			roundState: newTestRoundState(
				&istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(1)},
				valSet,
			),
		},
		{
			// wrong PREPARE message with same sequence but different round
			expected: errInconsistentSubject,
			prepare: &istanbul.Subject{
				View:   &istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(0)},
				Digest: newTestProposal().Hash(),
			},
			roundState: newTestRoundState(
				&istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
				valSet,
			),
		},
		{
			// wrong PREPARE message with same round but different sequence
			expected: errInconsistentSubject,
			prepare: &istanbul.Subject{
				View:   &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(1)},
				Digest: newTestProposal().Hash(),
			},
			roundState: newTestRoundState(
				&istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
				valSet,
			),
		},
	}
	for i, test := range testCases {
		c := sys.backends[0].engine.(*core)
		c.current = test.roundState

		if err := c.verifyPrepare(test.prepare); err != nil {
			if err != test.expected {
				t.Errorf("result %d: error mismatch: have %v, want %v", i, err, test.expected)
			}
		}
	}
}
