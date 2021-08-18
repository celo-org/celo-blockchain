// Copyright 2020 The go-ethereum Authors
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

package bls

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
	"golang.org/x/crypto/sha3"
)

const (
	blsG1Add      = byte(10)
	blsG1Mul      = byte(11)
	blsG1MultiExp = byte(12)
	blsG2Add      = byte(13)
	blsG2Mul      = byte(14)
	blsG2MultiExp = byte(15)
	blsPairing    = byte(16)
	blsMapG1      = byte(17)
	blsMapG2      = byte(18)
)

func getValidators(number *big.Int, _ common.Hash) []istanbul.Validator {
	preimage := append([]byte("fakevalidators"), common.LeftPadBytes(number.Bytes()[:], 32)...)
	hash := sha3.Sum256(preimage)
	var validators []istanbul.Validator
	for i := 0; i < 16; i, hash = i+1, sha3.Sum256(hash[:]) {
		key, _ := crypto.ToECDSA(hash[:])
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(key)
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		addr := crypto.PubkeyToAddress(key.PublicKey)
		validators = append(validators, validator.New(addr, blsPublicKey))
	}
	return validators
}

func makeTestSeal(number *big.Int) types.IstanbulAggregatedSeal {
	preimage := append([]byte("fakeseal"), common.LeftPadBytes(number.Bytes()[:], 32)...)
	hash := sha3.Sum256(preimage)
	return types.IstanbulAggregatedSeal{Bitmap: new(big.Int).SetBytes(hash[:2])}
}

func makeTestHeaderHash(number *big.Int) common.Hash {
	preimage := append([]byte("fakeheader"), common.LeftPadBytes(number.Bytes()[:], 32)...)
	return common.Hash(sha3.Sum256(preimage))
}

func makeTestHeaderExtra(number *big.Int) *types.IstanbulExtra {
	return &types.IstanbulExtra{
		AggregatedSeal:       makeTestSeal(number),
		ParentAggregatedSeal: makeTestSeal(new(big.Int).Sub(number, common.Big1)),
	}
}

func makeTestHeader(number *big.Int) *types.Header {
	extra, err := rlp.EncodeToBytes(makeTestHeaderExtra(number))
	if err != nil {
		panic(err)
	}
	return &types.Header{
		ParentHash: makeTestHeaderHash(new(big.Int).Sub(number, common.Big1)),
		Number:     number,
		GasUsed:    params.DefaultGasLimit / 2,
		Extra:      append(make([]byte, types.IstanbulExtraVanity), extra...),
		Time:       number.Uint64() * 5,
	}
}

func getHeaderByNumber(number uint64) *types.Header {
	return makeTestHeader(new(big.Int).SetUint64(number))
}

var testHeader = makeTestHeader(big.NewInt(10000))

var vmBlockCtx = vm.BlockContext{
	CanTransfer: func(db vm.StateDB, addr common.Address, amount *big.Int) bool {
		return db.GetBalance(addr).Cmp(amount) >= 0
	},
	Transfer: func(e *vm.EVM, a1, a2 common.Address, i *big.Int) { panic("transfer: not implemented") },
	GetHash:  func(u uint64) common.Hash { panic("getHash: not implemented") },
	VerifySeal: func(header *types.Header) bool {
		// If the block is later than the unsealed reference block, return false.
		return !(header.Number.Cmp(testHeader.Number) > 0)
	},
	Coinbase:    common.Address{},
	BlockNumber: new(big.Int).Set(testHeader.Number),
	Time:        new(big.Int).SetUint64(testHeader.Time),

	GetRegisteredAddress: func(evm *vm.EVM, registryId common.Hash) (common.Address, error) {
		return common.ZeroAddress, errors.New("not implemented: GetAddressFromRegistry")
	},

	EpochSize:         100,
	GetValidators:     getValidators,
	GetHeaderByNumber: getHeaderByNumber,
}

var vmTxCtx = vm.TxContext{
	GasPrice: common.Big1,
	Origin:   common.HexToAddress("a11ce"),
}

// Create a global mock EVM for use in the following tests.
var mockEVM = &vm.EVM{
	Context:   vmBlockCtx,
	TxContext: vmTxCtx,
}

func FuzzG1Add(data []byte) int      { return fuzz(blsG1Add, data) }
func FuzzG1Mul(data []byte) int      { return fuzz(blsG1Mul, data) }
func FuzzG1MultiExp(data []byte) int { return fuzz(blsG1MultiExp, data) }
func FuzzG2Add(data []byte) int      { return fuzz(blsG2Add, data) }
func FuzzG2Mul(data []byte) int      { return fuzz(blsG2Mul, data) }
func FuzzG2MultiExp(data []byte) int { return fuzz(blsG2MultiExp, data) }
func FuzzPairing(data []byte) int    { return fuzz(blsPairing, data) }
func FuzzMapG1(data []byte) int      { return fuzz(blsMapG1, data) }
func FuzzMapG2(data []byte) int      { return fuzz(blsMapG2, data) }

func checkInput(id byte, inputLen int) bool {
	switch id {
	case blsG1Add:
		return inputLen == 256
	case blsG1Mul:
		return inputLen == 160
	case blsG1MultiExp:
		return inputLen%160 == 0
	case blsG2Add:
		return inputLen == 512
	case blsG2Mul:
		return inputLen == 288
	case blsG2MultiExp:
		return inputLen%288 == 0
	case blsPairing:
		return inputLen%384 == 0
	case blsMapG1:
		return inputLen == 64
	case blsMapG2:
		return inputLen == 128
	}
	panic("programmer error")
}

// The fuzzer functions must return
// 1 if the fuzzer should increase priority of the
//    given input during subsequent fuzzing (for example, the input is lexically
//    correct and was parsed successfully);
// -1 if the input must not be added to corpus even if gives new coverage; and
// 0  otherwise
// other values are reserved for future use.
func fuzz(id byte, data []byte) int {
	// Even on bad input, it should not crash, so we still test the gas calc
<<<<<<< HEAD:tests/fuzzers/bls12381/bls_fuzzer.go
	// precompile := vm.PrecompiledContractsYoloV2[common.BytesToAddress([]byte{id})] //?
	precompile := vm.PrecompiledContractsDonut[common.BytesToAddress([]byte{id})]
||||||| e78727290:tests/fuzzers/bls12381/bls_fuzzer.go
	precompile := vm.PrecompiledContractsYoloV2[common.BytesToAddress([]byte{id})]
=======
	precompile := vm.PrecompiledContractsBLS[common.BytesToAddress([]byte{id})]
>>>>>>> v1.10.7:tests/fuzzers/bls12381/precompile_fuzzer.go
	gas := precompile.RequiredGas(data)
	if !checkInput(id, len(data)) {
		return 0
	}
	// If the gas cost is too large (25M), bail out
	if gas > 25*1000*1000 {
		return 0
	}
	cpy := make([]byte, len(data))
	copy(cpy, data)
	_, err := precompile.Run(cpy, common.HexToAddress("1337"), mockEVM)
	if !bytes.Equal(cpy, data) {
		panic(fmt.Sprintf("input data modified, precompile %d: %x %x", id, data, cpy))
	}
	if err != nil {
		return 0
	}
	return 1
}
