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

package validator

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/crypto"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
)

var (
	testAddress  = "70524d664ffe731100208a0154e556f9bb679ae6"
	testAddress2 = "b37866a925bccd69cfa98d43b510f1d23d78a851"
)

func TestValidatorSet(t *testing.T) {
	t.Run("NewValidatorSet", testNewValidatorSet)
	t.Run("NormalValSet", testNormalValSet)
	t.Run("EmptyValSet", testEmptyValSet)
	t.Run("AddAndRemoveValidator", testAddAndRemoveValidator)
	t.Run("QuorumSizes", testQuorumSizes)
}

func testNewValidatorSet(t *testing.T) {
	const ValCnt = 100

	// Create 100 validators with random addresses
	b := []byte{}
	for i := 0; i < ValCnt; i++ {
		key, _ := crypto.GenerateKey()
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(key)
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		addr := crypto.PubkeyToAddress(key.PublicKey)
		val := New(addr, blsPublicKey)
		b = append(b, val.Address().Bytes()...)
		b = append(b, blsPublicKey[:]...)
	}

	// Create ValidatorSet
	valSet := NewSet(ExtractValidators(b))
	if valSet == nil {
		t.Errorf("the validator byte array cannot be parsed")
		t.FailNow()
	}
}

func testNormalValSet(t *testing.T) {
	b1 := common.Hex2Bytes(testAddress)
	b2 := common.Hex2Bytes(testAddress2)
	addr1 := common.BytesToAddress(b1)
	addr2 := common.BytesToAddress(b2)
	val1 := New(addr1, blscrypto.SerializedPublicKey{})
	val2 := New(addr2, blscrypto.SerializedPublicKey{})

	validators, _ := istanbul.CombineIstanbulExtraToValidatorData([]common.Address{addr1, addr2}, []blscrypto.SerializedPublicKey{{}, {}})
	valSet := newDefaultSet(validators)
	if valSet == nil {
		t.Errorf("the format of validator set is invalid")
		t.FailNow()
	}

	// check size
	if size := valSet.Size(); size != 2 {
		t.Errorf("the size of validator set is wrong: have %v, want 2", size)
	}
	// test get by index
	if val := valSet.GetByIndex(uint64(0)); !reflect.DeepEqual(val, val1) {
		t.Errorf("validator mismatch: have %v, want %v", val, val1)
	}
	// test get by invalid index
	if val := valSet.GetByIndex(uint64(2)); val != nil {
		t.Errorf("validator mismatch: have %v, want nil", val)
	}
	// test get by address
	if _, val := valSet.GetByAddress(addr2); !reflect.DeepEqual(val, val2) {
		t.Errorf("validator mismatch: have %v, want %v", val, val2)
	}
	// test get by invalid address
	invalidAddr := common.HexToAddress("0x9535b2e7faaba5288511d89341d94a38063a349b")
	if _, val := valSet.GetByAddress(invalidAddr); val != nil {
		t.Errorf("validator mismatch: have %v, want nil", val)
	}
}

func testEmptyValSet(t *testing.T) {
	valSet := NewSet(ExtractValidators([]byte{}))
	if valSet == nil {
		t.Errorf("validator set should not be nil")
	}
}

func testAddAndRemoveValidator(t *testing.T) {
	valSet := NewSet(ExtractValidators([]byte{}))
	if !valSet.AddValidators(
		[]istanbul.ValidatorData{
			{
				Address:      common.BytesToAddress([]byte(string(rune(3)))),
				BLSPublicKey: blscrypto.SerializedPublicKey{},
			},
		},
	) {
		t.Error("the validator should be added")
	}
	if valSet.AddValidators(
		[]istanbul.ValidatorData{
			{
				Address:      common.BytesToAddress([]byte(string(rune(3)))),
				BLSPublicKey: blscrypto.SerializedPublicKey{},
			},
		},
	) {
		t.Error("the existing validator should not be added")
	}
	valSet.AddValidators(
		[]istanbul.ValidatorData{
			{
				Address:      common.BytesToAddress([]byte(string(rune(2)))),
				BLSPublicKey: blscrypto.SerializedPublicKey{},
			},
			{
				Address:      common.BytesToAddress([]byte(string(rune(1)))),
				BLSPublicKey: blscrypto.SerializedPublicKey{},
			},
		},
	)
	if valSet.Size() != 3 {
		t.Error("the size of validator set should be 3")
	}

	expectedOrder := []int{3, 2, 1}
	for i, v := range valSet.List() {
		expected := common.BytesToAddress([]byte(string(rune(expectedOrder[i]))))
		if v.Address() != expected {
			t.Errorf("the order of validators is wrong: have %v, want %v", v.Address().Hex(), expected.Hex())
		}
	}

	if !valSet.RemoveValidators(big.NewInt(1)) { // remove first falidator
		t.Error("the validator should be removed")
	}

	if len(valSet.List()) != 2 || len(valSet.List()) != valSet.Size() { // validators set should have the same size
		t.Error("the size of validator set should be 2")
	}
	valSet.RemoveValidators(big.NewInt(2))                              // remove second validator
	if len(valSet.List()) != 1 || len(valSet.List()) != valSet.Size() { // validators set should have the same size
		t.Error("the size of validator set should be 1")
	}
	valSet.RemoveValidators(big.NewInt(1))                              // remove third validator
	if len(valSet.List()) != 0 || len(valSet.List()) != valSet.Size() { // validators set should have the same size
		t.Error("the size of validator set should be 0")
	}
}

func generateValidators(n int) ([]istanbul.ValidatorData, [][]byte) {
	vals := make([]istanbul.ValidatorData, 0)
	keys := make([][]byte, 0)
	for i := 0; i < n; i++ {
		privateKey, _ := crypto.GenerateKey()
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(privateKey)
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		vals = append(vals, istanbul.ValidatorData{
			Address:      crypto.PubkeyToAddress(privateKey.PublicKey),
			BLSPublicKey: blsPublicKey,
		})
		keys = append(keys, blsPrivateKey)
	}
	return vals, keys
}

func testQuorumSizes(t *testing.T) {
	testCases := []struct {
		validatorSetSize      int
		expectedMinQuorumSize int
	}{
		{validatorSetSize: 1, expectedMinQuorumSize: 1},
		{validatorSetSize: 2, expectedMinQuorumSize: 2},
		{validatorSetSize: 3, expectedMinQuorumSize: 2},
		{validatorSetSize: 4, expectedMinQuorumSize: 3},
		{validatorSetSize: 5, expectedMinQuorumSize: 4},
		{validatorSetSize: 6, expectedMinQuorumSize: 4},
		{validatorSetSize: 7, expectedMinQuorumSize: 5},
	}

	for _, testCase := range testCases {
		vals, _ := generateValidators(testCase.validatorSetSize)
		valSet := newDefaultSet(vals)

		if valSet.MinQuorumSize() != testCase.expectedMinQuorumSize {
			t.Errorf("error mismatch quorum size for valset of size %d: have %d, want %d", valSet.Size(), valSet.MinQuorumSize(), testCase.expectedMinQuorumSize)
		}
	}
}

func TestValidatorRLPEncoding(t *testing.T) {

	val := New(common.BytesToAddress([]byte(string(rune(2)))), blscrypto.SerializedPublicKey{1, 2, 3})

	rawVal, err := rlp.EncodeToBytes(val)
	if err != nil {
		t.Errorf("Error %v", err)
	}

	var result *defaultValidator
	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Errorf("Error %v", err)
	}

	if !reflect.DeepEqual(val, result) {
		t.Errorf("validator mismatch: have %v, want %v", val, result)
	}
}

func TestValidatorSetRLPEncoding(t *testing.T) {

	valSet := NewSet([]istanbul.ValidatorData{
		{Address: common.BytesToAddress([]byte(string(rune(2)))), BLSPublicKey: blscrypto.SerializedPublicKey{1, 2, 3}},
		{Address: common.BytesToAddress([]byte(string(rune(4)))), BLSPublicKey: blscrypto.SerializedPublicKey{3, 1, 4}},
	})

	rawVal, err := rlp.EncodeToBytes(valSet)
	if err != nil {
		t.Errorf("Error %v", err)
	}

	var result *defaultSet
	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Errorf("Error %v", err)
	}

	if !reflect.DeepEqual(valSet, result) {
		t.Errorf("validatorSet mismatch: have %v, want %v", valSet, result)
	}
}
