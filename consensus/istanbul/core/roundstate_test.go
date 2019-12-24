package core

import (
	"math/big"
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
