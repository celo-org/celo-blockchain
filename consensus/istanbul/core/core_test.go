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
	"errors"
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-bls-go/bls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	elog "github.com/celo-org/celo-blockchain/log"
)

func makeBlock(number int64) *types.Block {
	header := &types.Header{
		Number:  big.NewInt(number),
		GasUsed: 0,
		Time:    uint64(0),
	}
	return types.NewBlock(header, nil, nil, nil)
}

func newTestProposalWithNum(num int64) istanbul.Proposal {
	return makeBlock(num)
}

func newTestProposal() istanbul.Proposal {
	return makeBlock(1)
}

var InvalidProposalError = errors.New("invalid proposal")

func TestNewRequest(t *testing.T) {
	testLogger.SetHandler(elog.StdoutHandler)

	N := uint64(4)
	F := uint64(1)

	sys := NewTestSystemWithBackend(N, F)

	close := sys.Run(true)
	defer close()

	request1 := makeBlock(1)
	sys.backends[0].NewRequest(request1)

	<-time.After(1 * time.Second)

	request2 := makeBlock(2)
	sys.backends[0].NewRequest(request2)

	<-time.After(1 * time.Second)

	for _, backend := range sys.backends {
		if len(backend.committedMsgs) != 2 {
			t.Errorf("the number of executed requests mismatch: have %v, want 2", len(backend.committedMsgs))
		} else {
			if !reflect.DeepEqual(request1.Number(), backend.committedMsgs[0].commitProposal.Number()) {
				t.Errorf("the number of requests mismatch: have %v, want %v", request1.Number(), backend.committedMsgs[0].commitProposal.Number())
			}
			if !reflect.DeepEqual(request2.Number(), backend.committedMsgs[1].commitProposal.Number()) {
				t.Errorf("the number of requests mismatch: have %v, want %v", request2.Number(), backend.committedMsgs[1].commitProposal.Number())
			}
		}
	}
}

func TestVerifyProposal(t *testing.T) {
	// Check that it should not be in the cache
	sys := NewTestSystemWithBackend(1, 0)

	close := sys.Run(true)
	defer close()

	backendCore := sys.backends[0].engine.(*core)
	backend := backendCore.backend.(*testSystemBackend)

	testCases := []struct {
		name             string
		proposal         istanbul.Proposal
		verifyImpl       func(proposal istanbul.Proposal) (time.Duration, error)
		expectedErr      error
		expectedDuration time.Duration
	}{
		// Test case with valid proposal
		{
			"Valid proposal",
			newTestProposalWithNum(1),
			backend.verifyWithSuccess,
			nil,
			0,
		},

		// Test case with invalid proposal
		{
			"Invalid proposal",
			newTestProposalWithNum(2),
			backend.verifyWithFailure,
			InvalidProposalError,
			0,
		},

		// Test case with future proposal
		{
			"Future proposal",
			newTestProposalWithNum(3),
			backend.verifyWithFutureProposal,
			consensus.ErrFutureBlock,
			5,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name,
			func(t *testing.T) {
				// Inject in the verification function implementation
				backend.setVerifyImpl(testCase.verifyImpl)

				// Verify a cache miss
				_, isCached := backendCore.current.GetProposalVerificationStatus(testCase.proposal.Hash())
				if isCached {
					t.Errorf("Should of had a cache miss")
				}

				// Do a verification with success
				_, err := backendCore.verifyProposal(testCase.proposal)
				if err != testCase.expectedErr {
					t.Errorf("Unexpected return status on first verifyProposal call. Want: %v, Actual: %v", testCase.expectedErr, err)
				}

				// The cache entry for this proposal should be created, if it wasn't the future proposal case
				err, isCached = backendCore.current.GetProposalVerificationStatus(testCase.proposal.Hash())
				if testCase.name != "Future proposal" {
					if !isCached {
						t.Errorf("Should of had a cache hit")
					}

					if err != testCase.expectedErr {
						t.Errorf("Unexpected cached proposal verification status. Want: %v, actual: %v", testCase.expectedErr, err)
					}
				} else { // testCase.name == "Future proposal"
					if isCached {
						t.Errorf("Should of had a cache miss for the future proposal test case")
					}
				}

				// Call verify proposal again to check for the cached verifcation result and duration
				duration, err := backendCore.verifyProposal(testCase.proposal)
				if duration != testCase.expectedDuration || err != testCase.expectedErr {
					t.Errorf("Unexpected return status on second verifyProposal call.  Want: err - %v, duration - %v; Actual: err - %v, duration - %v", testCase.expectedErr, testCase.expectedDuration, err, duration)
				}
			})
	}
}

func TestEpochSnarkData(t *testing.T) {
	N := uint64(4)
	F := uint64(1)

	sys := NewTestSystemWithBackendDonut(N, F, 1, 2)

	close := sys.Run(true)
	defer close()

	request1 := makeBlock(1)
	sys.backends[0].NewRequest(request1)

	<-time.After(1 * time.Second)

	request2 := makeBlock(2)
	sys.backends[0].NewRequest(request2)

	<-time.After(1 * time.Second)

	backendCore := sys.backends[0].engine.(*core)
	privateKey, _ := bls.DeserializePrivateKey(sys.backends[0].blsKey)
	defer privateKey.Destroy()

	serializedPrivateKey, _ := privateKey.Serialize()

	publicKey, _ := blscrypto.PrivateToPublic(serializedPrivateKey)

	epochSeal, err := backendCore.generateEpochValidatorSetData(0, 0, common.Hash{}, sys.backends[0].Validators(backendCore.current.Proposal()))
	require.NoError(t, err)
	if epochSeal.Cip22 || len(epochSeal.ExtraData) > 0 {
		t.Errorf("Unexpected cip22 (%t != false) or extraData length (%v > 0)", epochSeal.Cip22, len(epochSeal.ExtraData))
	}
	epochSig, err := epochSeal.Sign(backendCore.backend.SignBLS)
	assert.NoError(t, err)
	err = epochSeal.Verify(publicKey, epochSig)
	assert.NoError(t, err)

	epochSealDonut, err := backendCore.generateEpochValidatorSetData(2, 0, common.Hash{}, sys.backends[0].Validators(backendCore.current.Proposal()))
	if !epochSealDonut.Cip22 || len(epochSealDonut.ExtraData) == 0 {
		t.Errorf("Unexpected cip22 (%t != true) or extraData length (%v == 0)", epochSealDonut.Cip22, len(epochSealDonut.ExtraData))
	}
	epochSig, err = epochSealDonut.Sign(backendCore.backend.SignBLS)
	assert.NoError(t, err)
	err = epochSealDonut.Verify(publicKey, epochSig)
	assert.NoError(t, err)
}
