package istanbul

import (
	"bytes"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/params"
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

// **** Big size prepreparev2 test

func fillF(slice []byte, length int) {
	for i := 0; i < length && i < len(slice); i++ {
		slice[i] = 0xFF
	}
}

func getF(length int) []byte {
	f := make([]byte, length)
	fillF(f, length)
	return f
}

func bigF(length int) *big.Int {
	even := new(big.Int).Lsh(common.Big1, uint(length))
	return new(big.Int).Sub(even, common.Big1)
}

func writeExtra(header *types.Header, validators int) error {
	extra := types.IstanbulExtra{
		AddedValidators:           make([]common.Address, validators),
		AddedValidatorsPublicKeys: make([]blscrypto.SerializedPublicKey, validators),
		RemovedValidators:         big.NewInt(int64(validators)),
		Seal:                      getF(64),
		AggregatedSeal: types.IstanbulAggregatedSeal{
			Bitmap:    bigF(validators),
			Signature: getF(48),
			Round:     common.Big32,
		},
		ParentAggregatedSeal: types.IstanbulAggregatedSeal{
			Bitmap:    bigF(validators),
			Signature: getF(48),
			Round:     common.Big32,
		},
	}
	for i := 0; i < validators; i++ {
		extra.AddedValidators[i] = common.BytesToAddress(getF(20))
		fillF(extra.AddedValidatorsPublicKeys[i][:], 96)
	}
	payload, err := rlp.EncodeToBytes(&extra)
	if err != nil {
		return err
	}

	if len(header.Extra) < types.IstanbulExtraVanity {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity-len(header.Extra))...)
	}
	header.Extra = append(header.Extra[:types.IstanbulExtraVanity], payload...)
	return nil
}

func bigHeader(validators int) *types.Header {
	header := &types.Header{
		ParentHash:  common.BigToHash(common.Big257),
		Coinbase:    common.BigToAddress(common.Big32),
		Root:        common.BigToHash(common.Big1),
		TxHash:      common.BigToHash(common.Big256),
		ReceiptHash: common.BigToHash(common.Big3),
		Bloom:       [types.BloomByteLength]byte{},
		Number:      big.NewInt(37),
		GasUsed:     123213,
		Time:        100,
	}
	fillF(header.Bloom[:], types.BloomByteLength)
	writeExtra(header, validators)
	return header
}

func bigTxs(gasLimit int, gasPerByte int) []*types.Transaction {
	curr := common.BytesToAddress(getF(20))
	tx := types.NewCeloTransaction(999, common.BytesToAddress(getF(20)),
		big.NewInt(329274), 2942729, big.NewInt(294279),
		&curr, getF(gasLimit/gasPerByte))
	return []*types.Transaction{tx}
}

func bigBlock(gasLimit int, validators int, gasPerByte int) *types.Block {
	randomness := &types.Randomness{
		Revealed:  common.BigToHash(common.Big3),
		Committed: common.BigToHash(common.Big2),
	}
	// Full header
	header := bigHeader(validators)
	txs := bigTxs(gasLimit, gasPerByte)
	receipts := []*types.Receipt{{
		Type:              9,
		PostState:         getF(32),
		Status:            9,
		CumulativeGasUsed: 1_729_919,
		Bloom:             [types.BloomByteLength]byte{},
		Logs: []*types.Log{{
			Address: common.BytesToAddress(getF(20)),
			Topics:  []common.Hash{common.BytesToHash(getF(32)), common.BytesToHash(getF(32))},
			Data:    getF(53),
		}},
	}}
	fillF(receipts[0].Bloom[:], types.BloomByteLength)
	block := types.NewBlock(header, txs, receipts, randomness, newHasher())
	return block
}

func bigPC(quorum int) *PreparedCertificateV2 {
	// while quorum amount of commits is imposible by consensus design,
	// and the max should be (quorum - 1), for simplification
	// we'll just use quorum commit messages
	pc := &PreparedCertificateV2{}
	pc.ProposalHash = common.BytesToHash(getF(32))
	pc.PrepareOrCommitMessages = make([]Message, quorum)
	for i := 0; i < quorum; i++ {
		msg := NewCommitMessage(&CommittedSubject{
			Subject: &Subject{
				View:   &View{Round: big.NewInt(21), Sequence: big.NewInt(3_192_191_242)},
				Digest: common.BytesToHash(getF(32)),
			},
			EpochValidatorSetSeal: getF(48),
			CommittedSeal:         getF(48),
		}, common.BytesToAddress(getF(20)))
		msg.Signature = getF(64)
		pc.PrepareOrCommitMessages[i] = *msg
	}
	return pc
}

func bigRequest(quorum int) *RoundChangeRequest {
	r := &RoundChangeRequest{
		Address: common.BytesToAddress(getF(20)),
		View: View{
			Round:    big.NewInt(23),
			Sequence: big.NewInt(2_912_119_182),
		},
		PreparedCertificateV2: *bigPC(quorum),
		Signature:             getF(64),
	}
	return r
}

func bigRCC(validators int) *RoundChangeCertificateV2 {
	rcc := &RoundChangeCertificateV2{}
	quorum := int(math.Ceil(float64(2*validators) / 3))
	rcc.Requests = make([]RoundChangeRequest, quorum)
	for i := 0; i < quorum; i++ {
		rcc.Requests[i] = *bigRequest(quorum)
	}
	return rcc
}

// TestBigPreprepareV2Size constructs the largest possible consensus message in
// order to ensure that it does not exceed the built in size limits.
func TestBigPreprepareV2Size(t *testing.T) {
	gasLimit := 34_600_000
	validators := 110
	gasPerByte := params.TxDataZeroGas
	sizeLimit := int(9.5 * 1024 * 1024)
	block := bigBlock(gasLimit, validators, int(gasPerByte))
	assert.NotNil(t, block)
	bigRCC := bigRCC(validators)
	assert.NotNil(t, bigRCC)
	message := NewPreprepareV2Message(&PreprepareV2{
		View: &View{
			Round:    big.NewInt(15),
			Sequence: big.NewInt(2_170_123_456),
		},
		Proposal:                 block,
		RoundChangeCertificateV2: *bigRCC,
	}, common.BytesToAddress(getF(20)))

	rawVal, err := rlp.EncodeToBytes(message)
	if err != nil {
		t.Fatalf("Error %v", err)
	}
	assert.LessOrEqual(t, len(rawVal), sizeLimit, "PreprepareV2 message size is too big")
	result := &Message{}
	if err = rlp.DecodeBytes(rawVal, &result); err != nil {
		t.Fatalf("Error %v", err)
	}
	rawVal2, err := rlp.EncodeToBytes(result)
	if err != nil {
		t.Fatalf("Error %v", err)
	}
	if !reflect.DeepEqual(rawVal, rawVal2) {
		t.Fatalf("Serialization/Deserialization failed for PreprepareV2")
	}
}
