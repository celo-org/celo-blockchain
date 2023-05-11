package vm

import (
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
