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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

func TestEIP155Signing(t *testing.T) {
	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)

	signer := NewEIP155Signer(big.NewInt(18))
	tx, err := SignTx(NewTransaction(0, addr, new(big.Int), 0, new(big.Int), nil, nil, nil), signer, key)
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
	tx, err := SignTx(NewTransaction(0, addr, new(big.Int), 0, new(big.Int), nil, nil, nil), signer, key)
	if err != nil {
		t.Fatal(err)
	}
	if !tx.Protected() {
		t.Fatal("expected tx to be protected")
	}

	if tx.ChainId().Cmp(signer.chainId) != 0 {
		t.Error("expected chainId to be", signer.chainId, "got", tx.ChainId())
	}

	tx = NewTransaction(0, addr, new(big.Int), 0, new(big.Int), nil, nil, nil)
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
		{"f866808398968082a4108080943535353535353535353535353535353535353535018255441ba05e13b77d1ca6cab8f479a0f979e0bc2556ce17b306c832fb2b9313e57021c75ba02302d57566abf5a33ae7691b9751f5e70585260bea55b8cd5184192342372d7a", "0xf87a4defd9fb983855996d1a7fed5b02af98d9cc"},
		{"f867018401312d008252088080943535353535353535353535353535353535353535018255441ba02a15ff927052f41cb41d8f8622564ba726abbd8a8e15994e87d6e33dc0e2aa03a02f77057ad12f763f5cc1200b105056a45e5c90413fc3641de56b02d36d533dc5", "0x6935890fef53eda8184fb4b41e3c1afc66bde460"},
		{"f87b0a8402625a00826d60809435353735353535353535353535353535353535359435353735353535353535353535353535353535350a8255441ba05547015b5fedaa3a9b37b2c456369e0a031578df6f84863babe6a45af9357ecaa02144e0ffa81eff11ed857503423c77f8b44f85bfa532b8160efb4ec40b4cfe01", "0x46ac6444292d6e397ad5e233cec345b979ab2d4e"},
		{"f87b0a8402625a00826d60809435353735353535353535353535353535353535359435353735353535353535353535353535353535350a8255741ba03b08a1ba373e618447ea87e8f16b60600fdfc3e3a145f147cded36ed67fb08eda02414c92fd86494343b2e24c6e5b14c10f216fc68d1f37ddfc8ea7e0fed7bb2ab", "0xb5c28cc71edcae4b8887f9742c82ae7470b6fbc3"},
		{"f87b0a8402625a00826d60809435353735353535353535353535353535353535a59435353735353535353535353535353535353535350a8255741ca00d1dd55bd93b85484812c37fee292e36166cef2a979f19a794b48ffb254b8e6ba07ac6fbb7918b7a0112238eeb52858974ad5df3ca30cada2f40ddcabab69e66ed", "0x32587123ba45edd5e5d981dd0d0983d25c5876a2"},
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

	tx := NewTransaction(0, common.Address{}, new(big.Int), 0, new(big.Int), nil, nil, nil)

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
