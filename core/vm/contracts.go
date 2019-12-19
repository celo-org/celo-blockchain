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

package vm

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"

	"github.com/celo-org/bls-zexe/go"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/crypto/bn256"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/ripemd160"
)

// PrecompiledContract is the basic interface for native Go contracts. The implementation
// requires a deterministic gas count based on the input size of the Run method of the
// contract.
type PrecompiledContract interface {
	RequiredGas(input []byte) uint64                                                       // RequiredGas calculates the contract gas use
	Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) // Run runs the precompiled contract
}

// PrecompiledContractsHomestead contains the default set of pre-compiled Ethereum
// contracts used in the Frontier and Homestead releases.
var PrecompiledContractsHomestead = map[common.Address]PrecompiledContract{
	common.BytesToAddress([]byte{1}): &ecrecover{},
	common.BytesToAddress([]byte{2}): &sha256hash{},
	common.BytesToAddress([]byte{3}): &ripemd160hash{},
	common.BytesToAddress([]byte{4}): &dataCopy{},
}

func celoPrecompileAddress(index byte) common.Address {
	return common.BytesToAddress(append([]byte{0}, (CeloPrecompiledContractsAddressOffset - index)))
}

var (
	CeloPrecompiledContractsAddressOffset = byte(0xff)

	transferAddress              = celoPrecompileAddress(2)
	fractionMulExpAddress        = celoPrecompileAddress(3)
	proofOfPossessionAddress     = celoPrecompileAddress(4)
	getValidatorAddress          = celoPrecompileAddress(5)
	numberValidatorsAddress      = celoPrecompileAddress(6)
	epochSizeAddress             = celoPrecompileAddress(7)
	blockNumberFromHeaderAddress = celoPrecompileAddress(8)
	hashHeaderAddress            = celoPrecompileAddress(9)
	getParentSealBitmapAddress   = celoPrecompileAddress(10)
	getVerifiedSealBitmapAddress = celoPrecompileAddress(11)
)

// PrecompiledContractsByzantium contains the default set of pre-compiled Ethereum
// contracts used in the Byzantium release.
var PrecompiledContractsByzantium = map[common.Address]PrecompiledContract{
	common.BytesToAddress([]byte{1}): &ecrecover{},
	common.BytesToAddress([]byte{2}): &sha256hash{},
	common.BytesToAddress([]byte{3}): &ripemd160hash{},
	common.BytesToAddress([]byte{4}): &dataCopy{},
	common.BytesToAddress([]byte{5}): &bigModExp{},
	common.BytesToAddress([]byte{6}): &bn256Add{},
	common.BytesToAddress([]byte{7}): &bn256ScalarMul{},
	common.BytesToAddress([]byte{8}): &bn256Pairing{},

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
}

// RunPrecompiledContract runs and evaluates the output of a precompiled contract.
func RunPrecompiledContract(p PrecompiledContract, input []byte, contract *Contract, evm *EVM) (ret []byte, err error) {
	log.Trace("Running precompiled contract", "codeaddr", contract.CodeAddr, "input", input, "caller", contract.CallerAddress, "gas", contract.Gas)
	ret, gas, err := p.Run(input, contract.CallerAddress, evm, contract.Gas)
	contract.UseGas(contract.Gas - gas)
	log.Trace("Finished running precompiled contract", "codeaddr", contract.CodeAddr, "input", input, "caller", contract.CallerAddress, "gas", contract.Gas, "gas_left", gas)
	return ret, err
}

func debitRequiredGas(p PrecompiledContract, input []byte, gas uint64) (uint64, error) {
	requiredGas := p.RequiredGas(input)
	if requiredGas > gas {
		return gas, ErrOutOfGas
	}
	return gas - requiredGas, nil
}

// ECRECOVER implemented as a native contract.
type ecrecover struct{}

func (c *ecrecover) RequiredGas(input []byte) uint64 {
	return params.EcrecoverGas
}

func (c *ecrecover) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	const ecRecoverInputLength = 128

	input = common.RightPadBytes(input, ecRecoverInputLength)
	// "input" is (hash, v, r, s), each 32 bytes
	// but for ecrecover we want (r, s, v)

	r := new(big.Int).SetBytes(input[64:96])
	s := new(big.Int).SetBytes(input[96:128])
	v := input[63] - 27

	// tighter sig s values input homestead only apply to tx sigs
	if !allZero(input[32:63]) || !crypto.ValidateSignatureValues(v, r, s, false) {
		return nil, gas, nil
	}
	// v needs to be at the end for libsecp256k1
	pubKey, err := crypto.Ecrecover(input[:32], append(input[64:128], v))
	// make sure the public key is a valid one
	if err != nil {
		return nil, gas, nil
	}

	// the first byte of pubkey is bitcoin heritage
	return common.LeftPadBytes(crypto.Keccak256(pubKey[1:])[12:], 32), gas, nil
}

// SHA256 implemented as a native contract.
type sha256hash struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
//
// This method does not require any overflow checking as the input size gas costs
// required for anything significant is so high it's impossible to pay for.
func (c *sha256hash) RequiredGas(input []byte) uint64 {
	return uint64(len(input)+31)/32*params.Sha256PerWordGas + params.Sha256BaseGas
}
func (c *sha256hash) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	h := sha256.Sum256(input)
	return h[:], gas, nil
}

// RIPEMD160 implemented as a native contract.
type ripemd160hash struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
//
// This method does not require any overflow checking as the input size gas costs
// required for anything significant is so high it's impossible to pay for.
func (c *ripemd160hash) RequiredGas(input []byte) uint64 {
	return uint64(len(input)+31)/32*params.Ripemd160PerWordGas + params.Ripemd160BaseGas
}
func (c *ripemd160hash) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	ripemd := ripemd160.New()
	ripemd.Write(input)
	return common.LeftPadBytes(ripemd.Sum(nil), 32), gas, nil
}

// data copy implemented as a native contract.
type dataCopy struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
//
// This method does not require any overflow checking as the input size gas costs
// required for anything significant is so high it's impossible to pay for.
func (c *dataCopy) RequiredGas(input []byte) uint64 {
	return uint64(len(input)+31)/32*params.IdentityPerWordGas + params.IdentityBaseGas
}
func (c *dataCopy) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	return input, gas, nil
}

// bigModExp implements a native big integer exponential modular operation.
type bigModExp struct{}

var (
	big1      = big.NewInt(1)
	big4      = big.NewInt(4)
	big8      = big.NewInt(8)
	big16     = big.NewInt(16)
	big32     = big.NewInt(32)
	big64     = big.NewInt(64)
	big96     = big.NewInt(96)
	big480    = big.NewInt(480)
	big1024   = big.NewInt(1024)
	big3072   = big.NewInt(3072)
	big199680 = big.NewInt(199680)
)

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bigModExp) RequiredGas(input []byte) uint64 {
	var (
		baseLen = new(big.Int).SetBytes(getData(input, 0, 32))
		expLen  = new(big.Int).SetBytes(getData(input, 32, 32))
		modLen  = new(big.Int).SetBytes(getData(input, 64, 32))
	)
	if len(input) > 96 {
		input = input[96:]
	} else {
		input = input[:0]
	}
	// Retrieve the head 32 bytes of exp for the adjusted exponent length
	var expHead *big.Int
	if big.NewInt(int64(len(input))).Cmp(baseLen) <= 0 {
		expHead = new(big.Int)
	} else {
		if expLen.Cmp(big32) > 0 {
			expHead = new(big.Int).SetBytes(getData(input, baseLen.Uint64(), 32))
		} else {
			expHead = new(big.Int).SetBytes(getData(input, baseLen.Uint64(), expLen.Uint64()))
		}
	}
	// Calculate the adjusted exponent length
	var msb int
	if bitlen := expHead.BitLen(); bitlen > 0 {
		msb = bitlen - 1
	}
	adjExpLen := new(big.Int)
	if expLen.Cmp(big32) > 0 {
		adjExpLen.Sub(expLen, big32)
		adjExpLen.Mul(big8, adjExpLen)
	}
	adjExpLen.Add(adjExpLen, big.NewInt(int64(msb)))

	// Calculate the gas cost of the operation
	gas := new(big.Int).Set(math.BigMax(modLen, baseLen))
	switch {
	case gas.Cmp(big64) <= 0:
		gas.Mul(gas, gas)
	case gas.Cmp(big1024) <= 0:
		gas = new(big.Int).Add(
			new(big.Int).Div(new(big.Int).Mul(gas, gas), big4),
			new(big.Int).Sub(new(big.Int).Mul(big96, gas), big3072),
		)
	default:
		gas = new(big.Int).Add(
			new(big.Int).Div(new(big.Int).Mul(gas, gas), big16),
			new(big.Int).Sub(new(big.Int).Mul(big480, gas), big199680),
		)
	}
	gas.Mul(gas, math.BigMax(adjExpLen, big1))
	gas.Div(gas, new(big.Int).SetUint64(params.ModExpQuadCoeffDiv))

	if gas.BitLen() > 64 {
		return math.MaxUint64
	}
	return gas.Uint64()
}

func (c *bigModExp) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	var (
		baseLen = new(big.Int).SetBytes(getData(input, 0, 32)).Uint64()
		expLen  = new(big.Int).SetBytes(getData(input, 32, 32)).Uint64()
		modLen  = new(big.Int).SetBytes(getData(input, 64, 32)).Uint64()
	)
	if len(input) > 96 {
		input = input[96:]
	} else {
		input = input[:0]
	}
	// Handle a special case when both the base and mod length is zero
	if baseLen == 0 && modLen == 0 {
		return []byte{}, gas, nil
	}
	// Retrieve the operands and execute the exponentiation
	var (
		base = new(big.Int).SetBytes(getData(input, 0, baseLen))
		exp  = new(big.Int).SetBytes(getData(input, baseLen, expLen))
		mod  = new(big.Int).SetBytes(getData(input, baseLen+expLen, modLen))
	)
	if mod.BitLen() == 0 {
		// Modulo 0 is undefined, return zero
		return common.LeftPadBytes([]byte{}, int(modLen)), gas, nil
	}
	return common.LeftPadBytes(base.Exp(base, exp, mod).Bytes(), int(modLen)), gas, nil
}

// newCurvePoint unmarshals a binary blob into a bn256 elliptic curve point,
// returning it, or an error if the point is invalid.
func newCurvePoint(blob []byte) (*bn256.G1, error) {
	p := new(bn256.G1)
	if _, err := p.Unmarshal(blob); err != nil {
		return nil, err
	}
	return p, nil
}

// newTwistPoint unmarshals a binary blob into a bn256 elliptic curve point,
// returning it, or an error if the point is invalid.
func newTwistPoint(blob []byte) (*bn256.G2, error) {
	p := new(bn256.G2)
	if _, err := p.Unmarshal(blob); err != nil {
		return nil, err
	}
	return p, nil
}

// bn256Add implements a native elliptic curve point addition.
type bn256Add struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256Add) RequiredGas(input []byte) uint64 {
	return params.Bn256AddGas
}

func (c *bn256Add) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	x, err := newCurvePoint(getData(input, 0, 64))
	if err != nil {
		return nil, gas, err
	}
	y, err := newCurvePoint(getData(input, 64, 64))
	if err != nil {
		return nil, gas, err
	}
	res := new(bn256.G1)
	res.Add(x, y)
	return res.Marshal(), gas, nil
}

// bn256ScalarMul implements a native elliptic curve scalar multiplication.
type bn256ScalarMul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256ScalarMul) RequiredGas(input []byte) uint64 {
	return params.Bn256ScalarMulGas
}

func (c *bn256ScalarMul) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	p, err := newCurvePoint(getData(input, 0, 64))
	if err != nil {
		return nil, gas, err
	}
	res := new(bn256.G1)
	res.ScalarMult(p, new(big.Int).SetBytes(getData(input, 64, 32)))
	return res.Marshal(), gas, nil
}

var (
	// true32Byte is returned if the bn256 pairing check succeeds.
	true32Byte = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

	// false32Byte is returned if the bn256 pairing check fails.
	false32Byte = make([]byte, 32)

	// errBadPairingInput is returned if the bn256 pairing input is invalid.
	errBadPairingInput = errors.New("bad elliptic curve pairing size")
)

// bn256Pairing implements a pairing pre-compile for the bn256 curve
type bn256Pairing struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256Pairing) RequiredGas(input []byte) uint64 {
	return params.Bn256PairingBaseGas + uint64(len(input)/192)*params.Bn256PairingPerPointGas
}

func (c *bn256Pairing) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Handle some corner cases cheaply
	if len(input)%192 > 0 {
		return nil, gas, errBadPairingInput
	}
	// Convert the input into a set of coordinates
	var (
		cs []*bn256.G1
		ts []*bn256.G2
	)
	for i := 0; i < len(input); i += 192 {
		c, err := newCurvePoint(input[i : i+64])
		if err != nil {
			return nil, gas, err
		}
		t, err := newTwistPoint(input[i+64 : i+192])
		if err != nil {
			return nil, gas, err
		}
		cs = append(cs, c)
		ts = append(ts, t)
	}
	// Execute the pairing checks and return the results
	if bn256.PairingCheck(cs, ts) {
		return true32Byte, gas, nil
	}
	return false32Byte, gas, nil
}

// Native transfer contract to make Celo Gold ERC20 compatible.
type transfer struct{}

func (c *transfer) RequiredGas(input []byte) uint64 {
	return params.TxGas
}

func (c *transfer) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	celoGoldAddress, err := GetRegisteredAddressWithEvm(params.GoldTokenRegistryId, evm)
	if err != nil {
		return nil, gas, err
	}

	// input is comprised of 3 arguments:
	//   from:  32 bytes representing the address of the sender
	//   to:    32 bytes representing the address of the recipient
	//   value: 32 bytes, a 256 bit integer representing the amount of Celo Gold to transfer
	// 3 arguments x 32 bytes each = 96 bytes total input
	if len(input) < 96 {
		return nil, gas, ErrInputLength
	}

	if caller != *celoGoldAddress {
		return nil, gas, fmt.Errorf("Unable to call transfer from unpermissioned address")
	}
	from := common.BytesToAddress(input[0:32])
	to := common.BytesToAddress(input[32:64])
	var parsed bool
	value, parsed := math.ParseBig256(hexutil.Encode(input[64:96]))
	if !parsed {
		return nil, gas, fmt.Errorf("Error parsing transfer: unable to parse value from " + hexutil.Encode(input[64:96]))
	}
	// Fail if we're trying to transfer more than the available balance
	if !evm.Context.CanTransfer(evm.StateDB, from, value) {
		return nil, gas, ErrInsufficientBalance
	}

	gas, err = evm.TobinTransfer(evm.StateDB, from, to, gas, value)

	return input, gas, err
}

// computes a * (b ^ exponent) to `decimals` places of precision, where a and b are fractions
type fractionMulExp struct{}

func (c *fractionMulExp) RequiredGas(input []byte) uint64 {
	return params.FractionMulExpGas
}

func (c *fractionMulExp) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// input is comprised of 6 arguments:
	//   aNumerator:   32 bytes, 256 bit integer, numerator for the first fraction (a)
	//   aDenominator: 32 bytes, 256 bit integer, denominator for the first fraction (a)
	//   bNumerator:   32 bytes, 256 bit integer, numerator for the second fraction (b)
	//   bDenominator: 32 bytes, 256 bit integer, denominator for the second fraction (b)
	//   exponent:     32 bytes, 256 bit integer, exponent to raise the second fraction (b) to
	//   decimals:     32 bytes, 256 bit integer, places of precision
	//
	// 6 args x 32 bytes each = 192 bytes total input length
	if len(input) < 192 {
		return nil, gas, ErrInputLength
	}

	parseErrorStr := "Error parsing input: unable to parse %s value from %s"

	aNumerator, parsed := math.ParseBig256(hexutil.Encode(input[0:32]))
	if !parsed {
		return nil, gas, fmt.Errorf(parseErrorStr, "aNumerator", hexutil.Encode(input[0:32]))
	}

	aDenominator, parsed := math.ParseBig256(hexutil.Encode(input[32:64]))
	if !parsed {
		return nil, gas, fmt.Errorf(parseErrorStr, "aDenominator", hexutil.Encode(input[32:64]))
	}

	bNumerator, parsed := math.ParseBig256(hexutil.Encode(input[64:96]))
	if !parsed {
		return nil, gas, fmt.Errorf(parseErrorStr, "bNumerator", hexutil.Encode(input[64:96]))
	}

	bDenominator, parsed := math.ParseBig256(hexutil.Encode(input[96:128]))
	if !parsed {
		return nil, gas, fmt.Errorf(parseErrorStr, "bDenominator", hexutil.Encode(input[96:128]))
	}

	exponent, parsed := math.ParseBig256(hexutil.Encode(input[128:160]))
	if !parsed {
		return nil, gas, fmt.Errorf(parseErrorStr, "exponent", hexutil.Encode(input[128:160]))
	}

	decimals, parsed := math.ParseBig256(hexutil.Encode(input[160:192]))
	if !parsed {
		return nil, gas, fmt.Errorf(parseErrorStr, "decimals", hexutil.Encode(input[160:192]))
	}

	// Handle passing of zero denominators
	if aDenominator == big.NewInt(0) || bDenominator == big.NewInt(0) {
		return nil, gas, fmt.Errorf("Input Error: Denominator of zero provided!")
	}

	numeratorExp := new(big.Int).Mul(aNumerator, new(big.Int).Exp(bNumerator, exponent, nil))
	denominatorExp := new(big.Int).Mul(aDenominator, new(big.Int).Exp(bDenominator, exponent, nil))

	decimalAdjustment := new(big.Int).Exp(big.NewInt(10), decimals, nil)

	numeratorDecimalAdjusted := new(big.Int).Div(new(big.Int).Mul(numeratorExp, decimalAdjustment), denominatorExp).Bytes()
	denominatorDecimalAdjusted := decimalAdjustment.Bytes()

	numeratorPadded := common.LeftPadBytes(numeratorDecimalAdjusted, 32)
	denominatorPadded := common.LeftPadBytes(denominatorDecimalAdjusted, 32)

	return append(numeratorPadded, denominatorPadded...), gas, nil
}

type proofOfPossession struct{}

func (c *proofOfPossession) RequiredGas(input []byte) uint64 {
	return params.ProofOfPossessionGas
}

func (c *proofOfPossession) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// input is comprised of 3 arguments:
	//   address:   20 bytes, an address used to generate the proof-of-possession
	//   publicKey: 48 bytes, representing the public key (defined as a const in bls package)
	//   signature: 96 bytes, representing the signature on `address` (defined as a const in bls package)
	// the total length of input required is the sum of these constants
	if len(input) != common.AddressLength+blscrypto.PUBLICKEYBYTES+blscrypto.SIGNATUREBYTES {
		return nil, gas, ErrInputLength
	}
	addressBytes := input[:common.AddressLength]

	publicKeyBytes := input[common.AddressLength : common.AddressLength+blscrypto.PUBLICKEYBYTES]
	publicKey, err := bls.DeserializePublicKey(publicKeyBytes)
	if err != nil {
		return nil, gas, err
	}
	defer publicKey.Destroy()

	signatureBytes := input[common.AddressLength+blscrypto.PUBLICKEYBYTES : common.AddressLength+blscrypto.PUBLICKEYBYTES+blscrypto.SIGNATUREBYTES]
	signature, err := bls.DeserializeSignature(signatureBytes)
	if err != nil {
		return nil, gas, err
	}
	defer signature.Destroy()

	err = publicKey.VerifyPoP(addressBytes, signature)
	if err != nil {
		return nil, gas, err
	}

	return true32Byte, gas, nil
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
func (c *getValidator) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// input is comprised of two arguments:
	//   index: 32 byte integer representing the index of the validator to get
	//   blockNumber: 32 byte integer representing the block number to access
	if len(input) < 64 {
		return nil, gas, ErrInputLength
	}

	index := new(big.Int).SetBytes(input[0:32])

	blockNumber := new(big.Int).SetBytes(input[32:64])
	if blockNumber.Cmp(common.Big0) == 0 {
		// Validator set for the genesis block is empty, so any index is out of bounds.
		return nil, gas, ErrValidatorsOutOfBounds
	}
	if blockNumber.Cmp(evm.Context.BlockNumber) > 0 {
		return nil, gas, ErrBlockNumberOutOfBounds
	}

	// Note: Passing empty hash as here as this context does not have access to arbitrary block hashes, and it is not needed.
	validators := evm.Context.Engine.GetValidators(new(big.Int).Sub(blockNumber, common.Big1), common.Hash{})

	// Ensure index, which is guaranteed to be non-negative, is valid.
	if index.Cmp(big.NewInt(int64(len(validators)))) >= 0 {
		return nil, gas, ErrValidatorsOutOfBounds
	}

	validatorAddress := validators[index.Uint64()].Address()
	addressBytes := common.LeftPadBytes(validatorAddress[:], 32)

	return addressBytes, gas, nil
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
func (c *numberValidators) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// input is comprised of a single argument:
	//   blockNumber: 32 byte integer representing the block number to access
	if len(input) < 32 {
		return nil, gas, ErrInputLength
	}

	blockNumber := new(big.Int).SetBytes(input[0:32])
	if blockNumber.Cmp(common.Big0) == 0 {
		// Genesis validator set is empty. Return 0.
		return make([]byte, 32), gas, nil
	}
	if blockNumber.Cmp(evm.Context.BlockNumber) > 0 {
		return nil, gas, ErrBlockNumberOutOfBounds
	}

	// Note: Passing empty hash as here as this context does not have access to arbitrary block hashes, and it is not needed.
	validators := evm.Context.Engine.GetValidators(new(big.Int).Sub(blockNumber, common.Big1), common.Hash{})

	numberValidators := big.NewInt(int64(len(validators))).Bytes()
	numberValidatorsBytes := common.LeftPadBytes(numberValidators[:], 32)
	return numberValidatorsBytes, gas, nil
}

type epochSize struct{}

func (c *epochSize) RequiredGas(input []byte) uint64 {
	return params.GetEpochSizeGas
}

func (c *epochSize) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil || len(input) != 0 {
		return nil, gas, err
	}
	epochSize := new(big.Int).SetUint64(evm.Context.Engine.EpochSize()).Bytes()
	epochSizeBytes := common.LeftPadBytes(epochSize[:], 32)

	return epochSizeBytes, gas, nil
}

type blockNumberFromHeader struct{}

func (c *blockNumberFromHeader) RequiredGas(input []byte) uint64 {
	return params.GetBlockNumberFromHeaderGas
}

func (c *blockNumberFromHeader) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	var header types.Header
	err = rlp.DecodeBytes(input, &header)
	if err != nil {
		return nil, gas, ErrInputDecode
	}

	blockNumber := header.Number.Bytes()
	blockNumberBytes := common.LeftPadBytes(blockNumber[:], 32)

	return blockNumberBytes, gas, nil
}

type hashHeader struct{}

func (c *hashHeader) RequiredGas(input []byte) uint64 {
	return params.HashHeaderGas
}

func (c *hashHeader) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	var header types.Header
	err = rlp.DecodeBytes(input, &header)
	if err != nil {
		return nil, gas, ErrInputDecode
	}

	hashBytes := header.Hash().Bytes()

	return hashBytes, gas, nil
}

type getParentSealBitmap struct{}

func (c *getParentSealBitmap) RequiredGas(input []byte) uint64 {
	return params.GetParentSealBitmapGas
}

// Return the signer bitmap from the parent seal of a past block in the chain.
// Requested parent seal must have occurred within 4 epochs of the current block number.
func (c *getParentSealBitmap) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// input is comprised of a single argument:
	//   blockNumber: 32 byte integer representing the block number to access
	if len(input) < 32 {
		return nil, gas, ErrInputLength
	}

	blockNumber := new(big.Int).SetBytes(input[0:32])

	// Ensure the request is for information from a previously sealed block.
	if blockNumber.Cmp(common.Big0) == 0 {
		return nil, gas, ErrBlockNumberOutOfBounds
	}
	if blockNumber.Cmp(evm.Context.BlockNumber) >= 0 {
		return nil, gas, ErrBlockNumberOutOfBounds
	}

	// Ensure the request is for a sufficiently recent block to limit state expansion.
	historyLimit := new(big.Int).SetUint64(evm.Context.Engine.EpochSize() * 4)
	if blockNumber.Cmp(new(big.Int).Sub(evm.Context.BlockNumber, historyLimit)) <= 0 {
		return nil, gas, ErrBlockNumberOutOfBounds
	}

	header := evm.Context.GetHeaderByNumber(blockNumber.Uint64())
	if header == nil {
		// Under sane circumstances, this error should never be reached.
		return nil, gas, ErrUnexpected
	}

	extra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		// Header is from a non-Istanbul engine.
		return nil, gas, ErrEngineIncompatible
	}

	return common.LeftPadBytes(extra.ParentAggregatedSeal.Bitmap.Bytes()[:], 32), gas, nil
}

// getVerifiedSealBitmap is a precompile to verify the seal on a given header and extract its bitmap.
type getVerifiedSealBitmap struct{}

func (c *getVerifiedSealBitmap) RequiredGas(input []byte) uint64 {
	return params.GetVerifiedSealBitmapGas
}

func (c *getVerifiedSealBitmap) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// input is comprised of a single argument:
	//   header:  rlp encoded block header
	if len(input) == 0 {
		return nil, gas, ErrInputLength
	}

	// Decode and verify the seal against the engine rules.
	header := new(types.Header)
	if err := rlp.DecodeBytes(input, header); err != nil {
		return nil, gas, ErrInputDecode
	}
	if !evm.Context.VerifySeal(header) {
		return nil, gas, ErrInputValidation
	}

	// Extract the verified seal from the header.
	extra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		// Seal verified by a non-Istanbul engine. Return an error.
		return nil, gas, ErrEngineIncompatible
	}

	return common.LeftPadBytes(extra.AggregatedSeal.Bitmap.Bytes()[:], 32), gas, nil
}
