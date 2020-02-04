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
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

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
		Difficulty: big.NewInt(5),
		Number:     big.NewInt(number),
		GasLimit:   1002121,
		GasUsed:    123213,
		Time:       big.NewInt(100),
		Extra:      []byte{01, 02},
	}
	feeCurrencyAddr := common.HexToAddress("02")
	gatewayFeeRecipientAddr := common.HexToAddress("03")
	tx := types.NewTransaction(1, common.HexToAddress("01"), big.NewInt(1), 10000, big.NewInt(10), &feeCurrencyAddr, &gatewayFeeRecipientAddr, big.NewInt(34), []byte{04})
	return types.NewBlock(header, []*types.Transaction{tx}, nil, nil, nil)

}
func dummyMessage(code uint64) *Message {
	return &Message{
		Code:      code,
		Address:   common.HexToAddress("AABB"),
		Msg:       []byte{10, 20, 42},
		Signature: []byte{30, 40, 52},
	}
}
func dummyRoundChangeCertificate() *RoundChangeCertificate {
	return &RoundChangeCertificate{
		RoundChangeMessages: []Message{*dummyMessage(42), *dummyMessage(32), *dummyMessage(15)},
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
