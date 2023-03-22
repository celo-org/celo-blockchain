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
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/crypto"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-bls-go/bls"
)

func TestHandleCommit(t *testing.T) {
	N := uint64(4)
	F := uint64(1)

	// create block 4
	proposal := newTestProposalWithNum(4)
	expectedSubject := &istanbul.Subject{
		View: &istanbul.View{
			Round:    big.NewInt(0),
			Sequence: proposal.Number(),
		},
		Digest: proposal.Hash(),
	}

	testCases := []struct {
		system             *testSystem
		expectedErr        error
		checkParentCommits bool
	}{
		{
			// normal case
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					// same view as the expected one to everyone
					c.current = newTestRoundStateV2(
						expectedSubject.View,
						backend.peers,
					)

					if i == 0 {
						// replica 0 is the proposer
						c.current.(*roundStateImpl).state = StatePrepared
					}
				}
				return sys
			}(),
			nil,
			false,
		},
		{
			// future message
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					if i == 0 {
						// replica 0 is the proposer
						c.current = newTestRoundStateV2(
							expectedSubject.View,
							backend.peers,
						)
						c.current.(*roundStateImpl).state = StatePreprepared
					} else {
						c.current = newTestRoundStateV2(
							&istanbul.View{
								Round: big.NewInt(0),
								// proposal from 1 round in the future
								Sequence: big.NewInt(0).Add(proposal.Number(), common.Big1),
							},
							backend.peers,
						)
					}
				}
				return sys
			}(),
			errFutureMessage,
			false,
		},
		{
			// past message
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)

					if i == 0 {
						// replica 0 is the proposer
						c.current = newTestRoundStateV2(
							expectedSubject.View,
							backend.peers,
						)
						c.current.(*roundStateImpl).state = StatePreprepared
					} else {
						c.current = newTestRoundStateV2(
							&istanbul.View{
								Round: big.NewInt(0),
								// we're 2 blocks before so this is indeed a
								// very old proposal and will error as expected
								// with an old error message
								Sequence: big.NewInt(0).Sub(proposal.Number(), common.Big2),
							},
							backend.peers,
						)
					}
				}
				return sys
			}(),
			errOldMessage,
			false,
		},
		{
			// jump state
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					c := backend.engine.(*core)
					c.current = newTestRoundStateV2(
						&istanbul.View{
							Round:    big.NewInt(0),
							Sequence: proposal.Number(),
						},
						backend.peers,
					)

					// only replica0 stays at StatePreprepared
					// other replicas are at StatePrepared
					if i != 0 {
						c.current.(*roundStateImpl).state = StatePrepared
					} else {
						c.current.(*roundStateImpl).state = StatePreprepared
					}
				}
				return sys
			}(),
			nil,
			false,
		},
		{
			// message from previous sequence and round matching last proposal
			// this should pass the message check, but will return an error in
			// handleCheckedCommitForPreviousSequence, because the proposal hashes won't match.
			func() *testSystem {
				sys := NewTestSystemWithBackend(N, F)

				for i, backend := range sys.backends {
					backend.Commit(newTestProposalWithNum(3), types.IstanbulAggregatedSeal{}, types.IstanbulEpochValidatorSetSeal{}, nil)
					c := backend.engine.(*core)
					if i == 0 {
						// replica 0 is the proposer
						c.current = newTestRoundStateV2(
							expectedSubject.View,
							backend.peers,
						)
						c.current.(*roundStateImpl).state = StatePrepared
					} else {
						c.current = newTestRoundStateV2(
							&istanbul.View{
								Round:    big.NewInt(1),
								Sequence: big.NewInt(0).Sub(proposal.Number(), common.Big1),
							},
							backend.peers,
						)
					}
				}
				return sys
			}(),
			errInconsistentSubject,
			true,
		},
		// TODO: double send message
	}

OUTER:
	for _, test := range testCases {
		test.system.Run(false)

		v0 := test.system.backends[0]
		r0 := v0.engine.(*core)

		for i, v := range test.system.backends {
			validator := r0.current.ValidatorSet().GetByIndex(uint64(i))
			privateKey, _ := bls.DeserializePrivateKey(test.system.validatorsKeys[i])
			defer privateKey.Destroy()

			hash := PrepareCommittedSeal(v.engine.(*core).current.Proposal().Hash(), v.engine.(*core).current.Round())
			signature, _ := privateKey.SignMessage(hash, []byte{}, false, false)
			defer signature.Destroy()
			signatureBytes, _ := signature.Serialize()

			msg := istanbul.NewCommitMessage(
				&istanbul.CommittedSubject{Subject: v.engine.(*core).current.Subject(), CommittedSeal: signatureBytes},
				validator.Address(),
			)

			if err := r0.handleCommit(msg); err != nil {
				if err != test.expectedErr {
					t.Errorf("error mismatch: have %v, want %v", err, test.expectedErr)
				}
				continue OUTER
			}
		}

		// core should have received a parent seal from each of its neighbours
		// how can we add our signature to the ParentCommit? Broadcast to ourselve
		// does not make much sense
		if test.checkParentCommits {
			if r0.current.ParentCommits().Size() != r0.current.ValidatorSet().Size()-1 { // TODO: Maybe remove the -1?
				t.Errorf("parent seals mismatch: have %v, want %v", r0.current.ParentCommits().Size(), r0.current.ValidatorSet().Size()-1)
			}
		}

		// prepared is normal case
		if r0.current.State() != StateCommitted {
			// There are not enough commit messages in core
			if r0.current.State() != StatePrepared {
				t.Errorf("state mismatch: have %v, want %v", r0.current.State(), StatePrepared)
			}
			if r0.current.Commits().Size() > r0.current.ValidatorSet().MinQuorumSize() {
				t.Errorf("the size of commit messages should be less than %v", r0.current.ValidatorSet().MinQuorumSize())
			}
			continue
		}

		// core should have min quorum size prepare messages
		if r0.current.Commits().Size() < r0.current.ValidatorSet().MinQuorumSize() {
			t.Errorf("the size of commit messages should be greater than or equal to minQuorumSize: size %v", r0.current.Commits().Size())
		}

		// check signatures large than MinQuorumSize
		signedCount := 0
		for i := 0; i < r0.current.ValidatorSet().Size(); i++ {
			if v0.committedMsgs[0].aggregatedSeal.Bitmap.Bit(i) == 1 {
				signedCount++
			}
		}
		if signedCount < r0.current.ValidatorSet().MinQuorumSize() {
			t.Errorf("the expected signed count should be greater than or equal to %v, but got %v", r0.current.ValidatorSet().MinQuorumSize(), signedCount)
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
			Address:      peer.Address(),
			BLSPublicKey: blsPublicKey,
		},
	})
	// }, istanbul.RoundRobin)

	sys := NewTestSystemWithBackend(uint64(1), uint64(0))

	testCases := []struct {
		expected   error
		commit     *istanbul.CommittedSubject
		roundState RoundState
	}{
		{
			// normal case
			expected: nil,
			commit: &istanbul.CommittedSubject{
				Subject: &istanbul.Subject{
					View:   &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
					Digest: newTestProposal().Hash(),
				},
			},
			roundState: newTestRoundStateV2(
				&istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
				valSet,
			),
		},
		{
			// old message
			expected: errInconsistentSubject,
			commit: &istanbul.CommittedSubject{
				Subject: &istanbul.Subject{
					View:   &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
					Digest: newTestProposal().Hash(),
				},
			},
			roundState: newTestRoundStateV2(
				&istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(1)},
				valSet,
			),
		},
		{
			// different digest
			expected: errInconsistentSubject,
			commit: &istanbul.CommittedSubject{
				Subject: &istanbul.Subject{
					View:   &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
					Digest: common.BytesToHash([]byte("1234567890")),
				},
			},
			roundState: newTestRoundStateV2(
				&istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(1)},
				valSet,
			),
		},
		{
			// malicious package(lack of sequence)
			expected: errInconsistentSubject,
			commit: &istanbul.CommittedSubject{
				Subject: &istanbul.Subject{
					View:   &istanbul.View{Round: big.NewInt(0), Sequence: nil},
					Digest: newTestProposal().Hash(),
				},
			},
			roundState: newTestRoundStateV2(
				&istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(1)},
				valSet,
			),
		},
		{
			// wrong prepare message with same sequence but different round
			expected: errInconsistentSubject,
			commit: &istanbul.CommittedSubject{
				Subject: &istanbul.Subject{
					View:   &istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(0)},
					Digest: newTestProposal().Hash(),
				},
			},
			roundState: newTestRoundStateV2(
				&istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
				valSet,
			),
		},
		{
			// wrong prepare message with same round but different sequence
			expected: errInconsistentSubject,
			commit: &istanbul.CommittedSubject{
				Subject: &istanbul.Subject{
					View:   &istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(1)},
					Digest: newTestProposal().Hash(),
				},
			},
			roundState: newTestRoundStateV2(
				&istanbul.View{Round: big.NewInt(0), Sequence: big.NewInt(0)},
				valSet,
			),
		},
	}
	for i, test := range testCases {
		c := sys.backends[0].engine.(*core)
		c.current = test.roundState

		if err := c.verifyCommit(test.commit); err != test.expected {
			t.Errorf("result %d: error mismatch: have %v, want %v", i, err, test.expected)
		}
	}
}

// BenchmarkHandleCommit benchmarks handling a commit message
func BenchmarkHandleCommit(b *testing.B) {
	N := uint64(2)
	F := uint64(1) // F does not affect tests

	sys := NewMutedTestSystemWithBackend(N, F)
	// sys := NewTestSystemWithBackend(N, F)

	// create block 4
	proposal := newTestProposalWithNum(4)
	expectedSubject := &istanbul.Subject{
		View: &istanbul.View{
			Round:    big.NewInt(0),
			Sequence: proposal.Number(),
		},
		Digest: proposal.Hash(),
	}

	for i, backend := range sys.backends {
		c := backend.engine.(*core)
		// same view as the expected one to everyone
		c.current = newTestRoundStateV2(
			expectedSubject.View,
			backend.peers,
		)

		if i == 0 {
			// replica 0 is the proposer
			c.current.(*roundStateImpl).state = StatePrepared
		}
	}

	sys.Run(false)

	v0 := sys.backends[0]
	r0 := v0.engine.(*core)

	var im *istanbul.Message
	for i, v := range sys.backends {
		validator := r0.current.ValidatorSet().GetByIndex(uint64(i))
		privateKey, _ := bls.DeserializePrivateKey(sys.validatorsKeys[i])
		defer privateKey.Destroy()

		hash := PrepareCommittedSeal(v.engine.(*core).current.Proposal().Hash(), v.engine.(*core).current.Round())
		signature, _ := privateKey.SignMessage(hash, []byte{}, false, false)
		defer signature.Destroy()
		signatureBytes, _ := signature.Serialize()
		im = istanbul.NewCommitMessage(&istanbul.CommittedSubject{
			Subject:       v.engine.(*core).current.Subject(),
			CommittedSeal: signatureBytes,
		}, validator.Address())
	}
	// benchmarked portion
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := r0.handleCommit(im)
		if err != nil {
			b.Errorf("Error handling the pre-prepare message. err: %v", err)
		}
	}
}
