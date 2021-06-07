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

package backend

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/crypto"
)

func TestSign(t *testing.T) {
	b := newBackend()
	data := []byte("Here is a string....")
	sig, err := b.Sign(data)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	//Check signature recover
	hashData := crypto.Keccak256(data)
	pubkey, _ := crypto.Ecrecover(hashData, sig)
	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])
	if signer != getAddress() {
		t.Errorf("address mismatch: have %v, want %s", signer.Hex(), getAddress().Hex())
	}
}

func TestCheckSignature(t *testing.T) {
	key, _ := generatePrivateKey()
	data := []byte("Here is a string....")
	hashData := crypto.Keccak256(data)
	sig, _ := crypto.Sign(hashData, key)
	b := newBackend()
	a := getAddress()
	err := b.CheckSignature(data, a, sig)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	a = getInvalidAddress()
	err = b.CheckSignature(data, a, sig)
	if err != errInvalidSignature {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidSignature)
	}
}

func TestCheckValidatorSignature(t *testing.T) {

	vset, keys := newTestValidatorSet(5)

	// 1. Positive test: sign with validator's key should succeed
	data := []byte("dummy data")
	hashData := crypto.Keccak256(data)
	for i, k := range keys {
		// Sign
		sig, err := crypto.Sign(hashData, k)
		if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
		// CheckValidatorSignature should succeed
		addr, err := istanbul.CheckValidatorSignature(vset, data, sig)
		if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
		validator := vset.GetByIndex(uint64(i))
		if addr != validator.Address() {
			t.Errorf("validator address mismatch: have %v, want %v", addr, validator.Address())
		}
	}

	// 2. Negative test: sign with any key other than validator's key should return error
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	// Sign
	sig, err := crypto.Sign(hashData, key)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// CheckValidatorSignature should return ErrUnauthorizedAddress
	addr, err := istanbul.CheckValidatorSignature(vset, data, sig)
	expectedErr := fmt.Errorf("not an elected validator %s", crypto.PubkeyToAddress(key.PublicKey).Hex())
	if err.Error() != expectedErr.Error() {
		t.Errorf("error mismatch: have %v, want %v", err, expectedErr)
	}
	emptyAddr := common.Address{}
	if addr != emptyAddr {
		t.Errorf("address mismatch: have %v, want %v", addr, emptyAddr)
	}
}

func TestNormalCommit(t *testing.T) {

	chain, backend := newBlockChain(1, true)
	defer chain.Stop()
	block := makeBlockWithoutSeal(chain, backend, chain.Genesis())
	expBlock, _ := backend.signBlock(block)
	expectedSignature := make([]byte, types.IstanbulExtraBlsSignature)

	newHeadCh := make(chan core.ChainHeadEvent, 10)
	sub := chain.SubscribeChainHeadEvent(newHeadCh)
	defer sub.Unsubscribe()

	if err := backend.Commit(expBlock, types.IstanbulAggregatedSeal{Round: big.NewInt(0), Bitmap: big.NewInt(0), Signature: expectedSignature}, types.IstanbulEpochValidatorSetSeal{Bitmap: big.NewInt(0), Signature: nil}, nil); err != nil {
		if err != nil {
			t.Errorf("error mismatch: have %v, want %v", err, nil)
		}
	}

	// to avoid race condition is occurred by goroutine
	select {
	case result := <-newHeadCh:
		if result.Block.Hash() != expBlock.Hash() {
			t.Errorf("hash mismatch: have %v, want %v", result.Block.Hash(), expBlock.Hash())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout")
	}

}

func TestInvalidCommit(t *testing.T) {

	chain, backend := newBlockChain(1, true)
	defer chain.Stop()
	block := makeBlockWithoutSeal(chain, backend, chain.Genesis())
	expBlock, _ := backend.signBlock(block)

	if err := backend.Commit(expBlock, types.IstanbulAggregatedSeal{Round: big.NewInt(0), Bitmap: big.NewInt(0), Signature: nil}, types.IstanbulEpochValidatorSetSeal{Bitmap: big.NewInt(0), Signature: nil}, nil); err != nil {
		if err != errInvalidAggregatedSeal {
			t.Errorf("error mismatch: have %v, want %v", err, errInvalidAggregatedSeal)
		}

	}
}

func TestGetProposer(t *testing.T) {
	numValidators := 1
	genesisCfg, nodeKeys := getGenesisAndKeys(numValidators, true)
	chain, engine, _ := newBlockChainWithKeys(false, common.Address{}, false, genesisCfg, nodeKeys[0])
	defer chain.Stop()
	if _, err := makeBlock(nodeKeys, chain, engine, chain.Genesis()); err != nil {
		t.Errorf("Failed to make a block: %v", err)
	}

	expected := engine.AuthorForBlock(1)
	actual := engine.Address()
	if actual != expected {
		t.Errorf("proposer mismatch: have %v, want %v, currentblock: %v", actual.Hex(), expected.Hex(), chain.CurrentBlock().Number())
	}

}
