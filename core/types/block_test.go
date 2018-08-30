// Copyright 2014 The go-ethereum Authors
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
	"bytes"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

// from bcValidBlockTest.json, "SimpleTx"
func TestBlockEncoding(t *testing.T) {
	blockEnc := common.FromHex("0xf902bff90256a0df056b5ded5393055f6b3df0e5b5bb0a9b5d72b47066bd3089e66786b397be51a01dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d493479499d0747412109de2cf530e71a427e6f22ab881b2a02ec83283f8557ec0c85bc840d568ae9fd711ff7f1ec0eaec5e8cbd4a1435331aa0f3e88103399eb9e775501fa92e0af1493593d4ba9813e22c0dbf4a16bf60c8a0a0056b23fbba480696b65fe5a59b8f2148a1299103c4f57df839233af2cf4ca2d2b901000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000083020180078347e7c4825208845b819d479ad98301080e846765746888676f312e31302e328664617277696ea0d198efa28bfa0825a13950dbbc1896e1d4e97aa9a834a198f8b0458e3e5e6a328800203cd21b41a98db8413e835d97c294e9c5a24e072eb6cb07c91f562042c4dac5e67229b5cea30b35c73258e1a426173c4267c480a13d7c8c0c92ca70d55837b706194b763e0d57186a01f863f861800a82c35094095e7baea6a6c7c4c2dfeb977efac326af552d870a808208bda0d34f8c5ff9814a2f173f17026ffe1811d6f4c17b192bde71fec0114a6619db69a0377e512f53d89a5f932f6cef6ad4076167f4000bc5ea6c8eaaffb873b61a67f9c0")
	var block Block
	if err := rlp.DecodeBytes(blockEnc, &block); err != nil {
		t.Fatal("decode error: ", err)
	}

	check := func(f string, got, want interface{}) {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s mismatch: got %v, want %v", f, got, want)
		}
	}
	check("Difficulty", block.Difficulty(), big.NewInt(131456))
	check("GasLimit", block.GasLimit(), uint64(4712388))
	check("GasUsed", block.GasUsed(), uint64(21000))
	check("Coinbase", block.Coinbase(), common.HexToAddress("99d0747412109de2cf530e71a427e6f22ab881b2"))
	check("MixDigest", block.MixDigest(), common.HexToHash("d198efa28bfa0825a13950dbbc1896e1d4e97aa9a834a198f8b0458e3e5e6a32"))
	check("Root", block.Root(), common.HexToHash("2ec83283f8557ec0c85bc840d568ae9fd711ff7f1ec0eaec5e8cbd4a1435331a"))
	check("Hash", block.Hash(), common.HexToHash("31dff76060aeef544c9a8b6ed42cef6260699a9aec35abcb9ffe09bb9c805468"))
	check("Nonce", block.Nonce(), uint64(0x203CD21B41A98D))
	check("Time", block.Time(), big.NewInt(1535221063))
	check("Size", block.Size(), common.StorageSize(len(blockEnc)))
	check("ParentHash", block.ParentHash(), common.HexToHash("df056b5ded5393055f6b3df0e5b5bb0a9b5d72b47066bd3089e66786b397be51"))
	check("Signature", block.Signature(), common.FromHex("3e835d97c294e9c5a24e072eb6cb07c91f562042c4dac5e67229b5cea30b35c73258e1a426173c4267c480a13d7c8c0c92ca70d55837b706194b763e0d57186a01"))

	check("len(Transactions)", len(block.Transactions()), 1)
	check("Transactions[0].Hash", block.Transactions()[0].Hash(), common.HexToHash("88d59983cc5bb16689ac6ae163b21311e85533d1698def009aa3becd9bff9eeb"))

	ourBlockEnc, err := rlp.EncodeToBytes(&block)
	if err != nil {
		t.Fatal("encode error: ", err)
	}
	if !bytes.Equal(ourBlockEnc, blockEnc) {
		t.Errorf("encoded block mismatch:\ngot:  %x\nwant: %x", ourBlockEnc, blockEnc)
	}
}
