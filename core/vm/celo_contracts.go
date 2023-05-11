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
	"github.com/celo-org/celo-blockchain/crypto/bls12377"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-bls-go/bls"
	ed25519 "github.com/hdevalence/ed25519consensus"
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

func newContext(caller common.Address, evm *EVM) *celoPrecompileContext {
	return &celoPrecompileContext{
		BlockContext: &evm.Context,
		Rules:        &evm.chainRules,
		caller:       caller,
		evm:          evm,
	}
}

func (ctx *celoPrecompileContext) IsCallerGoldToken() (bool, error) {
	return ctx.IsGoldTokenAddress(ctx.evm, ctx.caller)
}

func celoPrecompileAddress(index byte) common.Address {
	celoPrecompiledContractsAddressOffset := byte(0xff)
	return common.BytesToAddress(append([]byte{0}, (celoPrecompiledContractsAddressOffset - index)))
}

// Native transfer contract to make Celo Gold ERC20 compatible.
type transfer struct{}

func (c *transfer) RequiredGas(input []byte) uint64 {
	return params.CallValueTransferGas
}

func (c *transfer) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {

	if isGoldToken, err := ctx.IsCallerGoldToken(); err != nil {
		return nil, err
	} else if !isGoldToken {
		return nil, fmt.Errorf("Unable to call transfer from unpermissioned address")
	}

	// input is comprised of 3 arguments:
	//   from:  32 bytes representing the address of the sender
	//   to:    32 bytes representing the address of the recipient
	//   value: 32 bytes, a 256 bit integer representing the amount of Celo Gold to transfer
	// 3 arguments x 32 bytes each = 96 bytes total input
	if (ctx.IsGFork && len(input) != 96) || len(input) < 96 {
		return nil, ErrInputLength
	}

	from := common.BytesToAddress(input[0:32])
	to := common.BytesToAddress(input[32:64])

	var parsed bool
	value, parsed := math.ParseBig256(hexutil.Encode(input[64:96]))
	if !parsed {
		return nil, fmt.Errorf("Error parsing transfer: unable to parse value from " + hexutil.Encode(input[64:96]))
	}

	if from == common.ZeroAddress {
		// Mint case: Create cGLD out of thin air
		ctx.evm.StateDB.AddBalance(to, value)
	} else {
		// Fail if we're trying to transfer more than the available balance
		if !ctx.CanTransfer(ctx.evm.StateDB, from, value) {
			return nil, ErrInsufficientBalance
		}

		ctx.Transfer(ctx.evm.StateDB, from, to, value)
	}

	return input, nil
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

// ed25519Verify implements a native Ed25519 signature verification.
type ed25519Verify struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *ed25519Verify) RequiredGas(input []byte) uint64 {
	const sha2_512WordLength = 64

	// round up to next whole word
	lengthCeil := len(input) + sha2_512WordLength - 1
	words := uint64(lengthCeil / sha2_512WordLength)
	return params.Ed25519VerifyGas + params.Sha2_512BaseGas + (words * params.Sha2_512PerWordGas)
}

func (c *ed25519Verify) Run(input []byte, ctx *celoPrecompileContext) ([]byte, error) {
	// Setup success/failure return values
	var fail32byte, success32Byte = true32Byte, false32Byte

	// Check if all required arguments are present
	if len(input) < 96 {
		return fail32byte, nil
	}

	publicKey := input[0:32]  // 32 bytes
	signature := input[32:96] // 64 bytes
	message := input[96:]     // arbitrary length

	// Verify the Ed25519 signature against the public key and message
	// https://godoc.org/golang.org/x/crypto/ed25519#Verify
	if ed25519.Verify(publicKey, message, signature) {
		return success32Byte, nil
	}
	return fail32byte, nil
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

// cip20HashFunctions is a precompile to compute any of several
// cryprographic hash functions
type cip20HashFunctions struct {
	hashes map[uint8]Cip20Hash
}

func (c *cip20HashFunctions) RequiredGas(input []byte) uint64 {
	if len(input) == 0 {
		return params.InvalidCip20Gas
	}

	if h, ok := c.hashes[input[0]]; ok {
		return h.RequiredGas(input[1:])
	}

	return params.InvalidCip20Gas
}

func (c *cip20HashFunctions) Run(input []byte) ([]byte, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("Input Error: 0-byte input")
	}

	if h, ok := c.hashes[input[0]]; ok {
		output, err := h.Run(input[1:]) // trim selector

		if err != nil {
			return nil, err
		}
		return output, nil
	}

	return nil, fmt.Errorf("Input Error: invalid CIP20 selector: %d", input[0])
}

// bls12377G1Add implements EIP-2539 G1Add precompile.
type bls12377G1Add struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377G1Add) RequiredGas(input []byte) uint64 {
	return params.Bls12377G1AddGas
}

func (c *bls12377G1Add) Run(input []byte) ([]byte, error) {
	// Implements EIP-2539 G1Add precompile.
	// > G1 addition call expects `256` bytes as an input that is interpreted as byte concatenation of two G1 points (`128` bytes each).
	// > Output is an encoding of addition operation result - single G1 point (`128` bytes).
	if len(input) != 256 {
		return nil, errBLS12377InvalidInputLength
	}
	var p0, p1 *bls12377.PointG1
	var err error

	// Initialize G1
	g := bls12377.NewG1()

	// Decode G1 point p_0
	if p0, err = g.DecodePoint(input[:128]); err != nil {
		return nil, err
	}
	// Decode G1 point p_1
	if p1, err = g.DecodePoint(input[128:]); err != nil {
		return nil, err
	}

	// Compute r = p_0 + p_1
	r := g.New()
	g.Add(r, p0, p1)

	// Encode the G1 point result into 128 bytes
	return g.EncodePoint(r), nil
}

// bls12377G1Mul implements EIP-2539 G1Mul precompile.
type bls12377G1Mul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377G1Mul) RequiredGas(input []byte) uint64 {
	return params.Bls12377G1MulGas
}

func (c *bls12377G1Mul) Run(input []byte) ([]byte, error) {
	// Implements EIP-2539 G1Mul precompile.
	// > G1 multiplication call expects `160` bytes as an input that is interpreted as byte concatenation of encoding of G1 point (`128` bytes) and encoding of a scalar value (`32` bytes).
	// > Output is an encoding of multiplication operation result - single G1 point (`128` bytes).
	if len(input) != 160 {
		return nil, errBLS12377InvalidInputLength
	}
	var p0 *bls12377.PointG1
	var err error

	// Initialize G1
	g := bls12377.NewG1()

	// Decode G1 point
	if p0, err = g.DecodePoint(input[:128]); err != nil {
		return nil, err
	}
	// Decode scalar value
	e := new(big.Int).SetBytes(input[128:])

	// Compute r = e * p_0
	r := g.New()
	g.MulScalar(r, p0, e)

	// Encode the G1 point into 128 bytes
	return g.EncodePoint(r), nil
}

// bls12377G1MultiExp implements EIP-2539 G1MultiExp precompile.
type bls12377G1MultiExp struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377G1MultiExp) RequiredGas(input []byte) uint64 {
	// Calculate G1 point, scalar value pair length
	k := len(input) / 160
	if k == 0 {
		// Return 0 gas for small input length
		return 0
	}
	// Lookup discount value for G1 point, scalar value pair length
	var discount uint64
	if dLen := len(params.Bls12377MultiExpDiscountTable); k < dLen {
		discount = params.Bls12377MultiExpDiscountTable[k-1]
	} else {
		discount = params.Bls12377MultiExpDiscountTable[dLen-1]
	}
	// Calculate gas and return the result
	return (uint64(k) * params.Bls12377G1MulGas * discount) / 1000
}

func (c *bls12377G1MultiExp) Run(input []byte) ([]byte, error) {
	// Implements EIP-2539 G1MultiExp precompile.
	// G1 multiplication call expects `160*k` bytes as an input that is interpreted as byte concatenation of `k` slices each of them being a byte concatenation of encoding of G1 point (`128` bytes) and encoding of a scalar value (`32` bytes).
	// Output is an encoding of multiexponentiation operation result - single G1 point (`128` bytes).
	k := len(input) / 160
	if len(input) == 0 || len(input)%160 != 0 {
		return nil, errBLS12377InvalidInputLength
	}
	points := make([]*bls12377.PointG1, k)
	scalars := make([]*big.Int, k)

	// Initialize G1
	g := bls12377.NewG1()

	// Decode point scalar pairs
	for i := 0; i < k; i++ {
		off := 160 * i
		t0, t1, t2 := off, off+128, off+160
		// Decode G1 point
		var err error
		if points[i], err = g.DecodePoint(input[t0:t1]); err != nil {
			return nil, err
		}
		// Decode scalar value
		scalars[i] = new(big.Int).SetBytes(input[t1:t2])
	}

	// Compute r = e_0 * p_0 + e_1 * p_1 + ... + e_(k-1) * p_(k-1)
	r := g.New()
	g.MultiExp(r, points, scalars)

	// Encode the G1 point to 128 bytes
	return g.EncodePoint(r), nil
}

// bls12377G2Add implements EIP-2539 G2Add precompile.
type bls12377G2Add struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377G2Add) RequiredGas(input []byte) uint64 {
	return params.Bls12377G2AddGas
}

func (c *bls12377G2Add) Run(input []byte) ([]byte, error) {
	// Implements EIP-2539 G2Add precompile.
	// > G2 addition call expects `512` bytes as an input that is interpreted as byte concatenation of two G2 points (`256` bytes each).
	// > Output is an encoding of addition operation result - single G2 point (`256` bytes).
	if len(input) != 512 {
		return nil, errBLS12377InvalidInputLength
	}
	var p0, p1 *bls12377.PointG2
	var err error

	// Initialize G2
	g := bls12377.NewG2()
	r := g.New()

	// Decode G2 point p_0
	if p0, err = g.DecodePoint(input[:256]); err != nil {
		return nil, err
	}
	// Decode G2 point p_1
	if p1, err = g.DecodePoint(input[256:]); err != nil {
		return nil, err
	}

	// Compute r = p_0 + p_1
	g.Add(r, p0, p1)

	// Encode the G2 point into 256 bytes
	return g.EncodePoint(r), nil
}

// bls12377G2Mul implements EIP-2539 G2Mul precompile.
type bls12377G2Mul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377G2Mul) RequiredGas(input []byte) uint64 {
	return params.Bls12377G2MulGas
}

func (c *bls12377G2Mul) Run(input []byte) ([]byte, error) {
	// Implements EIP-2539 G2MUL precompile logic.
	// > G2 multiplication call expects `288` bytes as an input that is interpreted as byte concatenation of encoding of G2 point (`256` bytes) and encoding of a scalar value (`32` bytes).
	// > Output is an encoding of multiplication operation result - single G2 point (`256` bytes).
	if len(input) != 288 {
		return nil, errBLS12377InvalidInputLength
	}
	var p0 *bls12377.PointG2
	var err error

	// Initialize G2
	g := bls12377.NewG2()

	// Decode G2 point
	if p0, err = g.DecodePoint(input[:256]); err != nil {
		return nil, err
	}
	// Decode scalar value
	e := new(big.Int).SetBytes(input[256:])

	// Compute r = e * p_0
	r := g.New()
	g.MulScalar(r, p0, e)

	// Encode the G2 point into 256 bytes
	return g.EncodePoint(r), nil
}

// bls12377G2MultiExp implements EIP-2539 G2MultiExp precompile.
type bls12377G2MultiExp struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377G2MultiExp) RequiredGas(input []byte) uint64 {
	// Calculate G2 point, scalar value pair length
	k := len(input) / 288
	if k == 0 {
		// Return 0 gas for small input length
		return 0
	}
	// Lookup discount value for G2 point, scalar value pair length
	var discount uint64
	if dLen := len(params.Bls12377MultiExpDiscountTable); k < dLen {
		discount = params.Bls12377MultiExpDiscountTable[k-1]
	} else {
		discount = params.Bls12377MultiExpDiscountTable[dLen-1]
	}
	// Calculate gas and return the result
	return (uint64(k) * params.Bls12377G2MulGas * discount) / 1000
}

func (c *bls12377G2MultiExp) Run(input []byte) ([]byte, error) {
	// Implements EIP-2539 G2MultiExp precompile logic
	// > G2 multiplication call expects `288*k` bytes as an input that is interpreted as byte concatenation of `k` slices each of them being a byte concatenation of encoding of G2 point (`256` bytes) and encoding of a scalar value (`32` bytes).
	// > Output is an encoding of multiexponentiation operation result - single G2 point (`256` bytes).
	k := len(input) / 288
	if len(input) == 0 || len(input)%288 != 0 {
		return nil, errBLS12377InvalidInputLength
	}
	points := make([]*bls12377.PointG2, k)
	scalars := make([]*big.Int, k)

	// Initialize G2
	g := bls12377.NewG2()

	// Decode point scalar pairs
	for i := 0; i < k; i++ {
		off := 288 * i
		t0, t1, t2 := off, off+256, off+288
		// Decode G1 point
		var err error
		if points[i], err = g.DecodePoint(input[t0:t1]); err != nil {
			return nil, err
		}
		// Decode scalar value
		scalars[i] = new(big.Int).SetBytes(input[t1:t2])
	}

	// Compute r = e_0 * p_0 + e_1 * p_1 + ... + e_(k-1) * p_(k-1)
	r := g.New()
	g.MultiExp(r, points, scalars)

	// Encode the G2 point to 256 bytes.
	return g.EncodePoint(r), nil
}

// bls12377Pairing implements EIP-2539 Pairing precompile.
type bls12377Pairing struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377Pairing) RequiredGas(input []byte) uint64 {
	return params.Bls12377PairingBaseGas + uint64(len(input)/384)*params.Bls12377PairingPerPairGas
}

func (c *bls12377Pairing) Run(input []byte) ([]byte, error) {
	// Implements EIP-2539 Pairing precompile logic.
	// > Pairing call expects `384*k` bytes as an inputs that is interpreted as byte concatenation of `k` slices. Each slice has the following structure:
	// > - `128` bytes of G1 point encoding
	// > - `256` bytes of G2 point encoding
	// > Output is a `32` bytes where last single byte is `0x01` if pairing result is equal to multiplicative identity in a pairing target field and `0x00` otherwise
	// > (which is equivalent of Big Endian encoding of Solidity values `uint256(1)` and `uin256(0)` respectively).
	k := len(input) / 384
	if len(input) == 0 || len(input)%384 != 0 {
		return nil, errBLS12377InvalidInputLength
	}

	// Initialize BLS12-377 pairing engine
	e := bls12377.NewPairingEngine()
	g1, g2 := e.G1, e.G2

	// Decode pairs
	for i := 0; i < k; i++ {
		off := 384 * i
		t0, t1, t2 := off, off+128, off+384

		// Decode G1 point
		p1, err := g1.DecodePoint(input[t0:t1])
		if err != nil {
			return nil, err
		}
		// Decode G2 point
		p2, err := g2.DecodePoint(input[t1:t2])
		if err != nil {
			return nil, err
		}

		// 'point is on curve' check already done,
		// Here we need to apply subgroup checks.
		if !g1.InCorrectSubgroup(p1) {
			return nil, errBLS12377G1PointSubgroup
		}
		if !g2.InCorrectSubgroup(p2) {
			return nil, errBLS12377G2PointSubgroup
		}

		// Update pairing engine with G1 and G2 ponits
		e.AddPair(p1, p2)
	}
	// Prepare 32 byte output
	out := make([]byte, 32)

	// Compute pairing and set the result
	if e.Check() {
		out[31] = 1
	}
	return out, nil
}
