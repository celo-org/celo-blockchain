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
	"gotest.tools/assert"
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
	gatewayFeeRecipientAddr := common.HexToAddress("03")
	tx := types.NewTransaction(1, common.HexToAddress("01"), big.NewInt(1), 10000, big.NewInt(10), &feeCurrencyAddr, &gatewayFeeRecipientAddr, big.NewInt(34), []byte{04})
	return types.NewBlock(header, []*types.Transaction{tx}, nil, nil, newHasher())
}
func dummyMessage(code uint64) *Message {
	msg := NewPrepareMessage(dummySubject(), common.HexToAddress("AABB"))
	// Set empty rather than nil signature since this is how rlp decodes non
	// existent slices.
	msg.Signature = []byte{}
	return msg
}

func dummyRoundChangeMessage() *Message {
	msg := NewPrepareMessage(dummySubject(), common.HexToAddress("AABB"))
	// Set empty rather than nil signature since this is how rlp decodes non
	// existent slices.
	msg.Signature = []byte{}
	msg.Code = MsgRoundChange
	roundChange := &RoundChange{
		View: &View{
			Round:    common.Big1,
			Sequence: common.Big2,
		},
		PreparedCertificate: PreparedCertificate{
			PrepareOrCommitMessages: []Message{},
			Proposal:                dummyBlock(2),
		},
	}
	setMessageBytes(msg, roundChange)
	return msg
}

func dummyRoundChangeCertificate() *RoundChangeCertificate {
	return &RoundChangeCertificate{
		RoundChangeMessages: []Message{*dummyRoundChangeMessage(), *dummyRoundChangeMessage(), *dummyRoundChangeMessage()},
	}
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

func TestRoundChangeCertificateRLPEncoding(t *testing.T) {
	var result, original *RoundChangeCertificate
	original = dummyRoundChangeCertificate()

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}

	assert.Equal(t, len(original.RoundChangeMessages), len(original.RoundChangeMessages))
	o1 := original.RoundChangeMessages[0]
	r1 := result.RoundChangeMessages[0]
	if !reflect.DeepEqual(o1.Code, r1.Code) {
		t.Fatalf("RLP Encode/Decode mismatch at first Code")
	}

	if !reflect.DeepEqual(o1.Code, r1.Code) {
		t.Fatalf("RLP Encode/Decode mismatch at first Code")
	}

	if !reflect.DeepEqual(o1.Address, r1.Address) {
		t.Fatalf("RLP Encode/Decode mismatch at first Address")
	}

	if !reflect.DeepEqual(o1.Signature, r1.Signature) {
		t.Fatalf("RLP Encode/Decode mismatch at first Signature")
	}

	if !reflect.DeepEqual(o1.Msg, r1.Msg) {
		t.Fatalf("RLP Encode/Decode mismatch at first internal Msg bytes. %v ----- %v", o1.Msg, r1.Msg)
	}

	original.RoundChangeMessages[0].prepare = nil
	original.RoundChangeMessages[1].prepare = nil
	original.RoundChangeMessages[2].prepare = nil
	if !reflect.DeepEqual(original, result) {
		t.Fatalf("RLP Encode/Decode mismatch. Got %v, expected %v", result, original)
	}
}

func TestPreprepareRLPEncoding(t *testing.T) {
	var result, original *Preprepare
	original = &Preprepare{
		View:                   dummyView(),
		RoundChangeCertificate: *dummyRoundChangeCertificate(),
		Proposal:               dummyBlock(1),
	}

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}

	o := original.RoundChangeCertificate
	o.RoundChangeMessages[0].prepare = nil
	o.RoundChangeMessages[1].prepare = nil
	o.RoundChangeMessages[2].prepare = nil

	// decoded Blocks don't equal Original ones so we need to check equality differently
	assertEqual(t, "RLP Encode/Decode mismatch: View", result.View, original.View)
	assertEqual(t, "RLP Encode/Decode mismatch: RoundChangeCertificate", result.RoundChangeCertificate, original.RoundChangeCertificate)
	assertEqual(t, "RLP Encode/Decode mismatch: BlockHash", result.Proposal.Hash(), original.Proposal.Hash())
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

func TestRoundChangeRLPEncoding(t *testing.T) {
	var result, original *RoundChange
	original = &RoundChange{
		View:                dummyView(),
		PreparedCertificate: *dummyPreparedCertificate(),
	}

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}

	// decoded Blocks don't equal Original ones so we need to check equality differently
	assertEqual(t, "RLP Encode/Decode mismatch: View", result.View, original.View)
	assertEqual(t, "RLP Encode/Decode mismatch: PreparedCertificate.PrepareOrCommitMessages", result.PreparedCertificate.PrepareOrCommitMessages, original.PreparedCertificate.PrepareOrCommitMessages)
	assertEqual(t, "RLP Encode/Decode mismatch: PreparedCertificate.BlockHash", result.PreparedCertificate.Proposal.Hash(), original.PreparedCertificate.Proposal.Hash())
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

func TestForwardMessageRLPEncoding(t *testing.T) {
	var result, original *ForwardMessage
	original = &ForwardMessage{
		Code:          0x11, // istanbulConsensusMsg, but doesn't matter what it is
		DestAddresses: []common.Address{common.HexToAddress("123123")},
		Msg:           []byte{23, 23, 12, 3},
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
