package vm

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/common/math"
	"github.com/celo-org/celo-blockchain/core/types"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-bls-go/bls"
)

var (
	errBLS12377InvalidInputLength = errors.New("invalid input length")
	errBLS12377G1PointSubgroup    = errors.New("g1 point is not on correct subgroup")
	errBLS12377G2PointSubgroup    = errors.New("g2 point is not on correct subgroup")
)

type CeloPrecompiledContract interface {
	RequiredGas(input []byte) uint64                              // RequiredGas calculates the contract gas use
	Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) // Run runs the precompiled contract
}

type wrap struct {
	PrecompiledContract
}

func (pw *wrap) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {
	return pw.PrecompiledContract.Run(input)
}

type celoPrecompileContext struct {
	*BlockContext
	*params.Rules

	caller common.Address
	evm    *EVM
}

func NewContext(caller common.Address, evm *EVM) *celoPrecompileContext {
	return &celoPrecompileContext{
		BlockContext: &evm.Context,
		Rules:        &evm.chainRules,
		caller:       caller,
		evm:          evm,
	}
}

func celoPrecompileAddress(index byte) common.Address {
	celoPrecompiledContractsAddressOffset := byte(0xff)
	return common.BytesToAddress(append([]byte{0}, (celoPrecompiledContractsAddressOffset - index)))
}

// computes a * (b ^ exponent) to `decimals` places of precision, where a and b are fractions
type fractionMulExp struct{}

func max(x, y int64) int64 {
	if x < y {
		return y
	}
	return x
}

func (c *fractionMulExp) RequiredGas(input []byte) uint64 {
	if len(input) < 192 {
		return params.FractionMulExpGas
	}
	exponent, parsed := math.ParseBig256(hexutil.Encode(input[128:160]))
	if !parsed {
		return params.FractionMulExpGas
	}
	decimals, parsed := math.ParseBig256(hexutil.Encode(input[160:192]))
	if !parsed {
		return params.FractionMulExpGas
	}
	if !decimals.IsInt64() || !exponent.IsInt64() {
		return params.FractionMulExpGas
	}

	numbers := max(decimals.Int64(), exponent.Int64())

	if numbers > 100000 {
		return params.FractionMulExpGas
	}

	gas := params.FractionMulExpGas

	for numbers > 10 {
		gas = gas * 3
		numbers = numbers / 2
	}

	return gas
}

func (c *fractionMulExp) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {
	// input is comprised of 6 arguments:
	//   aNumerator:   32 bytes, 256 bit integer, numerator for the first fraction (a)
	//   aDenominator: 32 bytes, 256 bit integer, denominator for the first fraction (a)
	//   bNumerator:   32 bytes, 256 bit integer, numerator for the second fraction (b)
	//   bDenominator: 32 bytes, 256 bit integer, denominator for the second fraction (b)
	//   exponent:     32 bytes, 256 bit integer, exponent to raise the second fraction (b) to
	//   decimals:     32 bytes, 256 bit integer, places of precision
	//
	// 6 args x 32 bytes each = 192 bytes total input length
	if (ctx.IsGFork && len(input) != 192) || len(input) < 192 {
		return nil, ErrInputLength
	}

	parseErrorStr := "Error parsing input: unable to parse %s value from %s"

	aNumerator, parsed := math.ParseBig256(hexutil.Encode(input[0:32]))
	if !parsed {
		return nil, fmt.Errorf(parseErrorStr, "aNumerator", hexutil.Encode(input[0:32]))
	}

	aDenominator, parsed := math.ParseBig256(hexutil.Encode(input[32:64]))
	if !parsed {
		return nil, fmt.Errorf(parseErrorStr, "aDenominator", hexutil.Encode(input[32:64]))
	}

	bNumerator, parsed := math.ParseBig256(hexutil.Encode(input[64:96]))
	if !parsed {
		return nil, fmt.Errorf(parseErrorStr, "bNumerator", hexutil.Encode(input[64:96]))
	}

	bDenominator, parsed := math.ParseBig256(hexutil.Encode(input[96:128]))
	if !parsed {
		return nil, fmt.Errorf(parseErrorStr, "bDenominator", hexutil.Encode(input[96:128]))
	}

	exponent, parsed := math.ParseBig256(hexutil.Encode(input[128:160]))
	if !parsed {
		return nil, fmt.Errorf(parseErrorStr, "exponent", hexutil.Encode(input[128:160]))
	}

	decimals, parsed := math.ParseBig256(hexutil.Encode(input[160:192]))
	if !parsed {
		return nil, fmt.Errorf(parseErrorStr, "decimals", hexutil.Encode(input[160:192]))
	}

	// Handle passing of zero denominators
	if aDenominator == big.NewInt(0) || bDenominator == big.NewInt(0) {
		return nil, fmt.Errorf("Input Error: Denominator of zero provided!")
	}

	if !decimals.IsInt64() || !exponent.IsInt64() || max(decimals.Int64(), exponent.Int64()) > 100000 {
		return nil, fmt.Errorf("Input Error: Decimals or exponent too large")
	}

	numeratorExp := new(big.Int).Mul(aNumerator, new(big.Int).Exp(bNumerator, exponent, nil))
	denominatorExp := new(big.Int).Mul(aDenominator, new(big.Int).Exp(bDenominator, exponent, nil))

	decimalAdjustment := new(big.Int).Exp(big.NewInt(10), decimals, nil)

	numeratorDecimalAdjusted := new(big.Int).Div(new(big.Int).Mul(numeratorExp, decimalAdjustment), denominatorExp).Bytes()
	denominatorDecimalAdjusted := decimalAdjustment.Bytes()

	numeratorPadded := common.LeftPadBytes(numeratorDecimalAdjusted, 32)
	denominatorPadded := common.LeftPadBytes(denominatorDecimalAdjusted, 32)

	return append(numeratorPadded, denominatorPadded...), nil
}

type proofOfPossession struct{}

func (c *proofOfPossession) RequiredGas(input []byte) uint64 {
	return params.ProofOfPossessionGas
}

func (c *proofOfPossession) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {
	// input is comprised of 3 arguments:
	//   address:   20 bytes, an address used to generate the proof-of-possession
	//   publicKey: 96 bytes, representing the public key (defined as a const in bls package)
	//   signature: 48 bytes, representing the signature on `address` (defined as a const in bls package)
	// the total length of input required is the sum of these constants
	if len(input) != common.AddressLength+blscrypto.PUBLICKEYBYTES+blscrypto.SIGNATUREBYTES {
		return nil, ErrInputLength
	}
	addressBytes := input[:common.AddressLength]

	publicKeyBytes := input[common.AddressLength : common.AddressLength+blscrypto.PUBLICKEYBYTES]
	publicKey, err := bls.DeserializePublicKeyCached(publicKeyBytes)
	if err != nil {
		return nil, err
	}
	defer publicKey.Destroy()

	signatureBytes := input[common.AddressLength+blscrypto.PUBLICKEYBYTES : common.AddressLength+blscrypto.PUBLICKEYBYTES+blscrypto.SIGNATUREBYTES]
	signature, err := bls.DeserializeSignature(signatureBytes)
	if err != nil {
		return nil, err
	}
	defer signature.Destroy()

	err = publicKey.VerifyPoP(addressBytes, signature)
	if err != nil {
		return nil, err
	}

	return true32Byte, nil
}

type getValidator struct{}

func (c *getValidator) RequiredGas(input []byte) uint64 {
	return params.GetValidatorGas
}

// Return the validators that are required to sign the given, possibly unsealed, block number. If this block is
// the last in an epoch, note that that may mean one or more of those validators may no longer be elected
// for subsequent blocks.
// WARNING: Validator set is always constructed from the canonical chain, therefore this precompile is undefined
// if the engine is aware of a chain with higher total difficulty.
func (c *getValidator) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {
	// input is comprised of two arguments:
	//   index: 32 byte integer representing the index of the validator to get
	//   blockNumber: 32 byte integer representing the block number to access
	if (ctx.IsGFork && len(input) != 64) || len(input) < 64 {
		return nil, ErrInputLength
	}

	index := new(big.Int).SetBytes(input[0:32])

	blockNumber := new(big.Int).SetBytes(input[32:64])
	if blockNumber.Cmp(common.Big0) == 0 {
		// Validator set for the genesis block is empty, so any index is out of bounds.
		return nil, ErrValidatorsOutOfBounds
	}
	if blockNumber.Cmp(ctx.BlockNumber) > 0 {
		return nil, ErrBlockNumberOutOfBounds
	}

	// Note: Passing empty hash as here as it is an extra expense and the hash is not actually used.
	validators := ctx.GetValidators(new(big.Int).Sub(blockNumber, common.Big1), common.Hash{})

	// Ensure index, which is guaranteed to be non-negative, is valid.
	if index.Cmp(big.NewInt(int64(len(validators)))) >= 0 {
		return nil, ErrValidatorsOutOfBounds
	}

	validatorAddress := validators[index.Uint64()].Address()
	addressBytes := common.LeftPadBytes(validatorAddress[:], 32)

	return addressBytes, nil
}

type getValidatorBLS struct{}

func (c *getValidatorBLS) RequiredGas(input []byte) uint64 {
	return params.GetValidatorBLSGas
}

func copyBLSNumber(result []byte, offset int, uncompressedBytes []byte, offset2 int) {
	for i := 0; i < 48; i++ {
		result[63-i+offset] = uncompressedBytes[i+offset2]
	}
}

// Return the validator BLS public key for the validator at given index. The public key is given in uncompressed format, 4*48 bytes.
func (c *getValidatorBLS) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {
	// input is comprised of two arguments:
	//   index: 32 byte integer representing the index of the validator to get
	//   blockNumber: 32 byte integer representing the block number to access
	if len(input) < 64 {
		return nil, ErrInputLength
	}

	index := new(big.Int).SetBytes(input[0:32])

	blockNumber := new(big.Int).SetBytes(input[32:64])
	if blockNumber.Cmp(common.Big0) == 0 {
		// Validator set for the genesis block is empty, so any index is out of bounds.
		return nil, ErrValidatorsOutOfBounds
	}
	if blockNumber.Cmp(ctx.BlockNumber) > 0 {
		return nil, ErrBlockNumberOutOfBounds
	}

	// Note: Passing empty hash as here as it is an extra expense and the hash is not actually used.
	validators := ctx.GetValidators(new(big.Int).Sub(blockNumber, common.Big1), common.Hash{})

	// Ensure index, which is guaranteed to be non-negative, is valid.
	if index.Cmp(big.NewInt(int64(len(validators)))) >= 0 {
		return nil, ErrValidatorsOutOfBounds
	}

	validator := validators[index.Uint64()]
	uncompressedBytes := validator.BLSPublicKeyUncompressed()
	if len(uncompressedBytes) != 192 {
		return nil, ErrUnexpected
	}

	result := make([]byte, 256)
	for i := 0; i < 256; i++ {
		result[i] = 0
	}

	copyBLSNumber(result, 0, uncompressedBytes, 0)
	copyBLSNumber(result, 64, uncompressedBytes, 48)
	copyBLSNumber(result, 128, uncompressedBytes, 96)
	copyBLSNumber(result, 192, uncompressedBytes, 144)

	return result, nil
}

type numberValidators struct{}

func (c *numberValidators) RequiredGas(input []byte) uint64 {
	return params.GetValidatorGas
}

// Return the number of validators that are required to sign this current, possibly unsealed, block. If this block is
// the last in an epoch, note that that may mean one or more of those validators may no longer be elected
// for subsequent blocks.
// WARNING: Validator set is always constructed from the canonical chain, therefore this precompile is undefined
// if the engine is aware of a chain with higher total difficulty.
func (c *numberValidators) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {
	// input is comprised of a single argument:
	//   blockNumber: 32 byte integer representing the block number to access
	if len(input) < 32 {
		return nil, ErrInputLength
	}

	blockNumber := new(big.Int).SetBytes(input[0:32])
	if blockNumber.Cmp(common.Big0) == 0 {
		// Genesis validator set is empty. Return 0.
		return make([]byte, 32), nil
	}
	if blockNumber.Cmp(ctx.BlockNumber) > 0 {
		return nil, ErrBlockNumberOutOfBounds
	}

	// Note: Passing empty hash as here as it is an extra expense and the hash is not actually used.
	validators := ctx.GetValidators(new(big.Int).Sub(blockNumber, common.Big1), common.Hash{})

	numberValidators := big.NewInt(int64(len(validators))).Bytes()
	numberValidatorsBytes := common.LeftPadBytes(numberValidators[:], 32)
	return numberValidatorsBytes, nil
}

type epochSize struct{}

func (c *epochSize) RequiredGas(input []byte) uint64 {
	return params.GetEpochSizeGas
}

func (c *epochSize) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {
	epochSize := new(big.Int).SetUint64(ctx.EpochSize).Bytes()
	epochSizeBytes := common.LeftPadBytes(epochSize[:], 32)

	return epochSizeBytes, nil
}

type blockNumberFromHeader struct{}

func (c *blockNumberFromHeader) RequiredGas(input []byte) uint64 {
	return params.GetBlockNumberFromHeaderGas
}

func (c *blockNumberFromHeader) Run(input []byte) ([]byte, error) {
	var header types.Header
	err := rlp.DecodeBytes(input, &header)
	if err != nil {
		return nil, ErrInputDecode
	}

	blockNumber := header.Number.Bytes()
	blockNumberBytes := common.LeftPadBytes(blockNumber[:], 32)

	return blockNumberBytes, nil
}

type hashHeader struct{}

func (c *hashHeader) RequiredGas(input []byte) uint64 {
	return params.HashHeaderGas
}

func (c *hashHeader) Run(input []byte) ([]byte, error) {
	var header types.Header
	err := rlp.DecodeBytes(input, &header)
	if err != nil {
		return nil, ErrInputDecode
	}

	hashBytes := header.Hash().Bytes()

	return hashBytes, nil
}

type getParentSealBitmap struct{}

func (c *getParentSealBitmap) RequiredGas(input []byte) uint64 {
	return params.GetParentSealBitmapGas
}

// Return the signer bitmap from the parent seal of a past block in the chain.
// Requested parent seal must have occurred within 4 epochs of the current block number.
func (c *getParentSealBitmap) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {
	// input is comprised of a single argument:
	//   blockNumber: 32 byte integer representing the block number to access
	if len(input) < 32 {
		return nil, ErrInputLength
	}

	blockNumber := new(big.Int).SetBytes(input[0:32])

	// Ensure the request is for information from a previously sealed block.
	if blockNumber.Cmp(common.Big0) == 0 || blockNumber.Cmp(ctx.BlockNumber) > 0 {
		return nil, ErrBlockNumberOutOfBounds
	}

	// Ensure the request is for a sufficiently recent block to limit state expansion.
	historyLimit := new(big.Int).SetUint64(ctx.EpochSize * 4)
	if blockNumber.Cmp(new(big.Int).Sub(ctx.BlockNumber, historyLimit)) <= 0 {
		return nil, ErrBlockNumberOutOfBounds
	}

	header := ctx.GetHeaderByNumber(blockNumber.Uint64())
	if header == nil {
		log.Error("Unexpected failure to retrieve block in getParentSealBitmap precompile", "blockNumber", blockNumber)
		return nil, ErrUnexpected
	}

	extra, err := header.IstanbulExtra()
	if err != nil {
		log.Error("Header without Istanbul extra data encountered in getParentSealBitmap precompile", "blockNumber", blockNumber, "err", err)
		return nil, ErrEngineIncompatible
	}

	return common.LeftPadBytes(extra.ParentAggregatedSeal.Bitmap.Bytes()[:], 32), nil
}

// getVerifiedSealBitmap is a precompile to verify the seal on a given header and extract its bitmap.
type getVerifiedSealBitmap struct{}

func (c *getVerifiedSealBitmap) RequiredGas(input []byte) uint64 {
	return params.GetVerifiedSealBitmapGas
}

func (c *getVerifiedSealBitmap) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {
	// input is comprised of a single argument:
	//   header:  rlp encoded block header
	var header types.Header
	if err := rlp.DecodeBytes(input, &header); err != nil {
		return nil, ErrInputDecode
	}

	// Verify the seal against the engine rules.
	if !ctx.VerifySeal(&header) {
		return nil, ErrInputVerification
	}

	// Extract the verified seal from the header.
	extra, err := header.IstanbulExtra()
	if err != nil {
		log.Error("Header without Istanbul extra data encountered in getVerifiedSealBitmap precompile", "extraData", header.Extra, "err", err)
		// Seal verified by a non-Istanbul engine. Return an error.
		return nil, ErrEngineIncompatible
	}

	return common.LeftPadBytes(extra.AggregatedSeal.Bitmap.Bytes()[:], 32), nil
}
