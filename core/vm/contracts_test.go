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

package vm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"testing"
	"time"

	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
	"golang.org/x/crypto/sha3"
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

var vmBlockCtx = BlockContext{
	CanTransfer: func(db StateDB, addr common.Address, amount *big.Int) bool {
		return db.GetBalance(addr).Cmp(amount) >= 0
	},
	Transfer: func(e *EVM, a1, a2 common.Address, i *big.Int) { panic("transfer: not implemented") },
	GetHash:  func(u uint64) common.Hash { panic("getHash: not implemented") },
	VerifySeal: func(header *types.Header) bool {
		// If the block is later than the unsealed reference block, return false.
		return !(header.Number.Cmp(testHeader.Number) > 0)
	},
	Coinbase:    common.Address{},
	BlockNumber: new(big.Int).Set(testHeader.Number),
	Time:        new(big.Int).SetUint64(testHeader.Time),

	GetRegisteredAddress: func(evm *EVM, registryId common.Hash) (common.Address, error) {
		return common.ZeroAddress, errors.New("not implemented: GetAddressFromRegistry")
	},

	EpochSize:         100,
	GetValidators:     getValidators,
	GetHeaderByNumber: getHeaderByNumber,
}

var vmTxCtx = TxContext{
	GasPrice: common.Big1,
	Origin:   common.HexToAddress("a11ce"),
}

// Create a global mock EVM for use in the following tests.
var mockEVM = &EVM{
	Context:   vmBlockCtx,
	TxContext: vmTxCtx,
}

func loadJSON(name string) ([]precompiledTest, error) {
	data, err := ioutil.ReadFile(fmt.Sprintf("testdata/precompiles/%v.json", name))
	if err != nil {
		return nil, err
	}
	var testcases []precompiledTest
	err = json.Unmarshal(data, &testcases)
	return testcases, err
}

func loadJSONFail(name string) ([]precompiledFailureTest, error) {
	data, err := ioutil.ReadFile(fmt.Sprintf("testdata/precompiles/%v.json", name))
	if err != nil {
		return nil, err
	}
	var testcases []precompiledFailureTest
	err = json.Unmarshal(data, &testcases)
	return testcases, err
}

// precompiledTest defines the input/output pairs for precompiled contract tests.
type precompiledTest struct {
	Input, Expected string
	Gas             uint64
	Name            string
	NoBenchmark     bool // Benchmark primarily the worst-cases
	ErrorExpected   bool
}

// precompiledFailureTest defines the input/error pairs for precompiled
// contract failure tests.
type precompiledFailureTest struct {
	Input         string
	ExpectedError string
	Name          string
}

// allPrecompiles does not map to the actual set of precompiles, as it also contains
// repriced versions of precompiles at certain slots
var allPrecompiles = map[common.Address]PrecompiledContract{
	common.BytesToAddress([]byte{1}):          &ecrecover{},
	common.BytesToAddress([]byte{2}):          &sha256hash{},
	common.BytesToAddress([]byte{3}):          &ripemd160hash{},
	common.BytesToAddress([]byte{4}):          &dataCopy{},
	common.BytesToAddress([]byte{5}):          &bigModExp{eip2565: false},
	common.BytesToAddress([]byte{0xf1, 0xf5}): &bigModExp{eip2565: true}, // "f1f5" otherwise "f5" collides with our precompile (getParentSealBitmapAddress)
	common.BytesToAddress([]byte{6}):          &bn256AddIstanbul{},
	common.BytesToAddress([]byte{7}):          &bn256ScalarMulIstanbul{},
	common.BytesToAddress([]byte{8}):          &bn256PairingIstanbul{},
	common.BytesToAddress([]byte{9}):          &blake2F{},
	common.BytesToAddress([]byte{10}):         &bls12381G1Add{},
	common.BytesToAddress([]byte{11}):         &bls12381G1Mul{},
	common.BytesToAddress([]byte{12}):         &bls12381G1MultiExp{},
	common.BytesToAddress([]byte{13}):         &bls12381G2Add{},
	common.BytesToAddress([]byte{14}):         &bls12381G2Mul{},
	common.BytesToAddress([]byte{15}):         &bls12381G2MultiExp{},
	common.BytesToAddress([]byte{16}):         &bls12381Pairing{},
	common.BytesToAddress([]byte{17}):         &bls12381MapG1{},
	common.BytesToAddress([]byte{18}):         &bls12381MapG2{},

	// Celo Precompiled Contracts
	transferAddress:              &transfer{},
	fractionMulExpAddress:        &fractionMulExp{},
	proofOfPossessionAddress:     &proofOfPossession{},
	getValidatorAddress:          &getValidator{},
	numberValidatorsAddress:      &numberValidators{},
	epochSizeAddress:             &epochSize{},
	blockNumberFromHeaderAddress: &blockNumberFromHeader{},
	hashHeaderAddress:            &hashHeader{},
	getParentSealBitmapAddress:   &getParentSealBitmap{},
	getVerifiedSealBitmapAddress: &getVerifiedSealBitmap{},

	// New in Donut hard fork
	ed25519Address:           &ed25519Verify{},
	b12_381G1AddAddress:      &bls12381G1Add{},
	b12_381G1MulAddress:      &bls12381G1Mul{},
	b12_381G1MultiExpAddress: &bls12381G1MultiExp{},
	b12_381G2AddAddress:      &bls12381G2Add{},
	b12_381G2MulAddress:      &bls12381G2Mul{},
	b12_381G2MultiExpAddress: &bls12381G2MultiExp{},
	b12_381PairingAddress:    &bls12381Pairing{},
	b12_381MapFpToG1Address:  &bls12381MapG1{},
	b12_381MapFp2ToG2Address: &bls12381MapG2{},
	b12_377G1AddAddress:      &bls12377G1Add{},
	b12_377G1MulAddress:      &bls12377G1Mul{},
	b12_377G1MultiExpAddress: &bls12377G1MultiExp{},
	b12_377G2AddAddress:      &bls12377G2Add{},
	b12_377G2MulAddress:      &bls12377G2Mul{},
	b12_377G2MultiExpAddress: &bls12377G2MultiExp{},
	b12_377PairingAddress:    &bls12377Pairing{},
	cip20Address:             &cip20HashFunctions{Cip20HashesDonut},
	cip26Address:             &getValidatorBLS{},
}

func testJSON(name, addr string, t *testing.T) {
	tests, err := loadJSON(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		testPrecompiled(addr, test, t)
	}
}

func testJSONFail(name, addr string, t *testing.T) {
	tests, err := loadJSONFail(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		testPrecompiledFailure(addr, test, t)
	}
}

func benchJSON(name, addr string, b *testing.B) {
	tests, err := loadJSON(name)
	if err != nil {
		b.Fatal(err)
	}
	for _, test := range tests {
		benchmarkPrecompiled(addr, test, b)
	}
}

// EIP-152 test vectors
var blake2FMalformedInputTests = []precompiledFailureTest{
	{
		Input:         "",
		ExpectedError: errBlake2FInvalidInputLength.Error(),
		Name:          "vector 0: empty input",
	},
	{
		Input:         "00000c48c9bdf267e6096a3ba7ca8485ae67bb2bf894fe72f36e3cf1361d5f3af54fa5d182e6ad7f520e511f6c3e2b8c68059b6bbd41fbabd9831f79217e1319cde05b61626300000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000300000000000000000000000000000001",
		ExpectedError: errBlake2FInvalidInputLength.Error(),
		Name:          "vector 1: less than 213 bytes input",
	},
	{
		Input:         "000000000c48c9bdf267e6096a3ba7ca8485ae67bb2bf894fe72f36e3cf1361d5f3af54fa5d182e6ad7f520e511f6c3e2b8c68059b6bbd41fbabd9831f79217e1319cde05b61626300000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000300000000000000000000000000000001",
		ExpectedError: errBlake2FInvalidInputLength.Error(),
		Name:          "vector 2: more than 213 bytes input",
	},
	{
		Input:         "0000000c48c9bdf267e6096a3ba7ca8485ae67bb2bf894fe72f36e3cf1361d5f3af54fa5d182e6ad7f520e511f6c3e2b8c68059b6bbd41fbabd9831f79217e1319cde05b61626300000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000300000000000000000000000000000002",
		ExpectedError: errBlake2FInvalidFinalFlag.Error(),
		Name:          "vector 3: malformed final block indicator flag",
	},
}

var getVerifiedSealBitmapTests = []precompiledTest{
	// Input is a block header. Output is bitmap.
	{
		Input:         "",
		Expected:      "unable to decode input",
		Name:          "input_invalid_empty",
		ErrorExpected: true,
	},
	{
		Input: func() string {
			header := makeTestHeader(common.Big1)
			encoded, _ := rlp.EncodeToBytes(header)
			return hexutil.Encode(encoded)[2:]
		}(),
		Expected: "0000000000000000000000000000000000000000000000000000000000007b1d",
		Name:     "correct_verified_header",
	},
	{
		Input: func() string {
			header := makeTestHeader(common.Big1)
			header.Extra = nil
			encoded, _ := rlp.EncodeToBytes(header)
			return hexutil.Encode(encoded)[2:]
		}(),
		Expected:      "blockchain engine incompatible with request",
		Name:          "input_incompatible_engine",
		ErrorExpected: true,
	},
}

func testPrecompiled(addr string, test precompiledTest, t *testing.T) {
	p := allPrecompiles[common.HexToAddress(addr)]
	in := common.Hex2Bytes(test.Input)
	gas := p.RequiredGas(in)
	t.Run(fmt.Sprintf("%s-Gas=%d", test.Name, gas), func(t *testing.T) {
		res, _, err := RunPrecompiledContract(p, in, gas, common.HexToAddress("1337"), mockEVM)
		if test.ErrorExpected {
			if err == nil {
				t.Errorf("Expected error: %v, but no error occurred", test.Expected)
			} else if err.Error() != test.Expected {
				t.Errorf("Expected error: \"%v\", but got \"%v\"", test.Expected, err.Error())
			}
		} else {
			if err != nil {
				t.Error(err)
			} else if common.Bytes2Hex(res) != test.Expected {
				t.Errorf("Expected %v, got %v", test.Expected, common.Bytes2Hex(res))
			}
			// TODO: Calculate and add our actual gas to every json file
			// if expGas := test.Gas; expGas != gas {
			// 	t.Errorf("%v: gas wrong, expected %d, got %d", test.Name, expGas, gas)
			// }
		}
		// Verify that the precompile did not touch the input buffer
		exp := common.Hex2Bytes(test.Input)
		if !bytes.Equal(in, exp) {
			t.Errorf("Precompiled %v modified input data", addr)
		}
	})
}

func testPrecompiledOOG(addr string, test precompiledTest, t *testing.T) {
	p := allPrecompiles[common.HexToAddress(addr)]
	in := common.Hex2Bytes(test.Input)
	gas := p.RequiredGas(in) - 1
	t.Run(fmt.Sprintf("%s-Gas=%d", test.Name, gas), func(t *testing.T) {
		_, _, err := RunPrecompiledContract(p, in, gas, common.HexToAddress("1337"), mockEVM)
		if err.Error() != "out of gas" {
			t.Errorf("Expected error [out of gas], got [%v]", err)
		}
		// Verify that the precompile did not touch the input buffer
		exp := common.Hex2Bytes(test.Input)
		if !bytes.Equal(in, exp) {
			t.Errorf("Precompiled %v modified input data", addr)
		}
	})
}

func testPrecompiledFailure(addr string, test precompiledFailureTest, t *testing.T) {
	p := allPrecompiles[common.HexToAddress(addr)]
	in := common.Hex2Bytes(test.Input)
	gas := p.RequiredGas(in)
	t.Run(test.Name, func(t *testing.T) {
		_, _, err := RunPrecompiledContract(p, in, gas, common.HexToAddress("1337"), mockEVM)
		if err.Error() != test.ExpectedError {
			t.Errorf("Expected error [%v], got [%v]", test.ExpectedError, err)
		}
		// Verify that the precompile did not touch the input buffer
		exp := common.Hex2Bytes(test.Input)
		if !bytes.Equal(in, exp) {
			t.Errorf("Precompiled %v modified input data", addr)
		}
	})
}

func benchmarkPrecompiled(addr string, test precompiledTest, bench *testing.B) {
	if test.NoBenchmark {
		return
	}
	p := allPrecompiles[common.HexToAddress(addr)]
	in := common.Hex2Bytes(test.Input)
	reqGas := p.RequiredGas(in)

	var (
		res  []byte
		err  error
		data = make([]byte, len(in))
	)

	bench.Run(fmt.Sprintf("%s-Gas=%d", test.Name, reqGas), func(bench *testing.B) {
		bench.ReportAllocs()
		start := time.Now()
		bench.ResetTimer()
		for i := 0; i < bench.N; i++ {
			copy(data, in)
			res, _, err = RunPrecompiledContract(p, data, reqGas, common.HexToAddress("1337"), mockEVM)
		}
		bench.StopTimer()
		elapsed := uint64(time.Since(start))
		if elapsed < 1 {
			elapsed = 1
		}
		gasUsed := reqGas * uint64(bench.N)
		bench.ReportMetric(float64(reqGas), "gas/op")
		// Keep it as uint64, multiply 100 to get two digit float later
		mgasps := (100 * 1000 * gasUsed) / elapsed
		bench.ReportMetric(float64(mgasps)/100, "mgas/s")
		//Check if it is correct
		if err != nil {
			bench.Error(err)
			return
		}
		if common.Bytes2Hex(res) != test.Expected {
			bench.Errorf("Expected %v, got %v", test.Expected, common.Bytes2Hex(res))
			return
		}
	})
}

// Benchmarks the sample inputs from the ECRECOVER precompile.
func BenchmarkPrecompiledEcrecover(bench *testing.B) {
	t := precompiledTest{
		Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
		Expected: "000000000000000000000000ceaccac640adf55b2028469bd36ba501f28b699d",
		Name:     "",
	}
	benchmarkPrecompiled("01", t, bench)
}

// Benchmarks the sample inputs from the SHA256 precompile.
func BenchmarkPrecompiledSha256(bench *testing.B) {
	t := precompiledTest{
		Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
		Expected: "811c7003375852fabd0d362e40e68607a12bdabae61a7d068fe5fdd1dbbf2a5d",
		Name:     "128",
	}
	benchmarkPrecompiled("02", t, bench)
}

// Benchmarks the sample inputs from the RIPEMD precompile.
func BenchmarkPrecompiledRipeMD(bench *testing.B) {
	t := precompiledTest{
		Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
		Expected: "0000000000000000000000009215b8d9882ff46f0dfde6684d78e831467f65e6",
		Name:     "128",
	}
	benchmarkPrecompiled("03", t, bench)
}

// Benchmarks the sample inputs from the identiy precompile.
func BenchmarkPrecompiledIdentity(bench *testing.B) {
	t := precompiledTest{
		Input:    "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
		Expected: "38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e000000000000000000000000000000000000000000000000000000000000001b38d18acb67d25c8bb9942764b62f18e17054f66a817bd4295423adf9ed98873e789d1dd423d25f0772d2748d60f7e4b81bb14d086eba8e8e8efb6dcff8a4ae02",
		Name:     "128",
	}
	benchmarkPrecompiled("04", t, bench)
}

// Tests the sample inputs from the ModExp EIP 198.
func TestPrecompiledModExp(t *testing.T)      { testJSON("modexp", "05", t) }
func BenchmarkPrecompiledModExp(b *testing.B) { benchJSON("modexp", "05", b) }

func TestPrecompiledModExpEip2565(t *testing.T)      { testJSON("modexp_eip2565", "f1f5", t) }
func BenchmarkPrecompiledModExpEip2565(b *testing.B) { benchJSON("modexp_eip2565", "f1f5", b) }

// Tests the sample inputs from the elliptic curve addition EIP 213.
func TestPrecompiledBn256Add(t *testing.T)      { testJSON("bn256Add", "06", t) }
func BenchmarkPrecompiledBn256Add(b *testing.B) { benchJSON("bn256Add", "06", b) }

// Tests OOG
func TestPrecompiledModExpOOG(t *testing.T) {
	modexpTests, err := loadJSON("modexp")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range modexpTests {
		testPrecompiledOOG("05", test, t)
	}
}

// Tests the sample inputs from the elliptic curve scalar multiplication EIP 213.
func TestPrecompiledBn256ScalarMul(t *testing.T)      { testJSON("bn256ScalarMul", "07", t) }
func BenchmarkPrecompiledBn256ScalarMul(b *testing.B) { benchJSON("bn256ScalarMul", "07", b) }

// Tests the sample inputs from the elliptic curve pairing check EIP 197.
func TestPrecompiledBn256Pairing(t *testing.T)      { testJSON("bn256Pairing", "08", t) }
func BenchmarkPrecompiledBn256Pairing(b *testing.B) { benchJSON("bn256Pairing", "08", b) }

func TestPrecompiledBlake2F(t *testing.T)      { testJSON("blake2F", "09", t) }
func BenchmarkPrecompiledBlake2F(b *testing.B) { benchJSON("blake2F", "09", b) }

func TestPrecompileBlake2FMalformedInput(t *testing.T) {
	for _, test := range blake2FMalformedInputTests {
		testPrecompiledFailure("09", test, t)
	}
}

// Tests the sample inputs from the ed25519 verify check CIP 25
func TestPrecompiledEd25519Verify(t *testing.T) { testJSON("ed25519Verify", "f3", t) }

// Benchmarks the sample inputs from the ed25519 verify check CIP 25
func BenchmarkPrecompiledEd25519Verify(b *testing.B) { benchJSON("ed25519Verify", "f3", b) }

// Tests sample inputs for fractionMulExp
// NOTE: This currently only verifies that inputs of invalid length are rejected
func TestPrecompiledFractionMulExp(t *testing.T) { testJSON("fractionMulExp", "fc", t) }

// Tests sample inputs for proofOfPossession
// NOTE: This currently only verifies that inputs of invalid length are rejected
func TestPrecompiledProofOfPossession(t *testing.T) { testJSON("proofOfPossession", "fb", t) }

// Tests sample inputs for getValidator
func TestGetValidator(t *testing.T) { testJSON("getValidator", "fa", t) }

func TestGetValidatorBLSPublicKey(t *testing.T) { testJSON("getValidatorBLSPublicKey", "e1", t) }

// Tests sample inputs for numberValidators
func TestNumberValidators(t *testing.T) { testJSON("numberValidators", "f9", t) }

// Tests sample inputs for getBlockNumberFromHeader
func TestGetBlockNumberFromHeader(t *testing.T) { testJSON("blockNumberFromHeader", "f7", t) }

// Tests sample inputs for hashHeader
func TestPrecompiledHashHeader(t *testing.T) { testJSON("hashHeader", "f6", t) }

// Tests sample inputs for getParentSealBitmapTests
func TestGetParentSealBitmap(t *testing.T) { testJSON("getParentSealBitmap", "f5", t) }

// Tests sample inputs for getParentSealBitmapTests
func TestGetVerifiedSealBitmap(t *testing.T) {
	for _, test := range getVerifiedSealBitmapTests {
		testPrecompiled("f4", test, t)
	}
}

const defaultBlake2sConfig = "2000010100000000000000000000000000000000000000000000000000000000"

type cip20Test struct {
	Preimage  string `json:"preimage"`
	Sha2_512  string `json:"sha2_512"`
	Keccak512 string `json:"keccak512"`
	Sha3_256  string `json:"sha3_256"`
	Sha3_512  string `json:"sha3_512"`
	Blake2s   string `json:"blake2s"`
}

func loadCip20JSON() ([]cip20Test, error) {
	data, err := ioutil.ReadFile("testdata/precompiles/cip20.json")
	if err != nil {
		return nil, err
	}
	var tests []cip20Test
	err = json.Unmarshal(data, &tests)
	if err != nil {
		return nil, err
	}
	return tests, nil
}

func (c *cip20Test) toPrecompiledTests() []precompiledTest {
	return []precompiledTest{
		{
			Input:    "00" + c.Preimage,
			Expected: c.Sha3_256,
			Name:     "sha3_256 of 0x" + c.Preimage,
		},
		{
			Input:    "01" + c.Preimage,
			Expected: c.Sha3_512,
			Name:     "sha3_25sha3_5126 of 0x" + c.Preimage,
		},
		{
			Input:    "02" + c.Preimage,
			Expected: c.Keccak512,
			Name:     "keccak512 of 0x" + c.Preimage,
		},
		{
			Input:    "03" + c.Preimage,
			Expected: c.Sha2_512,
			Name:     "sha2_512 of 0x" + c.Preimage,
		},
		{
			Input:    "10" + defaultBlake2sConfig + c.Preimage,
			Expected: c.Blake2s,
			Name:     "blake2s of 0x" + c.Preimage,
		},
	}
}

var cip20Tests = []precompiledTest{
	{
		Input:    "10" + defaultBlake2sConfig,
		Expected: "69217a3079908094e11121d042354a7c1f55b6482ca1a51e1b250dfd1ed0eef9",
		Name:     "default blake2s of empty string",
	},
	{
		Input:    "10" + "20000101" + "00000000" + "00000000" + "60000000" + "0000000000000000" + "0000000000000000",
		Expected: "7a746244ad211d351f57a218255888174e719b54e683651e9314f55402eed414",
		Name:     "celo-bls-snark-rs test_crh_empty",
	},
}

func TestCip20(t *testing.T) {
	cip20ShaVariantTests, err := loadCip20JSON()
	if err != nil {
		t.Fatal(err)
	}

	for _, vector := range cip20ShaVariantTests {
		for _, test := range vector.toPrecompiledTests() {
			testPrecompiled("e2", test, t)
		}
	}

	for _, test := range cip20Tests {
		testPrecompiled("e2", test, t)
	}
}

func TestPrecompiledBLS12377G1Add(t *testing.T) {
	testJSON("bls12377G1Add_matter", "e9", t)
	testJSON("bls12377G1Add_zexe", "e9", t)
}

func TestPrecompiledBLS12377G1Mul(t *testing.T) {
	testJSON("bls12377G1Mul_matter", "e8", t)
	testJSON("bls12377G1Mul_zexe", "e8", t)
}

func TestPrecompiledBLS12377G1ZMultiExp(t *testing.T) {
	testJSON("bls12377G1MultiExp_matter", "e7", t)
	testJSON("bls12377G1MultiExp_zexe", "e7", t)
}

func TestPrecompiledBLS12377G2Add(t *testing.T) {
	testJSON("bls12377G2Add_matter", "e6", t)
	testJSON("bls12377G2Add_zexe", "e6", t)
}

func TestPrecompiledBLS12377G2Mul(t *testing.T) {
	testJSON("bls12377G2Mul_matter", "e5", t)
	testJSON("bls12377G2Mul_zexe", "e5", t)
}

func TestPrecompiledBLS12377G2MultiExp(t *testing.T) {
	testJSON("bls12377G2MultiExp_matter", "e4", t)
	testJSON("bls12377G2MultiExp_zexe", "e4", t)
}

func TestPrecompiledBLS12377Pairing(t *testing.T) {
	testJSON("bls12377Pairing_matter", "e3", t)
	testJSON("bls12377Pairing_zexe", "e3", t)
}

func TestPrecompiledBLS12377G1AddFail(t *testing.T) {
	testJSONFail("fail-bls12377G1Add", "e9", t)
}

func TestPrecompiledBLS12377G1MulFail(t *testing.T) {
	testJSONFail("fail-bls12377G1Mul", "e8", t)
}

func TestPrecompiledBLS12377G1MultiexpFail(t *testing.T) {
	testJSONFail("fail-bls12377G1Multiexp", "e7", t)
}

func TestPrecompiledBLS12377G2AddFail(t *testing.T) {
	testJSONFail("fail-bls12377G2Add", "e6", t)
}

func TestPrecompiledBLS12377G2MulFail(t *testing.T) {
	testJSONFail("fail-bls12377G2Mul", "e5", t)
}

func TestPrecompiledBLS12377G2MultiexpFail(t *testing.T) {
	testJSONFail("fail-bls12377G2Multiexp", "e4", t)
}

func TestPrecompiledBLS12377PairingFail(t *testing.T) {
	testJSONFail("fail-bls12377Pairing", "e3", t)
}

func TestPrecompiledBLS12381G1Add(t *testing.T)      { testJSON("blsG1Add", "f2", t) }
func TestPrecompiledBLS12381G1Mul(t *testing.T)      { testJSON("blsG1Mul", "f1", t) }
func TestPrecompiledBLS12381G1MultiExp(t *testing.T) { testJSON("blsG1MultiExp", "f0", t) }
func TestPrecompiledBLS12381G2Add(t *testing.T)      { testJSON("blsG2Add", "ef", t) }
func TestPrecompiledBLS12381G2Mul(t *testing.T)      { testJSON("blsG2Mul", "ee", t) }
func TestPrecompiledBLS12381G2MultiExp(t *testing.T) { testJSON("blsG2MultiExp", "ed", t) }
func TestPrecompiledBLS12381Pairing(t *testing.T)    { testJSON("blsPairing", "ec", t) }
func TestPrecompiledBLS12381MapG1(t *testing.T)      { testJSON("blsMapG1", "eb", t) }
func TestPrecompiledBLS12381MapG2(t *testing.T)      { testJSON("blsMapG2", "ea", t) }

func BenchmarkPrecompiledBLS12381G1Add(b *testing.B)      { benchJSON("blsG1Add", "0a", b) }
func BenchmarkPrecompiledBLS12381G1Mul(b *testing.B)      { benchJSON("blsG1Mul", "0b", b) }
func BenchmarkPrecompiledBLS12381G1MultiExp(b *testing.B) { benchJSON("blsG1MultiExp", "0c", b) }
func BenchmarkPrecompiledBLS12381G2Add(b *testing.B)      { benchJSON("blsG2Add", "0d", b) }
func BenchmarkPrecompiledBLS12381G2Mul(b *testing.B)      { benchJSON("blsG2Mul", "0e", b) }
func BenchmarkPrecompiledBLS12381G2MultiExp(b *testing.B) { benchJSON("blsG2MultiExp", "0f", b) }
func BenchmarkPrecompiledBLS12381Pairing(b *testing.B)    { benchJSON("blsPairing", "10", b) }
func BenchmarkPrecompiledBLS12381MapG1(b *testing.B)      { benchJSON("blsMapG1", "11", b) }
func BenchmarkPrecompiledBLS12381MapG2(b *testing.B)      { benchJSON("blsMapG2", "12", b) }

// Failure tests
func TestPrecompiledBLS12381G1AddFail(t *testing.T)      { testJSONFail("fail-blsG1Add", "f2", t) }
func TestPrecompiledBLS12381G1MulFail(t *testing.T)      { testJSONFail("fail-blsG1Mul", "f1", t) }
func TestPrecompiledBLS12381G1MultiExpFail(t *testing.T) { testJSONFail("fail-blsG1MultiExp", "f0", t) }
func TestPrecompiledBLS12381G2AddFail(t *testing.T)      { testJSONFail("fail-blsG2Add", "ef", t) }
func TestPrecompiledBLS12381G2MulFail(t *testing.T)      { testJSONFail("fail-blsG2Mul", "ee", t) }
func TestPrecompiledBLS12381G2MultiExpFail(t *testing.T) { testJSONFail("fail-blsG2MultiExp", "ed", t) }
func TestPrecompiledBLS12381PairingFail(t *testing.T)    { testJSONFail("fail-blsPairing", "ec", t) }
func TestPrecompiledBLS12381MapG1Fail(t *testing.T)      { testJSONFail("fail-blsMapG1", "eb", t) }
func TestPrecompiledBLS12381MapG2Fail(t *testing.T)      { testJSONFail("fail-blsMapG2", "ea", t) }

// BenchmarkPrecompiledBLS12381G1MultiExpWorstCase benchmarks the worst case we could find that still fits a gaslimit of 10MGas.
func BenchmarkPrecompiledBLS12381G1MultiExpWorstCase(b *testing.B) {
	task := "0000000000000000000000000000000008d8c4a16fb9d8800cce987c0eadbb6b3b005c213d44ecb5adeed713bae79d606041406df26169c35df63cf972c94be1" +
		"0000000000000000000000000000000011bc8afe71676e6730702a46ef817060249cd06cd82e6981085012ff6d013aa4470ba3a2c71e13ef653e1e223d1ccfe9" +
		"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"
	input := task
	for i := 0; i < 4787; i++ {
		input = input + task
	}
	testcase := precompiledTest{
		Input:       input,
		Expected:    "0000000000000000000000000000000005a6310ea6f2a598023ae48819afc292b4dfcb40aabad24a0c2cb6c19769465691859eeb2a764342a810c5038d700f18000000000000000000000000000000001268ac944437d15923dc0aec00daa9250252e43e4b35ec7a19d01f0d6cd27f6e139d80dae16ba1c79cc7f57055a93ff5",
		Name:        "WorstCaseG1",
		NoBenchmark: false,
	}
	benchmarkPrecompiled("0c", testcase, b)
}

// BenchmarkPrecompiledBLS12381G2MultiExpWorstCase benchmarks the worst case we could find that still fits a gaslimit of 10MGas.
func BenchmarkPrecompiledBLS12381G2MultiExpWorstCase(b *testing.B) {
	task := "000000000000000000000000000000000d4f09acd5f362e0a516d4c13c5e2f504d9bd49fdfb6d8b7a7ab35a02c391c8112b03270d5d9eefe9b659dd27601d18f" +
		"000000000000000000000000000000000fd489cb75945f3b5ebb1c0e326d59602934c8f78fe9294a8877e7aeb95de5addde0cb7ab53674df8b2cfbb036b30b99" +
		"00000000000000000000000000000000055dbc4eca768714e098bbe9c71cf54b40f51c26e95808ee79225a87fb6fa1415178db47f02d856fea56a752d185f86b" +
		"000000000000000000000000000000001239b7640f416eb6e921fe47f7501d504fadc190d9cf4e89ae2b717276739a2f4ee9f637c35e23c480df029fd8d247c7" +
		"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"
	input := task
	for i := 0; i < 1040; i++ {
		input = input + task
	}

	testcase := precompiledTest{
		Input:       input,
		Expected:    "0000000000000000000000000000000018f5ea0c8b086095cfe23f6bb1d90d45de929292006dba8cdedd6d3203af3c6bbfd592e93ecb2b2c81004961fdcbb46c00000000000000000000000000000000076873199175664f1b6493a43c02234f49dc66f077d3007823e0343ad92e30bd7dc209013435ca9f197aca44d88e9dac000000000000000000000000000000000e6f07f4b23b511eac1e2682a0fc224c15d80e122a3e222d00a41fab15eba645a700b9ae84f331ae4ed873678e2e6c9b000000000000000000000000000000000bcb4849e460612aaed79617255fd30c03f51cf03d2ed4163ca810c13e1954b1e8663157b957a601829bb272a4e6c7b8",
		Name:        "WorstCaseG2",
		NoBenchmark: false,
	}
	benchmarkPrecompiled("0f", testcase, b)
}
