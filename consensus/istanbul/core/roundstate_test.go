package core

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestRoundStateRLPEncoding(t *testing.T) {
	dummyRoundState := func() RoundState {
		valSet := validator.NewSet([]istanbul.ValidatorData{
			{Address: common.BytesToAddress([]byte(string(2))), BLSPublicKey: []byte{1, 2, 3}},
			{Address: common.BytesToAddress([]byte(string(4))), BLSPublicKey: []byte{3, 1, 4}},
		})
		view := &istanbul.View{Round: big.NewInt(1), Sequence: big.NewInt(2)}
		return newRoundState(view, valSet, valSet.GetByIndex(0))
	}

	t.Run("With nil fields", func(t *testing.T) {
		rs := dummyRoundState()

		rawVal, err := rlp.EncodeToBytes(rs)
		if err != nil {
			t.Errorf("Error %v", err)
		}

		var result *roundStateImpl
		if err = rlp.DecodeBytes(rawVal, &result); err != nil {
			t.Errorf("Error %v", err)
		}

		assertEqualRoundState(t, rs, result)
	})

	t.Run("With a Pending Request", func(t *testing.T) {
		rs := dummyRoundState()
		rs.SetPendingRequest(&istanbul.Request{
			Proposal: makeBlock(1),
		})

		rawVal, err := rlp.EncodeToBytes(rs)
		if err != nil {
			t.Errorf("Error %v", err)
		}

		var result *roundStateImpl
		if err = rlp.DecodeBytes(rawVal, &result); err != nil {
			t.Errorf("Error %v", err)
		}

		assertEqualRoundState(t, rs, result)
	})

	t.Run("With a Preprepare", func(t *testing.T) {
		rs := dummyRoundState()

		rs.TransitionToPreprepared(&istanbul.Preprepare{
			Proposal:               makeBlock(1),
			View:                   rs.View(),
			RoundChangeCertificate: istanbul.RoundChangeCertificate{},
		})

		rawVal, err := rlp.EncodeToBytes(rs)
		if err != nil {
			t.Errorf("Error %v", err)
		}

		var result *roundStateImpl
		if err = rlp.DecodeBytes(rawVal, &result); err != nil {
			t.Errorf("Error %v", err)
		}

		assertEqualRoundState(t, rs, result)
	})

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

// func TestBlockRLPEncoding(t *testing.T) {
// 	block := makeBlock(1)

// 	raw, err := rlp.EncodeToBytes(block)
// 	if err != nil {
// 		t.Errorf("Error %v", err)
// 	}

// 	var result *types.Block
// 	if err = rlp.DecodeBytes(raw, &result); err != nil {
// 		t.Errorf("Error %v", err)
// 	}

// 	if !reflect.DeepEqual(block.Hash(), result.Hash()) {
// 		t.Errorf("Block.Hash() mismatch: have %v, want %v", block.Hash(), result.Hash())
// 	} else {
// 		t.Log("Block.Hash() matches")
// 	}

// 	if !reflect.DeepEqual(block, result) {
// 		t.Errorf("Block mismatch: have %v, want %v", block, result)
// 	}
// }
