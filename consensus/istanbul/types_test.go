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

package istanbul

import (
	"hash"
	"math/big"
	"reflect"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/rlp"
	"golang.org/x/crypto/sha3"
)

// testHasher is the helper tool for transaction/receipt list hashing.
// The original hasher is trie, in order to get rid of import cycle,
// use the testing hasher instead.
type testHasher struct {
	hasher hash.Hash
}

func newHasher() *testHasher {
	return &testHasher{hasher: sha3.NewLegacyKeccak256()}
}

func (h *testHasher) Reset() {
	h.hasher.Reset()
}

func (h *testHasher) Update(key, val []byte) {
	h.hasher.Write(key)
	h.hasher.Write(val)
}

func (h *testHasher) Hash() common.Hash {
	return common.BytesToHash(h.hasher.Sum(nil))
}

func TestViewCompare(t *testing.T) {
	// test equality
	srvView := &View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(1),
	}
	tarView := &View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(1),
	}
	if r := srvView.Cmp(tarView); r != 0 {
		t.Errorf("source(%v) should be equal to target(%v): have %v, want %v", srvView, tarView, r, 0)
	}

	// test larger Sequence
	tarView = &View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(1),
	}
	if r := srvView.Cmp(tarView); r != 1 {
		t.Errorf("source(%v) should be larger than target(%v): have %v, want %v", srvView, tarView, r, 1)
	}

	// test larger Round
	tarView = &View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(0),
	}
	if r := srvView.Cmp(tarView); r != 1 {
		t.Errorf("source(%v) should be larger than target(%v): have %v, want %v", srvView, tarView, r, 1)
	}

	// test smaller Sequence
	tarView = &View{
		Sequence: big.NewInt(3),
		Round:    big.NewInt(1),
	}
	if r := srvView.Cmp(tarView); r != -1 {
		t.Errorf("source(%v) should be smaller than target(%v): have %v, want %v", srvView, tarView, r, -1)
	}
	tarView = &View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(2),
	}
	if r := srvView.Cmp(tarView); r != -1 {
		t.Errorf("source(%v) should be smaller than target(%v): have %v, want %v", srvView, tarView, r, -1)
	}
}

func dummyView() *View {
	return &View{
		Round:    big.NewInt(15),
		Sequence: big.NewInt(42),
	}
}
func dummySubject() *Subject {
	return &Subject{
		View:   dummyView(),
		Digest: common.HexToHash("1234567890"),
	}
}

func dummyBlock(number int64) *types.Block {
	header := &types.Header{
		Number:  big.NewInt(number),
		GasUsed: 123213,
		Time:    100,
		Extra:   []byte{01, 02},
	}
	feeCurrencyAddr := common.HexToAddress("02")
	tx := types.NewCeloTransaction(1, common.HexToAddress("01"), big.NewInt(1), 10000, big.NewInt(10), &feeCurrencyAddr, []byte{04})
	return types.NewBlock(header, []*types.Transaction{tx}, nil, nil, newHasher())
}
func dummyMessage(code uint64) *Message {
	msg := NewPrepareMessage(dummySubject(), common.HexToAddress("AABB"))
	// Set empty rather than nil signature since this is how rlp decodes non
	// existent slices.
	msg.Signature = []byte{}
	return msg
}

func dummyPreparedCertificate() *PreparedCertificate {
	return &PreparedCertificate{
		PrepareOrCommitMessages: []Message{*dummyMessage(42), *dummyMessage(32), *dummyMessage(15)},
		Proposal:                dummyBlock(1),
	}
}

func assertEqual(t *testing.T, prefix string, o, r interface{}) {
	if !reflect.DeepEqual(o, r) {
		t.Errorf("%s:  Got %#v, expected %#v", prefix, r, o)
	}
}

func TestViewRLPEncoding(t *testing.T) {
	var result, original *View
	original = dummyView()

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}

	if !reflect.DeepEqual(original, result) {
		t.Fatalf("RLP Encode/Decode mismatch. Got %v, expected %v", result, original)
	}
}

func TestMessageRLPEncoding(t *testing.T) {
	var result, original *Message
	original = dummyMessage(42)

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}

	if !reflect.DeepEqual(original, result) {
		t.Fatalf("RLP Encode/Decode mismatch. Got %v, expected %v", result, original)
	}
}

func TestPreparedCertificateRLPEncoding(t *testing.T) {
	var result, original *PreparedCertificate
	original = dummyPreparedCertificate()

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}

	// decoded Blocks don't equal Original ones so we need to check equality differently
	assertEqual(t, "RLP Encode/Decode mismatch: PrepareOrCommitMessages", result.PrepareOrCommitMessages, original.PrepareOrCommitMessages)
	assertEqual(t, "RLP Encode/Decode mismatch: BlockHash", result.Proposal.Hash(), original.Proposal.Hash())
}

func TestSubjectRLPEncoding(t *testing.T) {
	var result, original *Subject
	original = dummySubject()

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}

	if !reflect.DeepEqual(original, result) {
		t.Fatalf("RLP Encode/Decode mismatch. Got %v, expected %v", result, original)
	}
}

func TestCommittedSubjectRLPEncoding(t *testing.T) {
	var result, original *CommittedSubject
	original = &CommittedSubject{
		Subject:               dummySubject(),
		CommittedSeal:         []byte{12, 13, 23},
		EpochValidatorSetSeal: []byte{1, 5, 50},
	}

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}

	if !reflect.DeepEqual(original, result) {
		t.Fatalf("RLP Encode/Decode mismatch. Got %v, expected %v", result, original)
	}
}
