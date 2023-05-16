// Copyright 2016 The go-ethereum Authors
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

package types

import (
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/rlp"
)

func TestEIP155Signing(t *testing.T) {
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	signer := NewEIP155Signer(big.NewInt(18))
	tx, err := SignTx(NewTransaction(0, addr, new(big.Int), 0, new(big.Int), nil), signer, key)
	if err != nil {
		t.Fatal(err)
	}

	from, err := Sender(signer, tx)
	if err != nil {
		t.Fatal(err)
	}
	if from != addr {
		t.Errorf("exected from and address to be equal. Got %x want %x", from, addr)
	}
}

func TestEIP155ChainId(t *testing.T) {
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	signer := NewEIP155Signer(big.NewInt(18))
	tx, err := SignTx(NewTransaction(0, addr, new(big.Int), 0, new(big.Int), nil), signer, key)
	if err != nil {
		t.Fatal(err)
	}
	if !tx.Protected() {
		t.Fatal("expected tx to be protected")
	}

	if tx.ChainId().Cmp(signer.chainId) != 0 {
		t.Error("expected chainId to be", signer.chainId, "got", tx.ChainId())
	}

	tx = NewTransaction(0, addr, new(big.Int), 0, new(big.Int), nil)
	tx, err = SignTx(tx, HomesteadSigner{}, key)
	if err != nil {
		t.Fatal(err)
	}

	if tx.Protected() {
		t.Error("didn't expect tx to be protected")
	}

	if tx.ChainId().Sign() != 0 {
		t.Error("expected chain id to be 0 got", tx.ChainId())
	}
}

func TestEIP155SigningVitalik(t *testing.T) {
	// Test vectors come from http://vitalik.ca/files/eip155_testvec.txt
	for i, test := range []struct {
		txRlp, addr string
	}{
		{"f864808504a817c800825208943535353535353535353535353535353535353535808026a0346284a4aa112c6c41d635ce0fd41c984d6397935d490e6c1c81bd67b4755e44a06629fb3e00d115dff2699fdc0d3c9b7f71cb255140737f371a76108e26599a7e", "0x9d8A62f656a8d1615C1294fd71e9CFb3E4855A4F"},
		{"f864018504a817c80182a410943535353535353535353535353535353535353535018025a0f1107052c8df9098c668609b4704e01be47a5ef745d7a8d33f4b2853a8a1d6c7a0564e441d472d39cc2ea70d5e1af392e296262e8a6e747c5caf0fe7a3ae7fca27", "0x5a17650BE84F28Ed583e93E6ed0C99b1D1FC1b34"},
		{"f864028504a817c80282f618943535353535353535353535353535353535353535088026a0bd856d685a405ccfdeb885b3d04882101a4278da737ad3c7d989bdc4213da9a5a061b23f9cee691e590b151a05f593a00b7a073065e68ef98d6b5aafeac5dfec20", "0x0EfbD0bEC0dA8dCc0Ad442A7D337E9CDc2dd6a54"},
		{"f865038504a817c803830148209435353535353535353535353535353535353535351b8025a032e199733aaa93e16308cde3292456979cb4d9e6dc873fa339591c2bb43faf9ba056c120a49d7aba69579d96d3e576514f7408e607f717c925c69c7117f04414b7", "0x0E8E18e1A11E6196f6B82426196027d042Fd6812"},
		{"f865048504a817c80483019a28943535353535353535353535353535353535353535408026a06767942ee29120044b7a7415741061909630cafe06c04c5f81906be18385ca9ea04a099c25e3683e4b4c32d9753d998cbb9e6a4194b59929ff9c22b3cbb64fd92d", "0x3D70712107839699cbE3eC3e75fa8Ec6a44C0625"},
		{"f865058504a817c8058301ec309435353535353535353535353535353535353535357d8026a03bb2592dbfb4fd00ac1ea7b4aa241efd08f7bbdcdf166cc804de768680858e18a0398e3320f6716143f9ada13f21f19535edb26e679c0cf7991fa5f34bff25dcef", "0x494f3f981cA2533dD9C6477f655133c89aBF2030"},
		{"f866068504a817c80683023e3894353535353535353535353535353535353535353581d88025a041d9c7777f4ff66552c46de7fdfc85c5a86bf880a26a5395112941c7744b3ec7a07209c1667636c60cd452160a6f4b26d3e5e3b487b33a6687088c47bcbaf07a45", "0x90A5ebA89D893FB670308119441d0e6555fd7a9F"},
		{"f867078504a817c807830290409435353535353535353535353535353535353535358201578026a006691a7f9fdf2c6ed2aa1dd8769d941cc706b075428f8f25f879b3027c1acd01a02c837e5b47900c9bcdc0b5db34244a8fc213ec96163aeda4db08ca6aebfc7c58", "0x7b3849cEd2246f6f593398FB48e7e2572ab9CBda"},
		{"f867088504a817c8088302e2489435353535353535353535353535353535353535358202008026a0cebf4fdf381c0ad24aec207dfb78c9e17cde4a0072e1d77204c6f5ff913355d1a02c113d962ddde1c526b82b37cebc0de32485371a697b3277548f533150de97a1", "0x3deacbDb4382dec333Ac86fE9526B047B3035B13"},
		{"f867098504a817c809830334509435353535353535353535353535353535353535358202d98026a0262ac04ece576b832f423077e2024059bbb34210b07a690cd18b33deeea22062a0549842ff4e16a3272213c810dc026c15a63f0e4b1676fee61882a41d8b8393a0", "0x72D509501Ab9a120df085b7aB64FbA60454D5Bbf"},
	} {
		signer := NewEIP155Signer(big.NewInt(1))

		var tx *Transaction
		err := rlp.DecodeBytes(common.Hex2Bytes(test.txRlp), &tx)
		if err != nil {
			t.Errorf("%d: %v", i, err)
			continue
		}

		from, err := Sender(signer, tx)
		if err != nil {
			t.Errorf("%d: %v", i, err)
			continue
		}

		addr := common.HexToAddress(test.addr)
		if from != addr {
			t.Errorf("%d: expected %x got %x", i, addr, from)
		}

	}
}

func TestChainId(t *testing.T) {
	key, _ := defaultTestKey()

	tx := NewTransaction(0, common.Address{}, new(big.Int), 0, new(big.Int), nil)

	var err error
	tx, err = SignTx(tx, NewEIP155Signer(big.NewInt(1)), key)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Sender(NewEIP155Signer(big.NewInt(2)), tx)
	if err != ErrInvalidChainId {
		t.Error("expected error:", ErrInvalidChainId)
	}

	_, err = Sender(NewEIP155Signer(big.NewInt(1)), tx)
	if err != nil {
		t.Error("expected no error")
	}
}
