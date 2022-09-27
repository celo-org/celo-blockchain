package istanbul

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/stretchr/testify/assert"
)

func dummyRoundChangeRequest() *RoundChangeRequest {
	req := RoundChangeRequest{
		View: View{
			Round:    common.Big1,
			Sequence: common.Big2,
		},
		PreparedCertificateV2: PreparedCertificateV2{
			PrepareOrCommitMessages: []Message{},
			ProposalHash:            dummyBlock(2).Hash(),
		},
	}
	// Set empty rather than nil signature since this is how rlp decodes non
	// existent slices.
	req.Signature = []byte{}
	return &req
}

func dummyRoundChangeCertificateV2() *RoundChangeCertificateV2 {
	return &RoundChangeCertificateV2{
		Requests: []RoundChangeRequest{*dummyRoundChangeRequest(), *dummyRoundChangeRequest(), *dummyRoundChangeRequest()},
	}
}

func TestRoundChangeCertificateMaxPCNil(t *testing.T) {
	rcc := &RoundChangeCertificateV2{
		Requests: []RoundChangeRequest{*dummyRoundChangeRequest(), *dummyRoundChangeRequest(), *dummyRoundChangeRequest()},
	}
	rcc.Requests[0].View.Round = big.NewInt(7)
	rcc.Requests[1].View.Round = big.NewInt(3)
	rcc.Requests[2].View.Round = big.NewInt(4)
	r := rcc.HighestRoundWithPreparedCertificate()
	assert.Nil(t, r)
}

func TestRoundChangeCertificateMaxPCNotNil(t *testing.T) {
	rcc := &RoundChangeCertificateV2{
		Requests: []RoundChangeRequest{*dummyRoundChangeRequest(), *dummyRoundChangeRequest(),
			*dummyRoundChangeRequest(), *dummyRoundChangeRequest(), *dummyRoundChangeRequest()},
	}
	rcc.Requests[0].View.Round = big.NewInt(6)
	rcc.Requests[0].PreparedCertificateV2.PrepareOrCommitMessages = make([]Message, 1)
	rcc.Requests[1].View.Round = big.NewInt(3)
	rcc.Requests[1].PreparedCertificateV2.PrepareOrCommitMessages = make([]Message, 1)
	rcc.Requests[2].View.Round = big.NewInt(7)
	rcc.Requests[2].PreparedCertificateV2.PrepareOrCommitMessages = make([]Message, 1)
	rcc.Requests[3].View.Round = big.NewInt(10) // doesn't count, Empty PC
	rcc.Requests[3].PreparedCertificateV2.PrepareOrCommitMessages = make([]Message, 0)
	rcc.Requests[3].View.Round = big.NewInt(4)
	rcc.Requests[3].PreparedCertificateV2.PrepareOrCommitMessages = make([]Message, 1)
	r := rcc.HighestRoundWithPreparedCertificate()
	assert.Same(t, r, rcc.Requests[2].View.Round)
	maxPC := rcc.AnyHighestPreparedCertificate()
	assert.NotNil(t, maxPC)
	assert.Same(t, &rcc.Requests[2].PreparedCertificateV2, maxPC)
}

func TestRoundChangeCertificateV2RLPEncoding(t *testing.T) {
	var result, original *RoundChangeCertificateV2
	original = dummyRoundChangeCertificateV2()

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}

	assert.Equal(t, len(original.Requests), len(original.Requests))
	o1 := original.Requests[0]
	r1 := result.Requests[0]

	if !reflect.DeepEqual(o1.Address, r1.Address) {
		t.Fatalf("RLP Encode/Decode mismatch at first Address")
	}

	if !reflect.DeepEqual(o1.Signature, r1.Signature) {
		t.Fatalf("RLP Encode/Decode mismatch at first Signature")
	}

	if !reflect.DeepEqual(o1.View, r1.View) {
		t.Fatalf("RLP Encode/Decode mismatch at first View")
	}

	if !reflect.DeepEqual(o1.PreparedCertificateV2, r1.PreparedCertificateV2) {
		t.Fatalf("RLP Encode/Decode mismatch at first PreparedCertificateV2")
	}

	if !reflect.DeepEqual(original, result) {
		t.Fatalf("RLP Encode/Decode mismatch. Got %v, expected %v", result, original)
	}
}

func TestRoundChangeRequestRLPEncoding(t *testing.T) {
	var result, original *RoundChangeRequest
	original = &RoundChangeRequest{
		Address: common.BigToAddress(big.NewInt(3)),
		View: View{
			Round:    common.Big1,
			Sequence: common.Big256,
		},
		PreparedCertificateV2: PreparedCertificateV2{
			PrepareOrCommitMessages: []Message{},
		},
		Signature: []byte{3, 2},
	}

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}
	o1 := original
	r1 := result
	if !reflect.DeepEqual(o1.Address, r1.Address) {
		t.Fatalf("RLP Encode/Decode mismatch at first Address")
	}

	if !reflect.DeepEqual(o1.Signature, r1.Signature) {
		t.Fatalf("RLP Encode/Decode mismatch at first Signature")
	}

	if !reflect.DeepEqual(o1.View, r1.View) {
		t.Fatalf("RLP Encode/Decode mismatch at first View")
	}

	if !reflect.DeepEqual(o1.PreparedCertificateV2, r1.PreparedCertificateV2) {
		t.Fatalf("RLP Encode/Decode mismatch at first PreparedCertificateV2. Got %v, expected %v", result, original)
	}

	if !reflect.DeepEqual(original, result) {
		t.Fatalf("RLP Encode/Decode mismatch. Got %v, expected %v", result, original)
	}
}

func TestRoundChangeV2RLPEncoding(t *testing.T) {
	var result, original *RoundChangeV2
	pc := EmptyPreparedCertificate()
	request := RoundChangeRequest{
		Address: common.BigToAddress(big.NewInt(3)),
		View: View{
			Round:    common.Big1,
			Sequence: common.Big256,
		},
		PreparedCertificateV2: PCV2FromPCV1(pc),
		Signature:             []byte{3, 2},
	}
	original = &RoundChangeV2{
		Request:          request,
		PreparedProposal: pc.Proposal,
	}

	rawVal, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Fatalf("Error %v", err)
	}

	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}

	if !reflect.DeepEqual(original.PreparedProposal.Number(), result.PreparedProposal.Number()) {
		t.Fatalf("RLP Encode/Decode mismatch. Got %v, expected %v", result.PreparedProposal.Number(), original.PreparedProposal.Number())
	}

	if !reflect.DeepEqual(original.Request, result.Request) {
		t.Fatalf("RLP Encode/Decode mismatch. Got %v, expected %v", result.Request, original.Request)
	}
}
