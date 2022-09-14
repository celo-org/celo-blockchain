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
)

func newView(seq, round uint64) *istanbul.View {
	return &istanbul.View{Round: new(big.Int).SetUint64(round), Sequence: new(big.Int).SetUint64(seq)}
}

func newTestRoundState(view *istanbul.View, validatorSet istanbul.ValidatorSet) RoundState {
	current := newRoundState(view, validatorSet, validatorSet.GetByIndex(0), false)
	current.(*roundStateImpl).preprepare = newTestPreprepare(view)
	return current
}

func newTestRoundStateV2(view *istanbul.View, validatorSet istanbul.ValidatorSet) RoundState {
	current := newRoundState(view, validatorSet, validatorSet.GetByIndex(0), false)
	current.(*roundStateImpl).preprepareV2 = newTestPreprepareV2(view)
	return current
}

func finishOnError(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Error %v", err)
	}
}

func assertEqualView(t *testing.T, have, want *istanbul.View) {
	if !reflect.DeepEqual(have, want) {
		t.Errorf("View are not equal: have %v, want: %v", have, want)
	}
}
func assertEqualRoundState(t *testing.T, have, want RoundState) {
	testEqual := func(name string, have, want interface{}) {
		if !reflect.DeepEqual(have, want) {
			t.Errorf("RoundState.%s mismatch: have %v, want %v", name, have, want)
		}
	}

	testEqual("State", have.State(), want.State())
	testEqual("Round", have.Round(), want.Round())
	testEqual("DesiredRound", have.DesiredRound(), want.DesiredRound())
	testEqual("Sequence", have.Sequence(), want.Sequence())
	testEqual("ValidatorSet", have.ValidatorSet(), want.ValidatorSet())
	testEqual("Proposer", have.Proposer(), want.Proposer())
	testEqual("ParentCommits", have.ParentCommits(), want.ParentCommits())
	testEqual("Commits", have.Commits(), want.Commits())
	testEqual("Prepares", have.Prepares(), want.Prepares())

	if have.PendingRequest() == nil || want.PendingRequest() == nil {
		testEqual("PendingRequest", have.PendingRequest(), want.PendingRequest())
	} else {
		haveBlock := have.PendingRequest().Proposal
		wantBlock := want.PendingRequest().Proposal
		testEqual("PendingRequest.Proposal.Hash", haveBlock.Hash(), wantBlock.Hash())
	}

	if have.Preprepare() == nil || want.Preprepare() == nil {
		testEqual("Preprepare", have.Preprepare(), want.Preprepare())
	} else {
		testEqual("Preprepare.Proposal.Hash", have.Preprepare().Proposal.Hash(), want.Preprepare().Proposal.Hash())
		testEqual("Preprepare.View", have.Preprepare().View, want.Preprepare().View)
		testEqual("Preprepare.RoundChangeCertificate.IsEmpty", have.Preprepare().RoundChangeCertificate.IsEmpty(), want.Preprepare().RoundChangeCertificate.IsEmpty())

		if !have.Preprepare().RoundChangeCertificate.IsEmpty() && !want.Preprepare().RoundChangeCertificate.IsEmpty() {
			testEqual("Preprepare.RoundChangeCertificate.RoundChangeMessages", have.Preprepare().RoundChangeCertificate.RoundChangeMessages, want.Preprepare().RoundChangeCertificate.RoundChangeMessages)
		}

	}

	havePPBlock := have.PreparedCertificate().Proposal
	wantPPBlock := want.PreparedCertificate().Proposal
	testEqual("PreparedCertificate().Proposal.Hash", havePPBlock.Hash(), wantPPBlock.Hash())
	testEqual("PreparedCertificate().PrepareOrCommitMessages", have.PreparedCertificate().PrepareOrCommitMessages, want.PreparedCertificate().PrepareOrCommitMessages)
}
