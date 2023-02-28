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

package core

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/stretchr/testify/require"
)

func testPreprepare(t *testing.T) {
	pp := &istanbul.PreprepareV2{
		View: &istanbul.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Proposal: makeBlock(1),
	}
	m := istanbul.NewPreprepareV2Message(pp, common.HexToAddress("0x1234567890"))
	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	decodedMsg := new(istanbul.Message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	decodedPP := decodedMsg.PreprepareV2()
	// if block is encoded/decoded by rlp, we cannot to compare interface data type using reflect.DeepEqual. (like istanbul.Proposal)
	// so individual comparison here.
	if !reflect.DeepEqual(pp.Proposal.Hash(), decodedPP.Proposal.Hash()) {
		t.Errorf("proposal hash mismatch: have %v, want %v", decodedPP.Proposal.Hash(), pp.Proposal.Hash())
	}

	if !reflect.DeepEqual(pp.View, decodedPP.View) {
		t.Errorf("view mismatch: have %v, want %v", decodedPP.View, pp.View)
	}

	if !reflect.DeepEqual(pp.Proposal.Number(), decodedPP.Proposal.Number()) {
		t.Errorf("proposal number mismatch: have %v, want %v", decodedPP.Proposal.Number(), pp.Proposal.Number())
	}
}

func testSubject(t *testing.T) {
	s := &istanbul.Subject{
		View: &istanbul.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Digest: common.BytesToHash([]byte("1234567890")),
	}

	m := istanbul.NewPrepareMessage(s, common.HexToAddress("0x1234567890"))

	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	decodedMsg := new(istanbul.Message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if !reflect.DeepEqual(s, decodedMsg.Prepare()) {
		t.Errorf("subject mismatch: have %v, want %v", decodedMsg.Prepare(), s)
	}
}

func testSubjectWithSignature(t *testing.T) {
	s := &istanbul.Subject{
		View: &istanbul.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Digest: common.BytesToHash([]byte("1234567890")),
	}
	correctKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	correctAddress := crypto.PubkeyToAddress(correctKey.PublicKey)
	signCorrect := func(data []byte) ([]byte, error) {
		return crypto.Sign(crypto.Keccak256(data), correctKey)
	}

	spooferAddress := common.HexToAddress("0x2")

	// 1. Encode test
	m := istanbul.NewPrepareMessage(s, correctAddress)

	err = m.Sign(signCorrect)
	require.NoError(t, err)
	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// 2. Decode test
	// 2.1 Test normal validate func
	decodedMsg := new(istanbul.Message)
	err = decodedMsg.FromPayload(msgPayload, func(data []byte, sig []byte) (common.Address, error) {
		return correctAddress, nil
	})
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if !reflect.DeepEqual(decodedMsg, m) {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// 2.2 Test nil validate func
	decodedMsg = new(istanbul.Message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	if err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(decodedMsg, m) {
		t.Errorf("message mismatch: have %v, want %v", decodedMsg, m)
	}

	// 2.3 Test failed validate func
	decodedMsg = new(istanbul.Message)
	err = decodedMsg.FromPayload(msgPayload, func(data []byte, sig []byte) (common.Address, error) {
		return common.Address{}, istanbul.ErrUnauthorizedAddress
	})
	if err != istanbul.ErrUnauthorizedAddress {
		t.Errorf("error mismatch: have %v, want %v", err, istanbul.ErrUnauthorizedAddress)
	}

	// 2.4 Test spoofing signature by another validator validate func
	decodedMsg = new(istanbul.Message)
	err = decodedMsg.FromPayload(msgPayload, func(data []byte, sig []byte) (common.Address, error) {
		return spooferAddress, nil
	})
	if err != istanbul.ErrInvalidSigner {
		t.Errorf("error mismatch: have %v, want ErrInvalidSigner", err)
	}
}

func TestMessageEncodeDecode(t *testing.T) {
	testPreprepare(t)
	testSubject(t)
	testSubjectWithSignature(t)
}
