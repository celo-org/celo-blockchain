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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/bls"
)

var (
	testAddress  = "70524d664ffe731100208a0154e556f9bb679ae6"
	testAddress2 = "b37866a925bccd69cfa98d43b510f1d23d78a851"
)

func TestValidatorSet(t *testing.T) {
	testNewValidatorSet(t)
	testNormalValSet(t)
	testEmptyValSet(t)
	testStickyProposer(t)
	testAddAndRemoveValidator(t)
	testQuorumSizes(t)
}

func testNewValidatorSet(t *testing.T) {
	var validators []istanbul.Validator
	const ValCnt = 100

	// Create 100 validators with random addresses
	b := []byte{}
	for i := 0; i < ValCnt; i++ {
		key, _ := crypto.GenerateKey()
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(key)
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		addr := crypto.PubkeyToAddress(key.PublicKey)
		val := New(addr, blsPublicKey)
		validators = append(validators, val)
		b = append(b, val.Address().Bytes()...)
		b = append(b, blsPublicKey...)
	}

	// Create ValidatorSet
	valSet := NewSet(ExtractValidators(b), istanbul.RoundRobin)
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
	val1 := New(addr1, []byte{})
	val2 := New(addr2, []byte{})

	validators, _ := istanbul.CombineIstanbulExtraToValidatorData([]common.Address{addr1, addr2}, [][]byte{{}, {}})
	valSet := newDefaultSet(validators, istanbul.RoundRobin)
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
	// test get proposer
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val1)
	}
	// test calculate proposer
	lastProposer := addr1
	valSet.CalcProposer(lastProposer, uint64(0))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val2)
	}
	valSet.CalcProposer(lastProposer, uint64(3))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val1)
	}
	// test empty last proposer
	lastProposer = common.Address{}
	valSet.CalcProposer(lastProposer, uint64(3))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val2)
	}
}

func testEmptyValSet(t *testing.T) {
	valSet := NewSet(ExtractValidators([]byte{}), istanbul.RoundRobin)
	if valSet == nil {
		t.Errorf("validator set should not be nil")
	}
}

func testAddAndRemoveValidator(t *testing.T) {
	valSet := NewSet(ExtractValidators([]byte{}), istanbul.RoundRobin)
	if !valSet.AddValidators(
		[]istanbul.ValidatorData{
			{
				common.BytesToAddress([]byte(string(3))),
				[]byte{},
			},
		},
	) {
		t.Error("the validator should be added")
	}
	if valSet.AddValidators(
		[]istanbul.ValidatorData{
			{
				common.BytesToAddress([]byte(string(3))),
				[]byte{},
			},
		},
	) {
		t.Error("the existing validator should not be added")
	}
	valSet.AddValidators(
		[]istanbul.ValidatorData{
			{
				common.BytesToAddress([]byte(string(2))),
				[]byte{},
			},
			{
				common.BytesToAddress([]byte(string(1))),
				[]byte{},
			},
		},
	)
	if valSet.Size() != 3 {
		t.Error("the size of validator set should be 3")
	}

	expectedOrder := []int{3, 2, 1}
	for i, v := range valSet.List() {
		expected := common.BytesToAddress([]byte(string(expectedOrder[i])))
		if v.Address() != expected {
			t.Errorf("the order of validators is wrong: have %v, want %v", v.Address().Hex(), expected.Hex())
		}
	}

	if !valSet.RemoveValidators(big.NewInt(1)) { // remove first falidator
		t.Error("the validator should be removed")
	}
	if valSet.RemoveValidators(big.NewInt(1)) {
		t.Error("the non-existing validator should not be removed")
	}
	if len(valSet.List()) != 3 || len(valSet.List()) != valSet.PaddedSize() || valSet.Size() != 2 { // validators set should have the same padded size but reduced size
		t.Error("the size of validator set should be 2")
	}
	valSet.RemoveValidators(big.NewInt(2))                                                          // remove second validator
	if len(valSet.List()) != 3 || len(valSet.List()) != valSet.PaddedSize() || valSet.Size() != 1 { // validators set should have the same padded size but reduced size
		t.Error("the size of validator set should be 1")
	}
	valSet.RemoveValidators(big.NewInt(4))                                                          // remove third validator
	if len(valSet.List()) != 3 || len(valSet.List()) != valSet.PaddedSize() || valSet.Size() != 0 { // validators set should have the same padded size but reduced size
		t.Error("the size of validator set should be 0")
	}
}

func testStickyProposer(t *testing.T) {
	b1 := common.Hex2Bytes(testAddress)
	b2 := common.Hex2Bytes(testAddress2)
	addr1 := common.BytesToAddress(b1)
	addr2 := common.BytesToAddress(b2)
	val1 := New(addr1, []byte{})
	val2 := New(addr2, []byte{})

	validators, _ := istanbul.CombineIstanbulExtraToValidatorData([]common.Address{addr1, addr2}, [][]byte{{}, {}})
	valSet := newDefaultSet(validators, istanbul.Sticky)

	// test get proposer
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val1)
	}
	// test calculate proposer
	lastProposer := addr1
	valSet.CalcProposer(lastProposer, uint64(0))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val1) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val1)
	}

	valSet.CalcProposer(lastProposer, uint64(1))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val2)
	}
	// test empty last proposer
	lastProposer = common.Address{}
	valSet.CalcProposer(lastProposer, uint64(3))
	if val := valSet.GetProposer(); !reflect.DeepEqual(val, val2) {
		t.Errorf("proposer mismatch: have %v, want %v", val, val2)
	}
}

func generateValidators(n int) []common.Address {
	vals := make([]common.Address, 0)
	for i := 0; i < n; i++ {
		privateKey, _ := crypto.GenerateKey()
		vals = append(vals, crypto.PubkeyToAddress(privateKey.PublicKey))
	}
	return vals
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
		vals := generateValidators(testCase.validatorSetSize)
		valSet := newDefaultSet(vals, istanbul.RoundRobin)

		if valSet.MinQuorumSize() != testCase.expectedMinQuorumSize {
			t.Errorf("error mismatch quorum size for valset of size %d: have %d, want %d", valSet.Size(), valSet.MinQuorumSize(), testCase.expectedMinQuorumSize)
		}
	}
}
