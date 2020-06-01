package core

import (
	"encoding/json"
	"math/big"
	"reflect"
	"sort"
	"strings"
	"testing"

	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestRoundStateRLPEncoding(t *testing.T) {
	dummyRoundState := func() RoundState {
		valSet := validator.NewSet([]istanbul.ValidatorData{
			{Address: common.HexToAddress("2"), BLSPublicKey: blscrypto.SerializedPublicKey{1, 2, 3}},
			{Address: common.HexToAddress("4"), BLSPublicKey: blscrypto.SerializedPublicKey{3, 1, 4}},
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

func TestRoundStateSummary(t *testing.T) {
	view := &istanbul.View{Round: big.NewInt(2), Sequence: big.NewInt(2)}

	validatorAddresses := []common.Address{
		common.HexToAddress("1"),
		common.HexToAddress("2"),
		common.HexToAddress("3"),
		common.HexToAddress("4"),
		common.HexToAddress("5"),
		common.HexToAddress("6"),
	}

	dummyRoundState := func() RoundState {

		valData := make([]istanbul.ValidatorData, len(validatorAddresses))
		for i, addr := range validatorAddresses {
			valData[i] = istanbul.ValidatorData{Address: addr, BLSPublicKey: blscrypto.SerializedPublicKey{1, 2, 3}}
		}
		valSet := validator.NewSet(valData)

		rs := newRoundState(view, valSet, valSet.GetByIndex(0))

		// Add a few prepares
		rs.AddPrepare(&istanbul.Message{
			Code:    istanbul.MsgPrepare,
			Address: validatorAddresses[1],
		})
		rs.AddPrepare(&istanbul.Message{
			Code:    istanbul.MsgPrepare,
			Address: validatorAddresses[2],
		})
		rs.AddPrepare(&istanbul.Message{
			Code:    istanbul.MsgPrepare,
			Address: validatorAddresses[3],
		})
		rs.AddPrepare(&istanbul.Message{
			Code:    istanbul.MsgPrepare,
			Address: validatorAddresses[4],
		})

		// Add a few commits
		rs.AddCommit(&istanbul.Message{
			Code:    istanbul.MsgCommit,
			Address: validatorAddresses[1],
		})
		rs.AddCommit(&istanbul.Message{
			Code:    istanbul.MsgCommit,
			Address: validatorAddresses[2],
		})
		rs.AddCommit(&istanbul.Message{
			Code:    istanbul.MsgCommit,
			Address: validatorAddresses[3],
		})

		// Add a few parent commits
		rs.AddParentCommit(&istanbul.Message{
			Code:    istanbul.MsgCommit,
			Address: validatorAddresses[3],
		})
		rs.AddParentCommit(&istanbul.Message{
			Code:    istanbul.MsgCommit,
			Address: validatorAddresses[4],
		})
		rs.AddParentCommit(&istanbul.Message{
			Code:    istanbul.MsgCommit,
			Address: validatorAddresses[5],
		})

		return rs
	}

	assertEqualAddressSet := func(t *testing.T, name string, got, expected []common.Address) {
		gotStrings := make([]string, len(got))
		for i, addr := range got {
			gotStrings[i] = addr.Hex()
		}

		expectedStrings := make([]string, len(expected))
		for i, addr := range expected {
			expectedStrings[i] = addr.Hex()
		}

		sort.StringSlice(expectedStrings).Sort()
		sort.StringSlice(gotStrings).Sort()

		if !reflect.DeepEqual(expectedStrings, gotStrings) {
			t.Errorf("%s: Got %v expected %v", name, gotStrings, expectedStrings)
		}
	}

	t.Run("With nil fields", func(t *testing.T) {
		rs := dummyRoundState()
		rsSummary := rs.Summary()

		if strings.Compare(rsSummary.State, rs.State().String()) != 0 {
			t.Errorf("State: Mismatch got %v expected %v", rsSummary.State, rs.State().String())
		}

		if rsSummary.Sequence.Cmp(rs.Sequence()) != 0 {
			t.Errorf("Sequence: Mismatch got %v expected %v", rsSummary.Sequence, rs.Sequence())
		}
		if rsSummary.Round.Cmp(rs.Round()) != 0 {
			t.Errorf("Round: Mismatch got %v expected %v", rsSummary.Round, rs.Round())
		}
		if rsSummary.DesiredRound.Cmp(rs.DesiredRound()) != 0 {
			t.Errorf("DesiredRound: Mismatch got %v expected %v", rsSummary.DesiredRound, rs.DesiredRound())
		}

		if rsSummary.PendingRequestHash != nil {
			t.Errorf("PendingRequestHash: Mismatch got %v expected %v", rsSummary.PendingRequestHash, nil)
		}

		if !reflect.DeepEqual(rsSummary.ValidatorSet, validatorAddresses) {
			t.Errorf("ValidatorSet: Mismatch got %v expected %v", rsSummary.ValidatorSet, validatorAddresses)
		}

		if !reflect.DeepEqual(rsSummary.Proposer, validatorAddresses[0]) {
			t.Errorf("Proposer: Mismatch got %v expected %v", rsSummary.Proposer, validatorAddresses[0])
		}

		assertEqualAddressSet(t, "Prepares", rsSummary.Prepares, validatorAddresses[1:5])
		assertEqualAddressSet(t, "Commits", rsSummary.Commits, validatorAddresses[1:4])
		assertEqualAddressSet(t, "ParentCommits", rsSummary.ParentCommits, validatorAddresses[3:6])

		if rsSummary.Preprepare != nil {
			t.Errorf("Preprepare: Mismatch got %v expected %v", rsSummary.Preprepare, nil)
		}

		if rsSummary.PreparedCertificate != nil {
			t.Errorf("PreparedCertificate: Mismatch got %v expected %v", rsSummary.PreparedCertificate, nil)
		}

		_, err := json.Marshal(rsSummary)
		if err != nil {
			t.Errorf("Error %v", err)
		}
	})

	t.Run("With a Pending Request", func(t *testing.T) {
		rs := dummyRoundState()
		block := makeBlock(1)
		rs.SetPendingRequest(&istanbul.Request{
			Proposal: block,
		})

		rsSummary := rs.Summary()

		if rsSummary.PendingRequestHash == nil || !reflect.DeepEqual(*rsSummary.PendingRequestHash, block.Hash()) {
			t.Errorf("PendingRequestHash: Mismatch got %v expected %v", rsSummary.PendingRequestHash, block.Hash())
		}

		_, err := json.Marshal(rsSummary)
		if err != nil {
			t.Errorf("Error %v", err)
		}
	})

	t.Run("With a Preprepare", func(t *testing.T) {
		rs := dummyRoundState()
		block := makeBlock(1)
		preprepare := &istanbul.Preprepare{
			Proposal: block,
			View:     rs.View(),
			RoundChangeCertificate: istanbul.RoundChangeCertificate{
				RoundChangeMessages: []istanbul.Message{
					{Code: istanbul.MsgRoundChange, Address: validatorAddresses[3]},
				},
			},
		}

		rs.TransitionToPreprepared(preprepare)

		rsSummary := rs.Summary()

		if rsSummary.Preprepare == nil {
			t.Fatalf("Got nil Preprepare")
		}
		if !reflect.DeepEqual(rsSummary.Preprepare.View, rs.View()) {
			t.Errorf("Preprepare.View: Mismatch got %v expected %v", rsSummary.Preprepare.View, rs.View())
		}
		if !reflect.DeepEqual(rsSummary.Preprepare.ProposalHash, block.Hash()) {
			t.Errorf("Preprepare.ProposalHash: Mismatch got %v expected %v", rsSummary.Preprepare.ProposalHash, block.Hash())
		}

		assertEqualAddressSet(t, "Preprepare.RoundChangeCertificateSenders", rsSummary.Preprepare.RoundChangeCertificateSenders, validatorAddresses[3:4])

		_, err := json.Marshal(rsSummary)
		if err != nil {
			t.Errorf("Error %v", err)
		}
	})

}
