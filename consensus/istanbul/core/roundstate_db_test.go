package core

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
)

func TestRoundStateDB(t *testing.T) {
	dummyRoundState := func() RoundState {
		valSet := validator.NewSet([]istanbul.ValidatorData{
			{Address: common.BytesToAddress([]byte(string(2))), BLSPublicKey: []byte{1, 2, 3}},
			{Address: common.BytesToAddress([]byte(string(4))), BLSPublicKey: []byte{3, 1, 4}},
		})
		return newRoundState(newView(2, 1), valSet, valSet.GetByIndex(0))
	}

	t.Run("Should save view & roundState", func(t *testing.T) {
		rsdb, _ := newRoundStateDB("")
		rs := dummyRoundState()
		err := rsdb.UpdateLastRoundState(rs)
		finishOnError(t, err)

		view, err := rsdb.GetLastView()
		finishOnError(t, err)
		assertEqualView(t, view, rs.View())

		savedRs, err := rsdb.GetRoundStateFor(view)
		finishOnError(t, err)
		assertEqualRoundState(t, savedRs, rs)
	})

	t.Run("Should save view from last saved roundState", func(t *testing.T) {
		rsdb, _ := newRoundStateDB("")
		rs := dummyRoundState()
		err := rsdb.UpdateLastRoundState(rs)
		finishOnError(t, err)
		rs.StartNewSequence(common.Big32, rs.ValidatorSet(), rs.ValidatorSet().GetByIndex(1), rs.ParentCommits())
		err = rsdb.UpdateLastRoundState(rs)
		finishOnError(t, err)

		view, err := rsdb.GetLastView()
		finishOnError(t, err)
		assertEqualView(t, view, rs.View())
	})

}
