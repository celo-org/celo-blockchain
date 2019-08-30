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

	"github.com/celo-org/bls-zexe/go"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/bls"
)

func TestHandleCommit(t *testing.T) {
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
		system      *testSystem
		expectedErr error
	}{
		{
			// normal case
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.valSet = backend.peers
					c.current = newTestRoundState(
						&istanbul.View{
							Round:    big.NewInt(0),
							Sequence: big.NewInt(1),
						},
						c.valSet,
					)

					if i == 0 {
						// replica 0 is the proposer
						c.state = StatePrepared
					}
				}
				return sys
			}(),
			nil,
		},
		{
			// future message
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.valSet = backend.peers
					if i == 0 {
						// replica 0 is the proposer
						c.current = newTestRoundState(
							expectedSubject.View,
							c.valSet,
						)
						c.state = StatePreprepared
					} else {
						c.current = newTestRoundState(
							&istanbul.View{
								Round:    big.NewInt(2),
								Sequence: big.NewInt(3),
							},
							c.valSet,
						)
					}
				}
				return sys
			}(),
			errFutureMessage,
		},
		{
			// subject not match
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.valSet = backend.peers
					if i == 0 {
						// replica 0 is the proposer
						c.current = newTestRoundState(
							expectedSubject.View,
							c.valSet,
						)
						c.state = StatePreprepared
					} else {
						c.current = newTestRoundState(
							&istanbul.View{
								Round:    big.NewInt(0),
								Sequence: big.NewInt(0),
							},
							c.valSet,
						)
					}
				}
				return sys
			}(),
			errOldMessage,
		},
		{
			// jump state
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.valSet = backend.peers
					c.current = newTestRoundState(
						&istanbul.View{
							Round:    big.NewInt(0),
							Sequence: proposal.Number(),
						},
						c.valSet,
					)

					// only replica0 stays at StatePreprepared
					// other replicas are at StatePrepared
					if i != 0 {
						c.state = StatePrepared
					} else {
						c.state = StatePreprepared
					}
				}
				return sys
			}(),
			nil,
		},
		// TODO: double send message
	}

OUTER:
	for _, test := range testCases {
		test.system.Run(false)

		v0 := test.system.backends[0]
		r0 := v0.engine.(*core)

		for i, v := range test.system.backends {
			validator := r0.valSet.GetByIndex(uint64(i))
			privateKey, _ := bls.DeserializePrivateKey(test.system.validatorsKeys[i])
			defer privateKey.Destroy()

			hash := PrepareCommittedSeal(v.engine.(*core).current.Proposal().Hash())
			signature, _ := privateKey.SignMessage(hash, []byte{}, false)
			defer signature.Destroy()
			signatureBytes, _ := signature.Serialize()
			m, _ := Encode(v.engine.(*core).current.Subject())
			if err := r0.handleCommit(&istanbul.Message{
				Code:          istanbul.MsgCommit,
				Msg:           m,
				Address:       validator.Address(),
				Signature:     []byte{},
				CommittedSeal: signatureBytes,
			}); err != nil {
				if err != test.expectedErr {
					t.Errorf("error mismatch: have %v, want %v", err, test.expectedErr)
				}
				continue OUTER
			}
		}

		// prepared is normal case
		if r0.state != StateCommitted {
			// There are not enough commit messages in core
			if r0.state != StatePrepared {
				t.Errorf("state mismatch: have %v, want %v", r0.state, StatePrepared)
			}
			if r0.current.Commits.Size() > r0.valSet.MinQuorumSize() {
				t.Errorf("the size of commit messages should be less than %v", r0.valSet.MinQuorumSize())
			}
			continue
		}

		// core should have min quorum size prepare messages
		if r0.current.Commits.Size() < r0.valSet.MinQuorumSize() {
			t.Errorf("the size of commit messages should be greater than or equal to minQuorumSize: size %v", r0.current.Commits.Size())
		}

		// check signatures large than MinQuorumSize
		signedCount := 0
		for i := 0; i < r0.valSet.Size(); i++ {
			if v0.committedMsgs[0].bitmap.Bit(i) == 1 {
				signedCount++
			}
		}
		if signedCount < r0.valSet.MinQuorumSize() {
			t.Errorf("the expected signed count should be greater than or equal to %v, but got %v", r0.valSet.MinQuorumSize(), signedCount)
		}
	}
}

// round is not checked for now
func TestVerifyCommit(t *testing.T) {
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
	}, istanbul.RoundRobin)

	sys := NewTestSystemWithBackend(uint64(1), uint64(0))

	testCases := []struct {
		expected   error
		commit     *istanbul.Subject
		roundState *roundState
	}{
		{
			// normal case
			expected: nil,
			commit: &istanbul.Subject{
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
			commit: &istanbul.Subject{
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
			commit: &istanbul.Subject{
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
			commit: &istanbul.Subject{
				View:   &istanbul.View{Round: big.NewInt(0), Sequence: nil},
				Digest: newTestProposal().Hash(),
			},
			roundState: newTestRoundState(
				&istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(1)},
				valSet,
			),
		},
		{
			// wrong prepare message with same sequence but different round
			expected: errInconsistentSubject,
			commit: &istanbul.Subject{
				View:   &istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(0)},
				Digest: newTestProposal().Hash(),
			},
			roundState: newTestRoundState(
				&istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
				valSet,
			),
		},
		{
			// wrong prepare message with same round but different sequence
			expected: errInconsistentSubject,
			commit: &istanbul.Subject{
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

		if err := c.verifyCommit(test.commit); err != nil {
			if err != test.expected {
				t.Errorf("result %d: error mismatch: have %v, want %v", i, err, test.expected)
			}
		}
	}
}
