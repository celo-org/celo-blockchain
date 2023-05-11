package vm

import (
	"encoding/json"
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/crypto"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
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
	Transfer: func(s StateDB, a1, a2 common.Address, i *big.Int) { panic("transfer: not implemented") },
	GetHash:  func(u uint64) common.Hash { panic("getHash: not implemented") },
	VerifySeal: func(header *types.Header) bool {
		// If the block is later than the unsealed reference block, return false.
		return !(header.Number.Cmp(testHeader.Number) > 0)
	},
	Coinbase:    common.Address{},
	BlockNumber: new(big.Int).Set(testHeader.Number),
	Time:        new(big.Int).SetUint64(testHeader.Time),

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

// Tests the sample inputs from the ed25519 verify check CIP 25
func TestPrecompiledEd25519Verify(t *testing.T) { testJson("ed25519Verify", "f3", t) }

// Benchmarks the sample inputs from the ed25519 verify check CIP 25
func BenchmarkPrecompiledEd25519Verify(b *testing.B) { benchJson("ed25519Verify", "f3", b) }

// Tests sample inputs for fractionMulExp
// NOTE: This currently only verifies that inputs of invalid length are rejected
func TestPrecompiledFractionMulExp(t *testing.T) {
	// Post GFork behaviour
	mockEVM.chainRules.IsGFork = true
	testJson("fractionMulExp", "fc", t)
	// Pre GFork behaviour
	mockEVM.chainRules.IsGFork = false
	testJson("fractionMulExpOld", "fc", t)
}

// Tests sample inputs for proofOfPossession
// NOTE: This currently only verifies that inputs of invalid length are rejected
func TestPrecompiledProofOfPossession(t *testing.T) { testJson("proofOfPossession", "fb", t) }

// Tests sample inputs for getValidator
func TestGetValidator(t *testing.T) { testJson("getValidator", "fa", t) }

func TestGetValidatorBLSPublicKey(t *testing.T) { testJson("getValidatorBLSPublicKey", "e1", t) }

// Tests sample inputs for numberValidators
func TestNumberValidators(t *testing.T) { testJson("numberValidators", "f9", t) }

// Tests sample inputs for getBlockNumberFromHeader
func TestGetBlockNumberFromHeader(t *testing.T) { testJson("blockNumberFromHeader", "f7", t) }

// Tests sample inputs for hashHeader
func TestPrecompiledHashHeader(t *testing.T) { testJson("hashHeader", "f6", t) }

// Tests sample inputs for getParentSealBitmapTests
func TestGetParentSealBitmap(t *testing.T) { testJson("getParentSealBitmap", "f5", t) }

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
	testJson("bls12377G1Add_matter", "e9", t)
	testJson("bls12377G1Add_zexe", "e9", t)
}

func TestPrecompiledBLS12377G1Mul(t *testing.T) {
	testJson("bls12377G1Mul_matter", "e8", t)
	testJson("bls12377G1Mul_zexe", "e8", t)
}

func TestPrecompiledBLS12377G1ZMultiExp(t *testing.T) {
	testJson("bls12377G1MultiExp_matter", "e7", t)
	testJson("bls12377G1MultiExp_zexe", "e7", t)
}

func TestPrecompiledBLS12377G2Add(t *testing.T) {
	testJson("bls12377G2Add_matter", "e6", t)
	testJson("bls12377G2Add_zexe", "e6", t)
}

func TestPrecompiledBLS12377G2Mul(t *testing.T) {
	testJson("bls12377G2Mul_matter", "e5", t)
	testJson("bls12377G2Mul_zexe", "e5", t)
}

func TestPrecompiledBLS12377G2MultiExp(t *testing.T) {
	testJson("bls12377G2MultiExp_matter", "e4", t)
	testJson("bls12377G2MultiExp_zexe", "e4", t)
}

func TestPrecompiledBLS12377Pairing(t *testing.T) {
	testJson("bls12377Pairing_matter", "e3", t)
	testJson("bls12377Pairing_zexe", "e3", t)
}

func TestPrecompiledBLS12377G1AddFail(t *testing.T) {
	testJsonFail("bls12377G1Add", "e9", t)
}

func TestPrecompiledBLS12377G1MulFail(t *testing.T) {
	testJsonFail("bls12377G1Mul", "e8", t)
}

func TestPrecompiledBLS12377G1MultiexpFail(t *testing.T) {
	testJsonFail("bls12377G1Multiexp", "e7", t)
}

func TestPrecompiledBLS12377G2AddFail(t *testing.T) {
	testJsonFail("bls12377G2Add", "e6", t)
}

func TestPrecompiledBLS12377G2MulFail(t *testing.T) {
	testJsonFail("bls12377G2Mul", "e5", t)
}

func TestPrecompiledBLS12377G2MultiexpFail(t *testing.T) {
	testJsonFail("bls12377G2Multiexp", "e4", t)
}

func TestPrecompiledBLS12377PairingFail(t *testing.T) {
	testJsonFail("bls12377Pairing", "e3", t)
}
