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
	"fmt"
	"hash"
	"math/big"
	"reflect"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/math"
	"github.com/celo-org/celo-blockchain/crypto"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
	_ "github.com/davecgh/go-spew/spew"
	"golang.org/x/crypto/sha3"
)

// Generated with the util method:
// createBlock -> CeloLegacy: 1
func TestBlockEncodingCeloLegacyTxs(t *testing.T) {
	blockEnc := common.FromHex("f90319f901d5a07285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7948888f1f195afa192cfee860698584c030f4c9db1a0ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217a05ebe898c5c30b45af9dd71f59a799520cd103b5d02e321f9e1c714ea48d1fdfca019829ca88fd387ee55bd8449485b92d2b7ff58cd6a62c1a151fda9e2f05c65b8b901000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000082303982c350845c47775cad636f6f6c65737420626c6f636b206f6e20636861696e00000000000000000000ccc0c08080c3808080c3808080f8f8f8f680830493e082c3509400000000000000000000000000000000000000009400000000000000000000000000000000000000008203e894095e7baea6a6c7c4c2dfeb977efac326af552d8701b86400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000820a96a0a95c8392c62f8c740efe721105dc31707c81d15921cf19b6ba8a74953ffaeb2aa03c93a80b8a6da3e6b039863267511f9953e75364699c05359a8d6d4a0bdb3941f842a00000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000c28080")

	var block Block
	if err := rlp.DecodeBytes(blockEnc, &block); err != nil {
		t.Fatal("decode error: ", err)
	}

	check := func(f string, got, want interface{}) {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s mismatch: got %v, want %v", f, got, want)
		}
	}
	check("GasUsed", block.GasUsed(), uint64(50000))
	check("Coinbase", block.Coinbase(), common.HexToAddress("8888f1f195afa192cfee860698584c030f4c9db1"))
	check("Root", block.Root(), common.HexToHash("ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217"))
	check("Hash", block.Hash(), common.HexToHash("16618ef6d143d11b44f426db736329c7b0e81e33d77fe7993621dd4277b1512c"))
	check("Time", block.Time(), uint64(1548187484))
	check("Size", block.Size(), common.StorageSize(len(blockEnc)))
	check("ParentHash", block.ParentHash(), common.HexToHash("7285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7"))
	check("Extra", block.Extra(), emptyIstanbulExtra([]byte("coolest block on chain")))

	check("len(Transactions)", len(block.Transactions()), 1)
	check("Transactions[0].Hash", block.Transactions()[0].Hash(), common.HexToHash("d74ae944920493f7d41d48a1da3c46318d1c0788e9b1774ecef4f878394eee53"))

	ourBlockEnc, err := rlp.EncodeToBytes(&block)
	if err != nil {
		t.Fatal("encode error: ", err)
	}
	if !bytes.Equal(ourBlockEnc, blockEnc) {
		t.Errorf("encoded block mismatch:\ngot:  %x\nwant: %x", ourBlockEnc, blockEnc)
	}
}

// Generated with the util method:
// createBlock -> CeloLegacy: 1, DynamicFeeTxs: 1
func TestEIP1559BlockEncoding(t *testing.T) {
	blockEnc := common.FromHex("f903bef901d6a07285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7948888f1f195afa192cfee860698584c030f4c9db1a0ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217a08afeeb61dfb28377082555e5be97cd4bec68828c03f5e910b773c6a402196c42a099c51a06b6b6165ed0c0dfac90df0b72f27225a47cddf9011bdb45012a0bf6d7b90100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008230398301d4c0845c47775cad636f6f6c65737420626c6f636b206f6e20636861696e00000000000000000000ccc0c08080c3808080c3808080f9019bf8f680830493e082c3509400000000000000000000000000000000000000009400000000000000000000000000000000000000008203e894095e7baea6a6c7c4c2dfeb977efac326af552d8701b86400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000820a96a0ff643382d19c8e29e3a8637f9241d93a977704eb91122d2ccbce9415107ef625a06a4725093d29a71ffbc73dc72d9588c2d173a715d56de69b53c77046b33a3eddb8a102f89e82053980800a8301117094095e7baea6a6c7c4c2dfeb977efac326af552d878080f838f7940000000000000000000000000000000000000001e1a0000000000000000000000000000000000000000000000000000000000000000001a04ba9f3a31eee69b0ac69a460ef5a6f93950a229f1fa74f887b220b398b20c148a023c9c9ce46eaa1d3136c4c6e995fb8af06d9594f94977607249dba58d8edcc6cf842a00000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000c28080")
	var block Block
	if err := rlp.DecodeBytes(blockEnc, &block); err != nil {
		t.Fatal("decode error: ", err)
	}

	check := func(f string, got, want interface{}) {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s mismatch: got %v, want %v", f, got, want)
		}
	}

	check("GasUsed", block.GasUsed(), uint64(120000))
	check("Coinbase", block.Coinbase(), common.HexToAddress("8888f1f195afa192cfee860698584c030f4c9db1"))
	check("Root", block.Root(), common.HexToHash("ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217"))
	check("Hash", block.Hash(), common.HexToHash("f3f0112783783ae0e79b384bd3265da05ef85f2b080d9d7f8540e7278dacda52"))
	check("Time", block.Time(), uint64(1548187484))
	check("Size", block.Size(), common.StorageSize(len(blockEnc)))
	check("ParentHash", block.ParentHash(), common.HexToHash("7285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7"))
	check("Extra", block.Extra(), emptyIstanbulExtra([]byte("coolest block on chain")))

	check("len(Transactions)", len(block.Transactions()), 2)
	check("Transactions[0].Hash", block.Transactions()[0].Hash(), common.HexToHash("eb823c9e0781df0f8961c2760d91a0387b0c00b41badf53d3b7656d1a10cabd3"))
	check("Transactions[1].Hash", block.Transactions()[1].Hash(), common.HexToHash("cf6799dc62eb4afe4699153cb17c828059b7759ee14c3545069de0673732ec99"))
	check("Transactions[1].Type", block.Transactions()[1].Type(), uint8(DynamicFeeTxType))
	ourBlockEnc, err := rlp.EncodeToBytes(&block)
	if err != nil {
		t.Fatal("encode error: ", err)
	}
	if !bytes.Equal(ourBlockEnc, blockEnc) {
		t.Errorf("encoded block mismatch:\ngot:  %x\nwant: %x", ourBlockEnc, blockEnc)
	}
}

// Generated with the util method:
// createBlock -> CeloLegacy: 1, AccessListTxs: 1
func TestEIP2718BlockEncoding(t *testing.T) {
	blockEnc := common.FromHex("f903bcf901d6a07285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7948888f1f195afa192cfee860698584c030f4c9db1a0ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217a0ab0d7413f7e84cf84c660d658b6a284bf89d4817d4b88e59b30ca67373760677a09c6c84a3ae7411f91bd89537bebf7e9d1e670512ccc785c8106cef6f04a29bd1b90100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008230398301adb0845c47775cad636f6f6c65737420626c6f636b206f6e20636861696e00000000000000000000ccc0c08080c3808080c3808080f90199f8f680830493e082c3509400000000000000000000000000000000000000009400000000000000000000000000000000000000008203e894095e7baea6a6c7c4c2dfeb977efac326af552d8701b86400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000820a95a085fe0bb7c1ac0b3e5b8076f2aa4cdc083bf666ca32a92758b09e06777d0b51aca04879d3b0ed60accb5fba0e8d5cadef138b1e8644975aff0994eb991168f58b6bb89f01f89c820539800a82ea6094095e7baea6a6c7c4c2dfeb977efac326af552d878080f838f7940000000000000000000000000000000000000001e1a0000000000000000000000000000000000000000000000000000000000000000080a06a2bb1555edd8a1713fcfcd52c99321881bd0f84ae2fa02ffc6cc4f2674b296da0254430e2d09f1a3e382ad2eb3c91aa1aee7687ae416eef93e89ed7d31a5b3824f842a00000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000c28080")
	var block Block
	if err := rlp.DecodeBytes(blockEnc, &block); err != nil {
		t.Fatal("decode error: ", err)
	}

	check := func(f string, got, want interface{}) {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s mismatch: got %v, want %v", f, got, want)
		}
	}
	check("GasUsed", block.GasUsed(), uint64(110000))
	check("Coinbase", block.Coinbase(), common.HexToAddress("8888f1f195afa192cfee860698584c030f4c9db1"))
	check("Root", block.Root(), common.HexToHash("ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217"))
	check("Hash", block.Hash(), common.HexToHash("ead833948c3798edac08c01accdbf46ce46666295ed9da9665e14d8c631f1b13"))
	check("Time", block.Time(), uint64(1548187484))
	check("Size", block.Size(), common.StorageSize(len(blockEnc)))
	check("ParentHash", block.ParentHash(), common.HexToHash("7285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7"))
	check("Extra", block.Extra(), emptyIstanbulExtra([]byte("coolest block on chain")))

	check("len(Transactions)", len(block.Transactions()), 2)
	check("Transactions[0].Hash", block.Transactions()[0].Hash(), common.HexToHash("67b37f4c36761cfa6f92da73e4d4685e51909f91e592021b67f82eeb2ab5cca6"))
	check("Transactions[1].Hash", block.Transactions()[1].Hash(), common.HexToHash("14937a6d6598adcf2fc3b98999c426cd194615ed87f4aee326fc48a2d6127f65"))
	check("Transactions[1].Type()", block.Transactions()[1].Type(), uint8(AccessListTxType))

	ourBlockEnc, err := rlp.EncodeToBytes(&block)
	if err != nil {
		t.Fatal("encode error: ", err)
	}
	if !bytes.Equal(ourBlockEnc, blockEnc) {
		t.Errorf("encoded block mismatch:\ngot:  %x\nwant: %x", ourBlockEnc, blockEnc)
	}
}

var benchBuffer = bytes.NewBuffer(make([]byte, 0, 32000))

func BenchmarkEncodeBlock(b *testing.B) {
	block := makeBenchBlock()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchBuffer.Reset()
		if err := rlp.Encode(benchBuffer, block); err != nil {
			b.Fatal(err)
		}
	}
}

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

func makeBenchBlock() *Block {
	var (
		key, _   = crypto.GenerateKey()
		txs      = make([]*Transaction, 70)
		receipts = make([]*Receipt, len(txs))
		signer   = LatestSigner(params.TestChainConfig)
		uncles   = make([]*Header, 3)
	)
	header := &Header{
		Number:  math.BigPow(2, 9),
		GasUsed: 1476322,
		Time:    9876543,
		Extra:   []byte("coolest block on chain"),
	}
	for i := range txs {
		amount := math.BigPow(2, int64(i))
		price := big.NewInt(300000)
		data := make([]byte, 100)
		tx := NewTransaction(uint64(i), common.Address{}, amount, 123457, price, data)
		signedTx, err := SignTx(tx, signer, key)
		if err != nil {
			panic(err)
		}
		txs[i] = signedTx
		receipts[i] = NewReceipt(make([]byte, 32), false, tx.Gas())
	}
	for i := range uncles {
		uncles[i] = &Header{
			Number:  math.BigPow(2, 9),
			GasUsed: 1476322,
			Time:    9876543,
			Extra:   []byte("benchmark uncle"),
		}
	}
	return NewBlock(header, txs, receipts, nil, newHasher())
}

func createBlock(celoLegacyTxs, accessListTxs, dynamicFeeTxs uint) {
	var (
		key, _     = crypto.GenerateKey()
		txs        = make([]*Transaction, celoLegacyTxs+accessListTxs+dynamicFeeTxs)
		receipts   = make([]*Receipt, len(txs))
		signer     = LatestSigner(params.TestChainConfig)
		to         = common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87")
		chainID    = params.TestChainConfig.ChainID
		parentHash = common.HexToHash("7285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7")
		coinbase   = common.HexToAddress("8888f1f195afa192cfee860698584c030f4c9db1")
		time       = uint64(1548187484)
		number     = big.NewInt(12345)
		root       = common.HexToHash("ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217")
	)
	header := &Header{
		ParentHash: parentHash,
		Coinbase:   coinbase,
		Root:       root,
		Number:     number,
		Time:       time,
		Extra:      emptyIstanbulExtra([]byte("coolest block on chain")),
	}
	var gasAccumulator uint64 = 0
	for i := range txs {
		amount := math.BigPow(2, int64(i))
		price := big.NewInt(300000)
		data := make([]byte, 100)
		var tx *Transaction
		if celoLegacyTxs > 0 {
			tx = NewCeloTransaction(uint64(i), to, amount, 50000, price, &common.ZeroAddress, &common.ZeroAddress, big.NewInt(1000), data)
			celoLegacyTxs -= 1
		} else {
			// All these types support accessLists
			addr := common.HexToAddress("0x0000000000000000000000000000000000000001")
			accesses := AccessList{AccessTuple{
				Address: addr,
				StorageKeys: []common.Hash{
					{0},
				},
			}}
			if accessListTxs > 0 {
				tx = NewTx(&AccessListTx{
					ChainID:    chainID,
					Nonce:      0,
					To:         &to,
					Gas:        60000,
					GasPrice:   big.NewInt(10),
					AccessList: accesses,
				})
				accessListTxs -= 1
			} else if dynamicFeeTxs > 0 {
				tx = NewTx(&DynamicFeeTx{
					ChainID:    chainID,
					Nonce:      0,
					To:         &to,
					Gas:        70000,
					GasFeeCap:  big.NewInt(10),
					GasTipCap:  big.NewInt(0),
					AccessList: accesses,
					Data:       []byte{},
				})
				dynamicFeeTxs -= 0
			}
		}
		signedTx, err := SignTx(tx, signer, key)
		if err != nil {
			panic(err)
		}
		txs[i] = signedTx
		receipts[i] = NewReceipt(make([]byte, 32), false, tx.Gas())
		gasAccumulator += tx.Gas()
	}
	header.GasUsed = gasAccumulator
	block := NewBlock(header, txs, receipts, nil, newHasher())
	ourBlockEnc, err := rlp.EncodeToBytes(&block)
	if err != nil {
		panic(err)
	}
	fmt.Println("Encoded Block: ")
	fmt.Println(common.Bytes2Hex(ourBlockEnc))
	fmt.Println("Test Info:")
	fmt.Println("GasUsed ", block.GasUsed())
	fmt.Println("Coinbase ", block.Coinbase())
	fmt.Println("Root", block.Root())
	fmt.Println("Hash", block.Hash())
	fmt.Println("Time", block.Time())
	fmt.Println("Size", block.Size(), common.StorageSize(len(ourBlockEnc)))
	fmt.Println("ParentHash", block.ParentHash())
	fmt.Println("len(Transactions)", len(block.Transactions()))

	for i := range txs {
		fmt.Println("Transactions[0].Hash", block.Transactions()[i].Hash())
		fmt.Println("Transactions[0].Type", block.Transactions()[i].Type())
	}
}

// copied from core.chain_makers to avoid import cycle
func emptyIstanbulExtra(vanity []byte) []byte {
	extra := IstanbulExtra{
		AddedValidators:           []common.Address{},
		AddedValidatorsPublicKeys: []blscrypto.SerializedPublicKey{},
		RemovedValidators:         big.NewInt(0),
		Seal:                      []byte{},
		AggregatedSeal:            IstanbulAggregatedSeal{},
		ParentAggregatedSeal:      IstanbulAggregatedSeal{},
	}
	payload, _ := rlp.EncodeToBytes(&extra)

	if len(vanity) < IstanbulExtraVanity {
		vanity = append(vanity, bytes.Repeat([]byte{0x00}, IstanbulExtraVanity-len(vanity))...)
	}
	vanity = append(vanity[:IstanbulExtraVanity], payload...)

	return vanity
}
