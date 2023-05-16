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
// createBlock -> Legacy: 1
func TestBlockEncoding(t *testing.T) {
	blockEnc := common.FromHex("f902ecf901d5a07285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7948888f1f195afa192cfee860698584c030f4c9db1a0ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217a04bc26331d505e250573ea734b7451c10014666e5d4e4cfa86fd6037cc5d2b3d5a0bfe0845dd9333a82059d7e92569026fe3bc80ce84cb1ec156b25d3f5af9cf884b9010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000823039825208845c47775cad636f6f6c65737420626c6f636b206f6e20636861696e00000000000000000000ccc0c08080c3808080c3808080f8cbf8c980830493e082520894095e7baea6a6c7c4c2dfeb977efac326af552d8701b86400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000820a96a08e71380b4c6f524fbf8247424d96179efd611ecc98ef3661eb7dff8863865c12a05f00607ea30c1c024fbf7c19a56099272157fe92b18fbbca95769266fa1e3041f842a00000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000c28080")

	var block Block
	if err := rlp.DecodeBytes(blockEnc, &block); err != nil {
		t.Fatal("decode error: ", err)
	}

	check := func(f string, got, want interface{}) {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s mismatch: got %v, want %v", f, got, want)
		}
	}
	check("GasUsed", block.GasUsed(), uint64(21000))
	check("Coinbase", block.Coinbase(), common.HexToAddress("8888f1f195afa192cfee860698584c030f4c9db1"))
	check("Root", block.Root(), common.HexToHash("ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217"))
	check("Hash", block.Hash(), common.HexToHash("69100faa0f301f61e89b7b762329d08af7b17a3caeb0504046f72831f69cfe28"))
	check("Time", block.Time(), uint64(1548187484))
	check("Size", block.Size(), common.StorageSize(len(blockEnc)))
	check("ParentHash", block.ParentHash(), common.HexToHash("7285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7"))
	check("Extra", block.Extra(), emptyIstanbulExtra([]byte("coolest block on chain")))

	check("len(Transactions)", len(block.Transactions()), 1)
	check("Transactions[0].Hash", block.Transactions()[0].Hash(), common.HexToHash("313d011d5c206cfb532fc44a217f21028ce877ec3e3ca5de263d1c4fda108df9"))

	ourBlockEnc, err := rlp.EncodeToBytes(&block)
	if err != nil {
		t.Fatal("encode error: ", err)
	}
	if !bytes.Equal(ourBlockEnc, blockEnc) {
		t.Errorf("encoded block mismatch:\ngot:  %x\nwant: %x", ourBlockEnc, blockEnc)
	}
}

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
// createBlock -> CeloLegacy: 1, Legacy: 1, DynamicFeeTxs: 1
func TestEIP1559BlockEncoding(t *testing.T) {
	blockEnc := common.FromHex("f90489f901d6a07285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7948888f1f195afa192cfee860698584c030f4c9db1a0ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217a0178d3cb756db0acf9e899fd2570f260875a41240246f24410f3c817421f7f781a0e75e4b78099bb7b8603c7745125e9066908277c1d3150d7b77fef4d767deea4bb9010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000823039830226c8845c47775cad636f6f6c65737420626c6f636b206f6e20636861696e00000000000000000000ccc0c08080c3808080c3808080f90266f8c980830493e082520894095e7baea6a6c7c4c2dfeb977efac326af552d8701b86400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000820a96a05b8dfbd15e874d74987fd6e45341f99e6f95e91f289563c08ae2e7b68e641f42a0036a7b3871170fe7899c8ff85564da341ef83b0166be26f3d3b3453f617e2f44f8f601830493e082c3509400000000000000000000000000000000000000009400000000000000000000000000000000000000008203e894095e7baea6a6c7c4c2dfeb977efac326af552d8702b86400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000820a96a0d51c2ada077d7ff7db5cc9241c7e145e2bed26c08bdf14a20d5da83fbfe5a4cba053fdb46dc7b83daa9eec8c69e8dbae7639bdb0fa5b2f4e8e58cf4a59f9ad2512b8a102f89e82053980800a8301117094095e7baea6a6c7c4c2dfeb977efac326af552d878080f838f7940000000000000000000000000000000000000001e1a0000000000000000000000000000000000000000000000000000000000000000080a0ad9a0634a6a78ac1fd7caa29a898bdb5c44646f7e6e94be7cfb593a2b5c62a7ca01f99362ba6ef06d18b46556601a92bb2f330a7b4893df6d2366bb44f8aff7fa7f842a00000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000c28080")
	var block Block
	if err := rlp.DecodeBytes(blockEnc, &block); err != nil {
		t.Fatal("decode error: ", err)
	}

	check := func(f string, got, want interface{}) {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s mismatch: got %v, want %v", f, got, want)
		}
	}

	check("GasUsed", block.GasUsed(), uint64(141000))
	check("Coinbase", block.Coinbase(), common.HexToAddress("8888f1f195afa192cfee860698584c030f4c9db1"))
	check("Root", block.Root(), common.HexToHash("ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217"))
	check("Hash", block.Hash(), common.HexToHash("36cf56759c5a4eea0077610dacdbb267ad00ba39662a13571ef9c4caaf1de133"))
	check("Time", block.Time(), uint64(1548187484))
	check("Size", block.Size(), common.StorageSize(len(blockEnc)))
	check("ParentHash", block.ParentHash(), common.HexToHash("7285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7"))
	check("Extra", block.Extra(), emptyIstanbulExtra([]byte("coolest block on chain")))

	check("len(Transactions)", len(block.Transactions()), 3)
	check("Transactions[0].Hash", block.Transactions()[0].Hash(), common.HexToHash("9d6717672638dc800925b6f518cf4e1e3a3aeca0f79fc7ff7b479883c5b565e7"))
	check("Transactions[1].Hash", block.Transactions()[1].Hash(), common.HexToHash("f9e94acbc4d774e40a6cecceee9743950ea6c4112ae9436238b5325bd21bb15e"))
	check("Transactions[2].Hash", block.Transactions()[2].Hash(), common.HexToHash("1ea96b999c4dc5ffc56c0cbc91ca364c78eeae39dac68e8919942c1769bc29e5"))
	check("Transactions[2].Type", block.Transactions()[2].Type(), uint8(DynamicFeeTxType))
	ourBlockEnc, err := rlp.EncodeToBytes(&block)
	if err != nil {
		t.Fatal("encode error: ", err)
	}
	if !bytes.Equal(ourBlockEnc, blockEnc) {
		t.Errorf("encoded block mismatch:\ngot:  %x\nwant: %x", ourBlockEnc, blockEnc)
	}
}

// Generated with the util method:
// createBlock -> CeloLegacy: 1, Legacy: 1, CeloDynamicFeeTxs: 1
func TestEIP1559CeloBlockEncoding(t *testing.T) {
	blockEnc := common.FromHex("f904b6f901d6a07285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7948888f1f195afa192cfee860698584c030f4c9db1a0ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217a03a3c17771c830c3581f93196ef443b2c852e710984625103743ee327b3476fe1a050244f4ed1b699592fc00227d7f0c504b21d26b5f1983245d25003477b1016c6b901000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000082303983023a50845c47775cad636f6f6c65737420626c6f636b206f6e20636861696e00000000000000000000ccc0c08080c3808080c3808080f90293f8c980830493e082520894095e7baea6a6c7c4c2dfeb977efac326af552d8701b86400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000820a95a038c14134f828deb8cc96486020beac608d6c9aba45f4d508fbafa01105194319a03f31cd332d7708641b47639c5175ce82c76475fd4a69253dac7e92a7ec5ea558f8f601830493e082c3509400000000000000000000000000000000000000009400000000000000000000000000000000000000008203e894095e7baea6a6c7c4c2dfeb977efac326af552d8702b86400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000820a95a0fb928690245cda26c2f1e6a6cc4276d390cacaba7079f999eac2c7342d4cfd24a030ac11137b365cc5b4366159006e3e67dba6288ca0fd385f65a546a3f25d9e42b8ce7cf8cb82053980800a830124f89400000000000000000000000000000000000000009400000000000000000000000000000000000000008203e894095e7baea6a6c7c4c2dfeb977efac326af552d878080f838f7940000000000000000000000000000000000000001e1a0000000000000000000000000000000000000000000000000000000000000000001a065a74a955526fafc49a3261a1fb3a0c552f80234861bac0b07423d4f8ec239e2a07b1f4c57ea47c388e969368816351a3eabb3523226a15f0adcbd9a24312ed9acf842a00000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000c28080")
	var block Block
	if err := rlp.DecodeBytes(blockEnc, &block); err != nil {
		t.Fatal("decode error: ", err)
	}

	check := func(f string, got, want interface{}) {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s mismatch: got %v, want %v", f, got, want)
		}
	}

	check("GasUsed", block.GasUsed(), uint64(146000))
	check("Coinbase", block.Coinbase(), common.HexToAddress("8888f1f195afa192cfee860698584c030f4c9db1"))
	check("Root", block.Root(), common.HexToHash("ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217"))
	check("Hash", block.Hash(), common.HexToHash("139266d976bb1bfcc5e12763b7f66e59cd6cf89422d1616d452c844e5ca96b12"))
	check("Time", block.Time(), uint64(1548187484))
	check("Size", block.Size(), common.StorageSize(len(blockEnc)))
	check("ParentHash", block.ParentHash(), common.HexToHash("7285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7"))
	check("Extra", block.Extra(), emptyIstanbulExtra([]byte("coolest block on chain")))

	check("len(Transactions)", len(block.Transactions()), 3)
	check("Transactions[0].Hash", block.Transactions()[0].Hash(), common.HexToHash("5923661a86dbd161e781fbdbf80485ea42f4beeb6c4aa31c05fe7f21ef896ed9"))
	check("Transactions[1].Hash", block.Transactions()[1].Hash(), common.HexToHash("47dd96a8229ac4cab7d2cd892fb51ad032713170e27f6c42df553b031db1179b"))
	check("Transactions[2].Hash", block.Transactions()[2].Hash(), common.HexToHash("0df3c3efa8321b7357fc3ba8a00f79440d853ffb15aa5436be88fe91ca78c1a6"))
	check("Transactions[2].Type", block.Transactions()[2].Type(), uint8(CeloDynamicFeeTxType))
	ourBlockEnc, err := rlp.EncodeToBytes(&block)
	if err != nil {
		t.Fatal("encode error: ", err)
	}
	if !bytes.Equal(ourBlockEnc, blockEnc) {
		t.Errorf("encoded block mismatch:\ngot:  %x\nwant: %x", ourBlockEnc, blockEnc)
	}
}

// Generated with the util method:
// createBlock -> CeloLegacy: 1, Legacy: 1, AccessListTxs: 1
func TestEIP2718BlockEncoding(t *testing.T) {
	blockEnc := common.FromHex("f90487f901d6a07285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7948888f1f195afa192cfee860698584c030f4c9db1a0ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217a097d7b55b3710c86f9b2b730bc89b11105b5054be68dd68f0bbf8fa07adcbf8c3a0847915d751ccc48f4279423a2823454e02966e5fa75f0e014a1902953b9b1a13b90100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008230398301ffb8845c47775cad636f6f6c65737420626c6f636b206f6e20636861696e00000000000000000000ccc0c08080c3808080c3808080f90264f8c980830493e082520894095e7baea6a6c7c4c2dfeb977efac326af552d8701b86400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000820a95a0f90778ea061aa025db0d925b30919792693e88635f29a16eb7bc65c9994cc65da032cd91bcdeb617ca60cf35c3b71edc0210c40302b4189ab97becd43bffce0437f8f601830493e082c3509400000000000000000000000000000000000000009400000000000000000000000000000000000000008203e894095e7baea6a6c7c4c2dfeb977efac326af552d8702b86400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000820a96a0f096036fb807799acf7af01431ef0bdfcf9d6d620cbcd2054f16daf92bcbe446a03d4187886e752a90346b002c3d21d14432e2222dc83680b1b1d93ee44272d3aab89f01f89c820539800a82ea6094095e7baea6a6c7c4c2dfeb977efac326af552d878080f838f7940000000000000000000000000000000000000001e1a0000000000000000000000000000000000000000000000000000000000000000080a09075e5c27f8c4f94233557ab376679136440fa775f6362b84ae5c73975acbb91a05fff437e4ae943f928cf7f64c6653b66a09281e9ebc1d1e25c55f6cf85f9421df842a00000000000000000000000000000000000000000000000000000000000000000a00000000000000000000000000000000000000000000000000000000000000000c28080")
	var block Block
	if err := rlp.DecodeBytes(blockEnc, &block); err != nil {
		t.Fatal("decode error: ", err)
	}

	check := func(f string, got, want interface{}) {
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s mismatch: got %v, want %v", f, got, want)
		}
	}
	check("GasUsed", block.GasUsed(), uint64(131000))
	check("Coinbase", block.Coinbase(), common.HexToAddress("8888f1f195afa192cfee860698584c030f4c9db1"))
	check("Root", block.Root(), common.HexToHash("ecc60e00b3fe5ce9f6e1a10e5469764daf51f1fe93c22ec3f9a7583a80357217"))
	check("Hash", block.Hash(), common.HexToHash("03cd004e3f7584b1d353e1d9463af39b7d374e3648c40b637d6141fa9bd5ac93"))
	check("Time", block.Time(), uint64(1548187484))
	check("Size", block.Size(), common.StorageSize(len(blockEnc)))
	check("ParentHash", block.ParentHash(), common.HexToHash("7285abd5b24742f184ad676e31f6054663b3529bc35ea2fcad8a3e0f642a46f7"))
	check("Extra", block.Extra(), emptyIstanbulExtra([]byte("coolest block on chain")))

	check("len(Transactions)", len(block.Transactions()), 3)
	check("Transactions[0].Hash", block.Transactions()[0].Hash(), common.HexToHash("9ea1cdf3060d2189059fbc1bc424603e19ee3292392f9e1c2a416a0b6d25ab6b"))
	check("Transactions[1].Hash", block.Transactions()[1].Hash(), common.HexToHash("923022ffa45a9a45a0308948e4454e1a0d59195d75c6166097bacc64ab863dcf"))
	check("Transactions[2].Hash", block.Transactions()[2].Hash(), common.HexToHash("fd80f6fe04308bb2a9be1600247196340b93119f9d76c257da19dd4c8d724ad5"))
	check("Transactions[2].Type()", block.Transactions()[2].Type(), uint8(AccessListTxType))

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

func createBlock(legacyTxs, celoLegacyTxs, accessListTxs, dynamicFeeTxs, celoDynamicFeeTxs uint) {
	var (
		key, _     = crypto.GenerateKey()
		txs        = make([]*Transaction, legacyTxs+celoLegacyTxs+accessListTxs+dynamicFeeTxs+celoDynamicFeeTxs)
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
		if legacyTxs > 0 {
			tx = NewTransaction(uint64(i), to, amount, 21000, price, data)
			legacyTxs -= 1
		} else if celoLegacyTxs > 0 {
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
			} else if celoDynamicFeeTxs > 0 {
				tx = NewTx(&CeloDynamicFeeTx{
					ChainID:             chainID,
					Nonce:               0,
					To:                  &to,
					Gas:                 75000,
					GasFeeCap:           big.NewInt(10),
					GasTipCap:           big.NewInt(0),
					FeeCurrency:         &common.ZeroAddress,
					GatewayFeeRecipient: &common.ZeroAddress,
					GatewayFee:          big.NewInt(1000),
					AccessList:          accesses,
					Data:                []byte{},
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
