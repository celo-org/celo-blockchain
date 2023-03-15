package core

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"

	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/stretchr/testify/assert"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
)

func mockViewMsg(view *istanbul.View, code uint64, addr common.Address) *istanbul.Message {
	var msg []byte = []byte{}
	subj := &istanbul.Subject{
		View:   view,
		Digest: common.BytesToHash([]byte("1234567890")),
	}
	if code == istanbul.MsgPrepare {
		msg, _ = rlp.EncodeToBytes(subj)
	}
	if code == istanbul.MsgCommit {
		cs := &istanbul.CommittedSubject{
			Subject:               subj,
			CommittedSeal:         []byte("commSeal"),
			EpochValidatorSetSeal: []byte("epochVS"),
		}
		msg, _ = rlp.EncodeToBytes(cs)
	}
	return &istanbul.Message{
		Code:      code,
		Address:   addr,
		Msg:       msg,
		Signature: []byte("sigg"),
	}
}

func TestRSDBRoundStateDB(t *testing.T) {
	pubkey1 := blscrypto.SerializedPublicKey{1, 2, 3}
	pubkey2 := blscrypto.SerializedPublicKey{3, 1, 4}
	dummyRoundState := func() RoundState {
		valSet := validator.NewSet([]istanbul.ValidatorData{
			{Address: common.BytesToAddress([]byte(string(rune(2)))), BLSPublicKey: pubkey1},
			{Address: common.BytesToAddress([]byte(string(rune(4)))), BLSPublicKey: pubkey2},
		})
		view := newView(2, 1)
		rs := newRoundState(view, valSet, valSet.GetByIndex(0))
		rs.AddPrepare(mockViewMsg(view, istanbul.MsgPrepare, valSet.GetByIndex(0).Address()))
		rs.AddCommit(mockViewMsg(view, istanbul.MsgPrepare, valSet.GetByIndex(0).Address()))
		rs.AddParentCommit(mockViewMsg(view, istanbul.MsgPrepare, valSet.GetByIndex(0).Address()))
		return rs
	}

	t.Run("Should save view & roundState", func(t *testing.T) {
		rsdb, _ := newRoundStateDB("", &RoundStateDBOptions{withGarbageCollector: false})
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

	t.Run("Should save rcvd messages", func(t *testing.T) {
		rsdb, _ := newRoundStateDB("", &RoundStateDBOptions{withGarbageCollector: false})
		rs := dummyRoundState()
		rs.Prepares().Add(
			mockViewMsg(
				rs.View(), istanbul.MsgPrepare, rs.ValidatorSet().GetByIndex(1).Address(),
			),
		)

		// prepares: 2(0, 1), commits: 1(0), parentCommits: 1(0)
		assert.Nil(t, rsdb.UpdateLastRoundState(rs))

		// Delete messages to be reconstructed from rcvd
		// Prepares will have 1 less
		// Commits will have 1 more
		// ParentCommits will be equal as before
		// save the msg for comparing later
		prepRemoved := rs.Prepares().Values()[0]
		rs.Prepares().Remove(prepRemoved.Address)
		rs.Commits().Add(mockViewMsg(rs.View(), istanbul.MsgCommit, rs.ValidatorSet().GetByIndex(1).Address()))

		err := rsdb.UpdateLastRcvd(rs)
		finishOnError(t, err)

		view, err := rsdb.GetLastView()
		finishOnError(t, err)
		assertEqualView(t, view, rs.View())

		savedRs, err := rsdb.GetRoundStateFor(view)
		assert.NoError(t, err)
		assert.NotNil(t, savedRs)
		finishOnError(t, err)

		// ReAdd to the original RoundState
		rs.Prepares().Add(prepRemoved)
		// Commits should both have the new amount
		assertEqualRoundState(t, savedRs, rs)

		// Add one more ParentCommit
		newPc := mockViewMsg(view, istanbul.MsgCommit, rs.ValidatorSet().GetByIndex(1).Address())
		rs.ParentCommits().Add(newPc)

		err = rsdb.UpdateLastRcvd(rs)
		finishOnError(t, err)

		savedRs2, err := rsdb.GetRoundStateFor(view)
		assert.NoError(t, err)
		assert.NotNil(t, savedRs2)
		finishOnError(t, err)

		assertEqualRoundState(t, savedRs2, rs)
	})

	t.Run("Should save view from last saved roundState", func(t *testing.T) {
		rsdb, _ := newRoundStateDB("", &RoundStateDBOptions{withGarbageCollector: false})
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

func TestRSDBDeleteEntriesOlderThan(t *testing.T) {
	pubkey1 := blscrypto.SerializedPublicKey{1, 2, 3}
	pubkey2 := blscrypto.SerializedPublicKey{3, 1, 4}
	createRoundState := func(view *istanbul.View) RoundState {
		valSet := validator.NewSet([]istanbul.ValidatorData{
			{Address: common.BytesToAddress([]byte(string(rune(2)))), BLSPublicKey: pubkey1},
			{Address: common.BytesToAddress([]byte(string(rune(4)))), BLSPublicKey: pubkey2},
		})
		return newRoundState(view, valSet, valSet.GetByIndex(0))
	}

	rsdb, _ := newRoundStateDB("", &RoundStateDBOptions{withGarbageCollector: false})
	for seq := uint64(1); seq <= 10; seq++ {
		for r := uint64(0); r < 10; r++ {
			rs := createRoundState(newView(seq, r))
			err := rsdb.UpdateLastRoundState(rs)
			finishOnError(t, err)
		}
	}

	// Will delete all entries from seq 1
	count, err := rsdb.(*roundStateDBImpl).deleteEntriesOlderThan(newView(2, 0))
	if err != nil {
		t.Fatalf("Error %v", err)
	}
	if count != 10 {
		t.Fatalf("Expected 10 deleted entries but got %d", count)
	}

	// Will delete all entries from seq 2,3 and seq 4 until round 5
	count, err = rsdb.(*roundStateDBImpl).deleteEntriesOlderThan(newView(4, 5))
	if err != nil {
		t.Fatalf("Error %v", err)
	}
	if count != 25 {
		t.Fatalf("Expected 10 deleted entries but got %d", count)
	}

}

func TestRcvdSerialization(t *testing.T) {
	valSet := newTestValidatorSet(0)
	r := rcvd{
		Prepares:      newMessageSet(valSet),
		Commits:       newMessageSet(valSet),
		ParentCommits: newMessageSet(valSet),
	}
	rRLP, err := r.ToRLP()
	assert.NoError(t, err)
	bytes, err := rlp.EncodeToBytes(rRLP)
	assert.NoError(t, err)
	var r2RLP *rcvdRLP = &rcvdRLP{}
	err = rlp.DecodeBytes(bytes, r2RLP)
	assert.NoError(t, err)
	var r2 rcvd
	err = r2.FromRLP(r2RLP)
	assert.NoError(t, err)
	assert.Equal(t, r, r2)
}

func TestRSDBKeyEncodingOrder(t *testing.T) {
	iterations := 1000

	t.Run("ViewKey encoding should decode the same view", func(t *testing.T) {
		for i := 0; i < iterations; i++ {
			view := newView(rand.Uint64(), rand.Uint64())
			key := view2Key(view)
			parsedView := key2View(key)
			if view.Cmp(parsedView) != 0 {
				t.Errorf("parsedView != view: %v != %v", parsedView, view)
			}
		}
	})

	t.Run("ViewKey encoding should maintain sort order", func(t *testing.T) {
		for i := 0; i < iterations; i++ {
			viewA := newView(rand.Uint64(), rand.Uint64())
			keyA := view2Key(viewA)

			viewB := newView(rand.Uint64(), rand.Uint64())
			keyB := view2Key(viewB)

			if viewA.Cmp(viewB) != bytes.Compare(keyA, keyB) {
				t.Errorf("view order != key order (viewA: %v, viewB: %v, keyA:%v, keyB:%v )",
					viewA,
					viewB,
					hex.EncodeToString(keyA),
					hex.EncodeToString(keyB),
				)

			}
		}
	})

	t.Run("RcvdViewKey encoding should maintain sort order", func(t *testing.T) {
		for i := 0; i < iterations; i++ {
			viewA := newView(rand.Uint64(), rand.Uint64())
			keyA := rcvdView2Key(viewA)

			viewB := newView(rand.Uint64(), rand.Uint64())
			keyB := rcvdView2Key(viewB)

			if viewA.Cmp(viewB) != bytes.Compare(keyA, keyB) {
				t.Errorf("view order != key order (viewA: %v, viewB: %v, keyA:%v, keyB:%v )",
					viewA,
					viewB,
					hex.EncodeToString(keyA),
					hex.EncodeToString(keyB),
				)

			}
		}
	})
}

func TestRSDBGetOldestValidView(t *testing.T) {
	pubkey1 := blscrypto.SerializedPublicKey{1, 2, 3}
	pubkey2 := blscrypto.SerializedPublicKey{3, 1, 4}
	valSet := validator.NewSet([]istanbul.ValidatorData{
		{Address: common.BytesToAddress([]byte(string(rune(2)))), BLSPublicKey: pubkey1},
		{Address: common.BytesToAddress([]byte(string(rune(4)))), BLSPublicKey: pubkey2},
	})
	sequencesToSave := uint64(100)
	runTestCase := func(name string, viewToStore, expectedView *istanbul.View) {
		t.Run(name, func(t *testing.T) {
			rsdb, _ := newRoundStateDB("", &RoundStateDBOptions{
				withGarbageCollector: false,
				sequencesToSave:      sequencesToSave,
			})

			if viewToStore != nil {
				t.Logf("Saving RoundState")
				err := rsdb.UpdateLastRoundState(newRoundState(viewToStore, valSet, valSet.GetByIndex(0)))
				if err != nil {
					t.Fatalf("UpdateLastRoundState error: %v", err)
				}
			}

			view, err := rsdb.GetOldestValidView()
			if err != nil {
				t.Fatalf("GetOldestValidView error: %v", err)
			}
			if view.Cmp(expectedView) != 0 {
				t.Errorf("Expected %v, got %v", expectedView, view)
			}
		})
	}

	runTestCase("When Nothing Stored", nil, newView(0, 0))
	runTestCase("When StoredSequence < sequencesToSave", newView(sequencesToSave-1, 90), newView(0, 0))
	runTestCase("When StoredSequence == sequencesToSave", newView(sequencesToSave, 90), newView(0, 0))
	runTestCase("When StoredSequence > sequencesToSave", newView(sequencesToSave+1, 90), newView(1, 0))
	runTestCase("When StoredSequence >> sequencesToSave", newView(sequencesToSave+1000, 90), newView(1000, 0))
}
