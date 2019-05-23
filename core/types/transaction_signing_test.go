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
		{"f864808504a817c80082520880943535353535353535353535353535353535353535808026a045ebcd8caaf9b1aa6db727417fb9ae3711c7c80d04fa2d392968433b7a927c819fdeb151b8cdfbbf8ced0f20a15ded738515e6153bd631ff857bb22cbc73b167", "0x9975323aA184a920817066042547A67b1700a780"},
		{"f865808504a817c80182a41080943535353535353535353535353535353535353535018026a0cb969df290d6c45c23b328d840e0368f721d27ae10ed8e2da42f3f9304ce7d97a0590f154b19418b8ed3bf46e3dfaeac0ff150aebef56a37f207dfc07f652e6431", "0x15DC115a7fDbFeA6402cb2146ee9eE1F0d9b685B"},
		{"f865028504a817c80282f61880943535353535353535353535353535353535353535088025a0191156841c4bddde926c8eba336d5c79e0307ba413e3864db8ea5a6a23f2a31ba073387f3e6950eabf5f5a0bf04e280cd2595f16328959c89ca0c8b18ce4cd5657", "0x418324E799943b2836037d85528e4aB95A7401Ac"},
		{"f866038504a817c80383014820809435353535353535353535353535353535353535351b8025a0e0721a26b80a7cc584bcf5fac435ba5cec27d7f160607863a0801c17342819aaa07b76a0fe5ac4d6a4c9e9ca834f0f617b6745f6fd553761ba711cdf729ee99edc", "0x791a7Fc01E196635d448aC6b8494Fb687F0bd830"},
		{"f866048504a817c80483019a28809435353535353535353535353535353535353535351b8026a01f0adc825c370861449bb1087540d264b689e5ef8705c4fcf1df684505a7df81a070c43ca64553df47f518d80b87303ab3aa2c7577f367281126dc097bf095b17e", "0x40BF0C80D1200Ec8B7463D089f872833df5fEB4C"},
		{"f866058504a817c8058301ec30809435353535353535353535353535353535353535357d8025a073062bf462d6f9a53dd9c6022c201f3b29f856e5483dd979f19a5765ce932e75a0638173eb0f9d95294a377df34a2e253af7c09c5a50d8ba0bd0f06974a08f0148", "0xF401EBD852A86edfeCa115A909454A5f328cF1e2"},
		{"f866058504a817c8058301ec30809435353535353535353535353535353535353535357d8026a0bfe68c70b52757c274c19dd544abc7f55e752eeae25cec7e7c3fa74feac935cda0784b97049886e9da06d31eb35a5d98977918f1ae925ceb25f97e61c33aef5d22", "0x15ec62f2157D920ec4245d3AcBbD4Bf2C6e6B1DB"},
		{"f867068504a817c80683023e388094353535353535353535353535353535353535353581d88025a027157f809b4d7611e278d6bc5300d532b97ae3345bee5346e4a5c85b77e06255a076daa5b9f51c953826e46bc72536da136e3e108f1c151f8a7fb889e5d9d1a71d", "0xe09a34b53c0FB7369B76b87FEec82e2687AeA446"},
		{"f868078504a817c80783029040809435353535353535353535353535353535353535358201578025a07aa28f5971dcb16a8297a48ea4bc4e07e1cd46746586cbda5914e9b236c1c998a012c5e7f71145affac105213308589c372ce5a2c0ead8f7e96fb8ac50efa193e9", "0x30422260Be95ca0708BEFB25607304596d9A494A"},
		{"f868088504a817c8088302e248809435353535353535353535353535353535353535358202008026a0ad08dcfc41528968b8cf639b198fafcae0dd40372fb046dba725ce3403488a8aa05cd88023ed3456666d4ac7aa462b7b703b825b6f48f50e536c1d641181a471dd", "0x6830F3AD457737d1F917ae257B35488fceBd00Cb"},
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
