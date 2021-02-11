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
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	//nolint:goimports
	"github.com/celo-org/celo-bls-go/bls"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/blake2b"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/crypto/bls12377"
	"github.com/ethereum/go-ethereum/crypto/bls12381"
	"github.com/ethereum/go-ethereum/crypto/bn256"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	ed25519 "github.com/hdevalence/ed25519consensus"

	//lint:ignore SA1019 Needed for precompile
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
	return common.BytesToAddress(append([]byte{0}, (celoPrecompiledContractsAddressOffset - index)))
}

var (
	celoPrecompiledContractsAddressOffset = byte(0xff)

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

	// New in Donut
	edd25519Address          = celoPrecompileAddress(12)
	b12_381G1AddAddress      = celoPrecompileAddress(13)
	b12_381G1MulAddress      = celoPrecompileAddress(14)
	b12_381G1MultiExpAddress = celoPrecompileAddress(15)
	b12_381G2AddAddress      = celoPrecompileAddress(16)
	b12_381G2MulAddress      = celoPrecompileAddress(17)
	b12_381G2MultiExpAddress = celoPrecompileAddress(18)
	b12_381PairingAddress    = celoPrecompileAddress(19)
	b12_381MapFpToG1Address  = celoPrecompileAddress(20)
	b12_381MapFp2ToG2Address = celoPrecompileAddress(21)
	b12_377G1AddAddress      = celoPrecompileAddress(22)
	b12_377G1MulAddress      = celoPrecompileAddress(23)
	b12_377G1MultiExpAddress = celoPrecompileAddress(24)
	b12_377G2AddAddress      = celoPrecompileAddress(25)
	b12_377G2MulAddress      = celoPrecompileAddress(26)
	b12_377G2MultiExpAddress = celoPrecompileAddress(27)
	b12_377PairingAddress    = celoPrecompileAddress(28)
	cip20Address             = celoPrecompileAddress(29)
	cip26Address             = celoPrecompileAddress(30)
)

// PrecompiledContractsByzantium contains the default set of pre-compiled Ethereum
// contracts used in the Byzantium release.
var PrecompiledContractsByzantium = map[common.Address]PrecompiledContract{
	common.BytesToAddress([]byte{1}): &ecrecover{},
	common.BytesToAddress([]byte{2}): &sha256hash{},
	common.BytesToAddress([]byte{3}): &ripemd160hash{},
	common.BytesToAddress([]byte{4}): &dataCopy{},
	common.BytesToAddress([]byte{5}): &bigModExp{},
	common.BytesToAddress([]byte{6}): &bn256AddByzantium{},
	common.BytesToAddress([]byte{7}): &bn256ScalarMulByzantium{},
	common.BytesToAddress([]byte{8}): &bn256PairingByzantium{},

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

// PrecompiledContractsIstanbul contains the default set of pre-compiled Ethereum
// contracts used in the Istanbul release.
var PrecompiledContractsIstanbul = map[common.Address]PrecompiledContract{
	common.BytesToAddress([]byte{1}): &ecrecover{},
	common.BytesToAddress([]byte{2}): &sha256hash{},
	common.BytesToAddress([]byte{3}): &ripemd160hash{},
	common.BytesToAddress([]byte{4}): &dataCopy{},
	common.BytesToAddress([]byte{5}): &bigModExp{},
	common.BytesToAddress([]byte{6}): &bn256AddIstanbul{},
	common.BytesToAddress([]byte{7}): &bn256ScalarMulIstanbul{},
	common.BytesToAddress([]byte{8}): &bn256PairingIstanbul{},
	common.BytesToAddress([]byte{9}): &blake2F{},

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

// PrecompiledContractsDonut contains the default set of pre-compiled Ethereum
// contracts used in the Donit release.
var PrecompiledContractsDonut = map[common.Address]PrecompiledContract{
	common.BytesToAddress([]byte{1}): &ecrecover{},
	common.BytesToAddress([]byte{2}): &sha256hash{},
	common.BytesToAddress([]byte{3}): &ripemd160hash{},
	common.BytesToAddress([]byte{4}): &dataCopy{},
	common.BytesToAddress([]byte{5}): &bigModExp{},
	common.BytesToAddress([]byte{6}): &bn256AddIstanbul{},
	common.BytesToAddress([]byte{7}): &bn256ScalarMulIstanbul{},
	common.BytesToAddress([]byte{8}): &bn256PairingIstanbul{},
	common.BytesToAddress([]byte{9}): &blake2F{},

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

	// TODO(Donut): Add instances
	edd25519Address:          &ed25519Verify{},
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
	// We must make sure not to modify the 'input', so placing the 'v' along with
	// the signature needs to be done on a new allocation
	sig := make([]byte, 65)
	copy(sig, input[64:128])
	sig[64] = v
	// v needs to be at the end for libsecp256k1
	pubKey, err := crypto.Ecrecover(input[:32], sig)
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

// runBn256Add implements the Bn256Add precompile, referenced by both
// Byzantium and Istanbul operations.
func runBn256Add(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
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

// bn256Add implements a native elliptic curve point addition conforming to
// Istanbul consensus rules.
type bn256AddIstanbul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256AddIstanbul) RequiredGas(input []byte) uint64 {
	return params.Bn256AddGasIstanbul
}

func (c *bn256AddIstanbul) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}
	return runBn256Add(input, caller, evm, gas)
}

// bn256AddByzantium implements a native elliptic curve point addition
// conforming to Byzantium consensus rules.
type bn256AddByzantium struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256AddByzantium) RequiredGas(input []byte) uint64 {
	return params.Bn256AddGasByzantium
}

func (c *bn256AddByzantium) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}
	return runBn256Add(input, caller, evm, gas)
}

// runBn256ScalarMul implements the Bn256ScalarMul precompile, referenced by
// both Byzantium and Istanbul operations.
func runBn256ScalarMul(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	p, err := newCurvePoint(getData(input, 0, 64))
	if err != nil {
		return nil, gas, err
	}
	res := new(bn256.G1)
	res.ScalarMult(p, new(big.Int).SetBytes(getData(input, 64, 32)))
	return res.Marshal(), gas, nil
}

// bn256ScalarMulIstanbul implements a native elliptic curve scalar
// multiplication conforming to Istanbul consensus rules.
type bn256ScalarMulIstanbul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256ScalarMulIstanbul) RequiredGas(input []byte) uint64 {
	return params.Bn256ScalarMulGasIstanbul
}

func (c *bn256ScalarMulIstanbul) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}
	return runBn256ScalarMul(input, caller, evm, gas)
}

// bn256ScalarMulByzantium implements a native elliptic curve scalar
// multiplication conforming to Byzantium consensus rules.
type bn256ScalarMulByzantium struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256ScalarMulByzantium) RequiredGas(input []byte) uint64 {
	return params.Bn256ScalarMulGasByzantium
}

func (c *bn256ScalarMulByzantium) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}
	return runBn256ScalarMul(input, caller, evm, gas)
}

var (
	// true32Byte is returned if the bn256 pairing check succeeds.
	true32Byte = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

	// false32Byte is returned if the bn256 pairing check fails.
	false32Byte = make([]byte, 32)

	// errBadPairingInput is returned if the bn256 pairing input is invalid.
	errBadPairingInput = errors.New("bad elliptic curve pairing size")
)

// runBn256Pairing implements the Bn256Pairing precompile, referenced by both
// Byzantium and Istanbul operations.
func runBn256Pairing(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
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
	return params.CallValueTransferGas
}

func (c *transfer) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}
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

	if from == common.ZeroAddress {
		// Mint case: Create cGLD out of thin air
		evm.StateDB.AddBalance(to, value)
	} else {
		// Fail if we're trying to transfer more than the available balance
		if !evm.Context.CanTransfer(evm.StateDB, from, value) {
			return nil, gas, ErrInsufficientBalance
		}

		gas, err = evm.TobinTransfer(evm.StateDB, from, to, gas, value)
	}

	return input, gas, err
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

	if !decimals.IsInt64() || !exponent.IsInt64() || max(decimals.Int64(), exponent.Int64()) > 100000 {
		return nil, gas, fmt.Errorf("Input Error: Decimals or exponent too large")
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
	//   publicKey: 96 bytes, representing the public key (defined as a const in bls package)
	//   signature: 48 bytes, representing the signature on `address` (defined as a const in bls package)
	// the total length of input required is the sum of these constants
	if len(input) != common.AddressLength+blscrypto.PUBLICKEYBYTES+blscrypto.SIGNATUREBYTES {
		return nil, gas, ErrInputLength
	}
	addressBytes := input[:common.AddressLength]

	publicKeyBytes := input[common.AddressLength : common.AddressLength+blscrypto.PUBLICKEYBYTES]
	publicKey, err := bls.DeserializePublicKeyCached(publicKeyBytes)
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

// bn256PairingIstanbul implements a pairing pre-compile for the bn256 curve
// conforming to Istanbul consensus rules.
type bn256PairingIstanbul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256PairingIstanbul) RequiredGas(input []byte) uint64 {
	return params.Bn256PairingBaseGasIstanbul + uint64(len(input)/192)*params.Bn256PairingPerPointGasIstanbul
}

func (c *bn256PairingIstanbul) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}
	return runBn256Pairing(input, caller, evm, gas)
}

// bn256PairingByzantium implements a pairing pre-compile for the bn256 curve
// conforming to Byzantium consensus rules.
type bn256PairingByzantium struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256PairingByzantium) RequiredGas(input []byte) uint64 {
	return params.Bn256PairingBaseGasByzantium + uint64(len(input)/192)*params.Bn256PairingPerPointGasByzantium
}

func (c *bn256PairingByzantium) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}
	return runBn256Pairing(input, caller, evm, gas)
}

type blake2F struct{}

func (c *blake2F) RequiredGas(input []byte) uint64 {
	// If the input is malformed, we can't calculate the gas, return 0 and let the
	// actual call choke and fault.
	if len(input) != blake2FInputLength {
		return 0
	}
	return uint64(binary.BigEndian.Uint32(input[0:4]))
}

const (
	blake2FInputLength        = 213
	blake2FFinalBlockBytes    = byte(1)
	blake2FNonFinalBlockBytes = byte(0)
)

var (
	errBlake2FInvalidInputLength = errors.New("invalid input length")
	errBlake2FInvalidFinalFlag   = errors.New("invalid final flag")
)

func (c *blake2F) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}
	// Make sure the input is valid (correct lenth and final flag)
	if len(input) != blake2FInputLength {
		return nil, gas, errBlake2FInvalidInputLength
	}
	if input[212] != blake2FNonFinalBlockBytes && input[212] != blake2FFinalBlockBytes {
		return nil, gas, errBlake2FInvalidFinalFlag
	}
	// Parse the input into the Blake2b call parameters
	var (
		rounds = binary.BigEndian.Uint32(input[0:4])
		final  = (input[212] == blake2FFinalBlockBytes)

		h [8]uint64
		m [16]uint64
		t [2]uint64
	)
	for i := 0; i < 8; i++ {
		offset := 4 + i*8
		h[i] = binary.LittleEndian.Uint64(input[offset : offset+8])
	}
	for i := 0; i < 16; i++ {
		offset := 68 + i*8
		m[i] = binary.LittleEndian.Uint64(input[offset : offset+8])
	}
	t[0] = binary.LittleEndian.Uint64(input[196:204])
	t[1] = binary.LittleEndian.Uint64(input[204:212])

	// Execute the compression function, extract and return the result
	blake2b.F(&h, m, t, final, rounds)

	output := make([]byte, 64)
	for i := 0; i < 8; i++ {
		offset := i * 8
		binary.LittleEndian.PutUint64(output[offset:offset+8], h[i])
	}
	return output, gas, nil
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

func (c *ed25519Verify) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Setup success/failure return values
	var fail32byte, success32Byte = true32Byte, false32Byte

	// Check if all required arguments are present
	if len(input) < 96 {
		return fail32byte, gas, nil
	}

	publicKey := input[0:32]  // 32 bytes
	signature := input[32:96] // 64 bytes
	message := input[96:]     // arbitrary length

	// Verify the Ed25519 signature against the public key and message
	// https://godoc.org/golang.org/x/crypto/ed25519#Verify
	if ed25519.Verify(publicKey, message, signature) {
		return success32Byte, gas, nil
	}
	return fail32byte, gas, nil
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

	// Note: Passing empty hash as here as it is an extra expense and the hash is not actually used.
	validators := evm.Context.GetValidators(new(big.Int).Sub(blockNumber, common.Big1), common.Hash{})

	// Ensure index, which is guaranteed to be non-negative, is valid.
	if index.Cmp(big.NewInt(int64(len(validators)))) >= 0 {
		return nil, gas, ErrValidatorsOutOfBounds
	}

	validatorAddress := validators[index.Uint64()].Address()
	addressBytes := common.LeftPadBytes(validatorAddress[:], 32)

	return addressBytes, gas, nil
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
func (c *getValidatorBLS) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
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

	// Note: Passing empty hash as here as it is an extra expense and the hash is not actually used.
	validators := evm.Context.GetValidators(new(big.Int).Sub(blockNumber, common.Big1), common.Hash{})

	// Ensure index, which is guaranteed to be non-negative, is valid.
	if index.Cmp(big.NewInt(int64(len(validators)))) >= 0 {
		return nil, gas, ErrValidatorsOutOfBounds
	}

	validator := validators[index.Uint64()]
	uncompressedBytes := validator.BLSPublicKeyUncompressed()
	if len(uncompressedBytes) != 192 {
		return nil, gas, ErrUnexpected
	}

	result := make([]byte, 256)
	for i := 0; i < 256; i++ {
		result[i] = 0
	}

	copyBLSNumber(result, 0, uncompressedBytes, 0)
	copyBLSNumber(result, 64, uncompressedBytes, 48)
	copyBLSNumber(result, 128, uncompressedBytes, 96)
	copyBLSNumber(result, 192, uncompressedBytes, 144)

	return result, gas, nil
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

	// Note: Passing empty hash as here as it is an extra expense and the hash is not actually used.
	validators := evm.Context.GetValidators(new(big.Int).Sub(blockNumber, common.Big1), common.Hash{})

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
	epochSize := new(big.Int).SetUint64(evm.Context.EpochSize).Bytes()
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
	if blockNumber.Cmp(common.Big0) == 0 || blockNumber.Cmp(evm.Context.BlockNumber) > 0 {
		return nil, gas, ErrBlockNumberOutOfBounds
	}

	// Ensure the request is for a sufficiently recent block to limit state expansion.
	historyLimit := new(big.Int).SetUint64(evm.Context.EpochSize * 4)
	if blockNumber.Cmp(new(big.Int).Sub(evm.Context.BlockNumber, historyLimit)) <= 0 {
		return nil, gas, ErrBlockNumberOutOfBounds
	}

	header := evm.Context.GetHeaderByNumber(blockNumber.Uint64())
	if header == nil {
		log.Error("Unexpected failure to retrieve block in getParentSealBitmap precompile", "blockNumber", blockNumber)
		return nil, gas, ErrUnexpected
	}

	extra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		log.Error("Header without Istanbul extra data encountered in getParentSealBitmap precompile", "blockNumber", blockNumber, "err", err)
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
	var header types.Header
	if err := rlp.DecodeBytes(input, &header); err != nil {
		return nil, gas, ErrInputDecode
	}

	// Verify the seal against the engine rules.
	if !evm.Context.VerifySeal(&header) {
		return nil, gas, ErrInputVerification
	}

	// Extract the verified seal from the header.
	extra, err := types.ExtractIstanbulExtra(&header)
	if err != nil {
		log.Error("Header without Istanbul extra data encountered in getVerifiedSealBitmap precompile", "extraData", header.Extra, "err", err)
		// Seal verified by a non-Istanbul engine. Return an error.
		return nil, gas, ErrEngineIncompatible
	}

	return common.LeftPadBytes(extra.AggregatedSeal.Bitmap.Bytes()[:], 32), gas, nil
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

func (c *cip20HashFunctions) Run(input []byte, _ common.Address, _ *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)

	if err != nil {
		return nil, gas, err
	}

	if len(input) == 0 {
		return nil, gas, fmt.Errorf("Input Error: 0-byte input")
	}

	if h, ok := c.hashes[input[0]]; ok {
		output, err := h.Run(input[1:]) // trim selector

		if err != nil {
			return nil, gas, err
		}
		return output, gas, nil
	}

	return nil, gas, fmt.Errorf("Input Error: invalid CIP20 selector: %d", input[0])
}

var (
	errBLS12381InvalidInputLength          = errors.New("invalid input length")
	errBLS12381InvalidFieldElementTopBytes = errors.New("invalid field element top bytes")
	errBLS12381G1PointSubgroup             = errors.New("g1 point is not on correct subgroup")
	errBLS12381G2PointSubgroup             = errors.New("g2 point is not on correct subgroup")
)

// bls12381G1Add implements EIP-2537 G1Add precompile.
type bls12381G1Add struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12381G1Add) RequiredGas(input []byte) uint64 {
	return params.Bls12381G1AddGas
}

func (c *bls12381G1Add) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2537 G1Add precompile.
	// > G1 addition call expects `256` bytes as an input that is interpreted as byte concatenation of two G1 points (`128` bytes each).
	// > Output is an encoding of addition operation result - single G1 point (`128` bytes).
	if len(input) != 256 {
		return nil, gas, errBLS12381InvalidInputLength
	}
	var p0, p1 *bls12381.PointG1

	// Initialize G1
	g := bls12381.NewG1()

	// Decode G1 point p_0
	if p0, err = g.DecodePoint(input[:128]); err != nil {
		return nil, gas, err
	}
	// Decode G1 point p_1
	if p1, err = g.DecodePoint(input[128:]); err != nil {
		return nil, gas, err
	}

	// Compute r = p_0 + p_1
	r := g.New()
	g.Add(r, p0, p1)

	// Encode the G1 point result into 128 bytes
	return g.EncodePoint(r), gas, nil
}

// bls12381G1Mul implements EIP-2537 G1Mul precompile.
type bls12381G1Mul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12381G1Mul) RequiredGas(input []byte) uint64 {
	return params.Bls12381G1MulGas
}

func (c *bls12381G1Mul) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2537 G1Mul precompile.
	// > G1 multiplication call expects `160` bytes as an input that is interpreted as byte concatenation of encoding of G1 point (`128` bytes) and encoding of a scalar value (`32` bytes).
	// > Output is an encoding of multiplication operation result - single G1 point (`128` bytes).
	if len(input) != 160 {
		return nil, gas, errBLS12381InvalidInputLength
	}
	var p0 *bls12381.PointG1

	// Initialize G1
	g := bls12381.NewG1()

	// Decode G1 point
	if p0, err = g.DecodePoint(input[:128]); err != nil {
		return nil, gas, err
	}
	// Decode scalar value
	e := new(big.Int).SetBytes(input[128:])

	// Compute r = e * p_0
	r := g.New()
	g.MulScalar(r, p0, e)

	// Encode the G1 point into 128 bytes
	return g.EncodePoint(r), gas, nil
}

// bls12381G1MultiExp implements EIP-2537 G1MultiExp precompile.
type bls12381G1MultiExp struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12381G1MultiExp) RequiredGas(input []byte) uint64 {
	// Calculate G1 point, scalar value pair length
	k := len(input) / 160
	if k == 0 {
		// Return 0 gas for small input length
		return 0
	}
	// Lookup discount value for G1 point, scalar value pair length
	var discount uint64
	if dLen := len(params.Bls12381MultiExpDiscountTable); k < dLen {
		discount = params.Bls12381MultiExpDiscountTable[k-1]
	} else {
		discount = params.Bls12381MultiExpDiscountTable[dLen-1]
	}
	// Calculate gas and return the result
	return (uint64(k) * params.Bls12381G1MulGas * discount) / 1000
}

func (c *bls12381G1MultiExp) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2537 G1MultiExp precompile.
	// G1 multiplication call expects `160*k` bytes as an input that is interpreted as byte concatenation of `k` slices each of them being a byte concatenation of encoding of G1 point (`128` bytes) and encoding of a scalar value (`32` bytes).
	// Output is an encoding of multiexponentiation operation result - single G1 point (`128` bytes).
	k := len(input) / 160
	if len(input) == 0 || len(input)%160 != 0 {
		return nil, gas, errBLS12381InvalidInputLength
	}
	points := make([]*bls12381.PointG1, k)
	scalars := make([]*big.Int, k)

	// Initialize G1
	g := bls12381.NewG1()

	// Decode point scalar pairs
	for i := 0; i < k; i++ {
		off := 160 * i
		t0, t1, t2 := off, off+128, off+160
		// Decode G1 point
		if points[i], err = g.DecodePoint(input[t0:t1]); err != nil {
			return nil, gas, err
		}
		// Decode scalar value
		scalars[i] = new(big.Int).SetBytes(input[t1:t2])
	}

	// Compute r = e_0 * p_0 + e_1 * p_1 + ... + e_(k-1) * p_(k-1)
	r := g.New()
	g.MultiExp(r, points, scalars)

	// Encode the G1 point to 128 bytes
	return g.EncodePoint(r), gas, nil
}

// bls12381G2Add implements EIP-2537 G2Add precompile.
type bls12381G2Add struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12381G2Add) RequiredGas(input []byte) uint64 {
	return params.Bls12381G2AddGas
}

func (c *bls12381G2Add) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2537 G2Add precompile.
	// > G2 addition call expects `512` bytes as an input that is interpreted as byte concatenation of two G2 points (`256` bytes each).
	// > Output is an encoding of addition operation result - single G2 point (`256` bytes).
	if len(input) != 512 {
		return nil, gas, errBLS12381InvalidInputLength
	}
	var p0, p1 *bls12381.PointG2

	// Initialize G2
	g := bls12381.NewG2()
	r := g.New()

	// Decode G2 point p_0
	if p0, err = g.DecodePoint(input[:256]); err != nil {
		return nil, gas, err
	}
	// Decode G2 point p_1
	if p1, err = g.DecodePoint(input[256:]); err != nil {
		return nil, gas, err
	}

	// Compute r = p_0 + p_1
	g.Add(r, p0, p1)

	// Encode the G2 point into 256 bytes
	return g.EncodePoint(r), gas, nil
}

// bls12381G2Mul implements EIP-2537 G2Mul precompile.
type bls12381G2Mul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12381G2Mul) RequiredGas(input []byte) uint64 {
	return params.Bls12381G2MulGas
}

func (c *bls12381G2Mul) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2537 G2MUL precompile logic.
	// > G2 multiplication call expects `288` bytes as an input that is interpreted as byte concatenation of encoding of G2 point (`256` bytes) and encoding of a scalar value (`32` bytes).
	// > Output is an encoding of multiplication operation result - single G2 point (`256` bytes).
	if len(input) != 288 {
		return nil, gas, errBLS12381InvalidInputLength
	}
	var p0 *bls12381.PointG2

	// Initialize G2
	g := bls12381.NewG2()

	// Decode G2 point
	if p0, err = g.DecodePoint(input[:256]); err != nil {
		return nil, gas, err
	}
	// Decode scalar value
	e := new(big.Int).SetBytes(input[256:])

	// Compute r = e * p_0
	r := g.New()
	g.MulScalar(r, p0, e)

	// Encode the G2 point into 256 bytes
	return g.EncodePoint(r), gas, nil
}

// bls12381G2MultiExp implements EIP-2537 G2MultiExp precompile.
type bls12381G2MultiExp struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12381G2MultiExp) RequiredGas(input []byte) uint64 {
	// Calculate G2 point, scalar value pair length
	k := len(input) / 288
	if k == 0 {
		// Return 0 gas for small input length
		return 0
	}
	// Lookup discount value for G2 point, scalar value pair length
	var discount uint64
	if dLen := len(params.Bls12381MultiExpDiscountTable); k < dLen {
		discount = params.Bls12381MultiExpDiscountTable[k-1]
	} else {
		discount = params.Bls12381MultiExpDiscountTable[dLen-1]
	}
	// Calculate gas and return the result
	return (uint64(k) * params.Bls12381G2MulGas * discount) / 1000
}

func (c *bls12381G2MultiExp) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2537 G2MultiExp precompile logic
	// > G2 multiplication call expects `288*k` bytes as an input that is interpreted as byte concatenation of `k` slices each of them being a byte concatenation of encoding of G2 point (`256` bytes) and encoding of a scalar value (`32` bytes).
	// > Output is an encoding of multiexponentiation operation result - single G2 point (`256` bytes).
	k := len(input) / 288
	if len(input) == 0 || len(input)%288 != 0 {
		return nil, gas, errBLS12381InvalidInputLength
	}
	points := make([]*bls12381.PointG2, k)
	scalars := make([]*big.Int, k)

	// Initialize G2
	g := bls12381.NewG2()

	// Decode point scalar pairs
	for i := 0; i < k; i++ {
		off := 288 * i
		t0, t1, t2 := off, off+256, off+288
		// Decode G1 point
		if points[i], err = g.DecodePoint(input[t0:t1]); err != nil {
			return nil, gas, err
		}
		// Decode scalar value
		scalars[i] = new(big.Int).SetBytes(input[t1:t2])
	}

	// Compute r = e_0 * p_0 + e_1 * p_1 + ... + e_(k-1) * p_(k-1)
	r := g.New()
	g.MultiExp(r, points, scalars)

	// Encode the G2 point to 256 bytes.
	return g.EncodePoint(r), gas, nil
}

// bls12381Pairing implements EIP-2537 Pairing precompile.
type bls12381Pairing struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12381Pairing) RequiredGas(input []byte) uint64 {
	return params.Bls12381PairingBaseGas + uint64(len(input)/384)*params.Bls12381PairingPerPairGas
}

func (c *bls12381Pairing) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2537 Pairing precompile logic.
	// > Pairing call expects `384*k` bytes as an inputs that is interpreted as byte concatenation of `k` slices. Each slice has the following structure:
	// > - `128` bytes of G1 point encoding
	// > - `256` bytes of G2 point encoding
	// > Output is a `32` bytes where last single byte is `0x01` if pairing result is equal to multiplicative identity in a pairing target field and `0x00` otherwise
	// > (which is equivalent of Big Endian encoding of Solidity values `uint256(1)` and `uin256(0)` respectively).
	k := len(input) / 384
	if len(input) == 0 || len(input)%384 != 0 {
		return nil, gas, errBLS12381InvalidInputLength
	}

	// Initialize BLS12-381 pairing engine
	e := bls12381.NewEngine()
	g1, g2 := e.G1, e.G2

	// Decode pairs
	for i := 0; i < k; i++ {
		off := 384 * i
		t0, t1, t2 := off, off+128, off+384

		// Decode G1 point
		p1, err := g1.DecodePoint(input[t0:t1])
		if err != nil {
			return nil, gas, err
		}
		// Decode G2 point
		p2, err := g2.DecodePoint(input[t1:t2])
		if err != nil {
			return nil, gas, err
		}

		// 'point is on curve' check already done,
		// Here we need to apply subgroup checks.
		if !g1.InCorrectSubgroup(p1) {
			return nil, gas, errBLS12381G1PointSubgroup
		}
		if !g2.InCorrectSubgroup(p2) {
			return nil, gas, errBLS12381G2PointSubgroup
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
	return out, gas, nil
}

// decodeBLS12381FieldElement decodes BLS12-381 elliptic curve field element.
// Removes top 16 bytes of 64 byte input.
func decodeBLS12381FieldElement(in []byte) ([]byte, error) {
	if len(in) != 64 {
		return nil, errors.New("invalid field element length")
	}
	// check top bytes
	for i := 0; i < 16; i++ {
		if in[i] != byte(0x00) {
			return nil, errBLS12381InvalidFieldElementTopBytes
		}
	}
	out := make([]byte, 48)
	copy(out[:], in[16:])
	return out, nil
}

// bls12381MapG1 implements EIP-2537 MapG1 precompile.
type bls12381MapG1 struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12381MapG1) RequiredGas(input []byte) uint64 {
	return params.Bls12381MapG1Gas
}

func (c *bls12381MapG1) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2537 Map_To_G1 precompile.
	// > Field-to-curve call expects `64` bytes an an input that is interpreted as a an element of the base field.
	// > Output of this call is `128` bytes and is G1 point following respective encoding rules.
	if len(input) != 64 {
		return nil, gas, errBLS12381InvalidInputLength
	}

	// Decode input field element
	fe, err := decodeBLS12381FieldElement(input)
	if err != nil {
		return nil, gas, err
	}

	// Initialize G1
	g := bls12381.NewG1()

	// Compute mapping
	r, err := g.MapToCurve(fe)
	if err != nil {
		return nil, gas, err
	}

	// Encode the G1 point to 128 bytes
	return g.EncodePoint(r), gas, nil
}

// bls12381MapG2 implements EIP-2537 MapG2 precompile.
type bls12381MapG2 struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12381MapG2) RequiredGas(input []byte) uint64 {
	return params.Bls12381MapG2Gas
}

func (c *bls12381MapG2) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2537 Map_FP2_TO_G2 precompile logic.
	// > Field-to-curve call expects `128` bytes an an input that is interpreted as a an element of the quadratic extension field.
	// > Output of this call is `256` bytes and is G2 point following respective encoding rules.
	if len(input) != 128 {
		return nil, gas, errBLS12381InvalidInputLength
	}

	// Decode input field element
	fe := make([]byte, 96)
	c0, err := decodeBLS12381FieldElement(input[:64])
	if err != nil {
		return nil, gas, err
	}
	copy(fe[48:], c0)
	c1, err := decodeBLS12381FieldElement(input[64:])
	if err != nil {
		return nil, gas, err
	}
	copy(fe[:48], c1)

	// Initialize G2
	g := bls12381.NewG2()

	// Compute mapping
	r, err := g.MapToCurve(fe)
	if err != nil {
		return nil, gas, err
	}

	// Encode the G2 point to 256 bytes
	return g.EncodePoint(r), gas, nil
}

var (
	errBLS12377InvalidInputLength = errors.New("invalid input length")
	errBLS12377G1PointSubgroup    = errors.New("g1 point is not on correct subgroup")
	errBLS12377G2PointSubgroup    = errors.New("g2 point is not on correct subgroup")
)

// bls12377G1Add implements EIP-2539 G1Add precompile.
type bls12377G1Add struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377G1Add) RequiredGas(input []byte) uint64 {
	return params.Bls12377G1AddGas
}

func (c *bls12377G1Add) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2539 G1Add precompile.
	// > G1 addition call expects `256` bytes as an input that is interpreted as byte concatenation of two G1 points (`128` bytes each).
	// > Output is an encoding of addition operation result - single G1 point (`128` bytes).
	if len(input) != 256 {
		return nil, gas, errBLS12377InvalidInputLength
	}
	var p0, p1 *bls12377.PointG1

	// Initialize G1
	g := bls12377.NewG1()

	// Decode G1 point p_0
	if p0, err = g.DecodePoint(input[:128]); err != nil {
		return nil, gas, err
	}
	// Decode G1 point p_1
	if p1, err = g.DecodePoint(input[128:]); err != nil {
		return nil, gas, err
	}

	// Compute r = p_0 + p_1
	r := g.New()
	g.Add(r, p0, p1)

	// Encode the G1 point result into 128 bytes
	return g.EncodePoint(r), gas, nil
}

// bls12377G1Mul implements EIP-2539 G1Mul precompile.
type bls12377G1Mul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377G1Mul) RequiredGas(input []byte) uint64 {
	return params.Bls12377G1MulGas
}

func (c *bls12377G1Mul) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2539 G1Mul precompile.
	// > G1 multiplication call expects `160` bytes as an input that is interpreted as byte concatenation of encoding of G1 point (`128` bytes) and encoding of a scalar value (`32` bytes).
	// > Output is an encoding of multiplication operation result - single G1 point (`128` bytes).
	if len(input) != 160 {
		return nil, gas, errBLS12377InvalidInputLength
	}
	var p0 *bls12377.PointG1

	// Initialize G1
	g := bls12377.NewG1()

	// Decode G1 point
	if p0, err = g.DecodePoint(input[:128]); err != nil {
		return nil, gas, err
	}
	// Decode scalar value
	e := new(big.Int).SetBytes(input[128:])

	// Compute r = e * p_0
	r := g.New()
	g.MulScalar(r, p0, e)

	// Encode the G1 point into 128 bytes
	return g.EncodePoint(r), gas, nil
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

func (c *bls12377G1MultiExp) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2539 G1MultiExp precompile.
	// G1 multiplication call expects `160*k` bytes as an input that is interpreted as byte concatenation of `k` slices each of them being a byte concatenation of encoding of G1 point (`128` bytes) and encoding of a scalar value (`32` bytes).
	// Output is an encoding of multiexponentiation operation result - single G1 point (`128` bytes).
	k := len(input) / 160
	if len(input) == 0 || len(input)%160 != 0 {
		return nil, gas, errBLS12377InvalidInputLength
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
		if points[i], err = g.DecodePoint(input[t0:t1]); err != nil {
			return nil, gas, err
		}
		// Decode scalar value
		scalars[i] = new(big.Int).SetBytes(input[t1:t2])
	}

	// Compute r = e_0 * p_0 + e_1 * p_1 + ... + e_(k-1) * p_(k-1)
	r := g.New()
	g.MultiExp(r, points, scalars)

	// Encode the G1 point to 128 bytes
	return g.EncodePoint(r), gas, nil
}

// bls12377G2Add implements EIP-2539 G2Add precompile.
type bls12377G2Add struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377G2Add) RequiredGas(input []byte) uint64 {
	return params.Bls12377G2AddGas
}

func (c *bls12377G2Add) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2539 G2Add precompile.
	// > G2 addition call expects `512` bytes as an input that is interpreted as byte concatenation of two G2 points (`256` bytes each).
	// > Output is an encoding of addition operation result - single G2 point (`256` bytes).
	if len(input) != 512 {
		return nil, gas, errBLS12377InvalidInputLength
	}
	var p0, p1 *bls12377.PointG2

	// Initialize G2
	g := bls12377.NewG2()
	r := g.New()

	// Decode G2 point p_0
	if p0, err = g.DecodePoint(input[:256]); err != nil {
		return nil, gas, err
	}
	// Decode G2 point p_1
	if p1, err = g.DecodePoint(input[256:]); err != nil {
		return nil, gas, err
	}

	// Compute r = p_0 + p_1
	g.Add(r, p0, p1)

	// Encode the G2 point into 256 bytes
	return g.EncodePoint(r), gas, nil
}

// bls12377G2Mul implements EIP-2539 G2Mul precompile.
type bls12377G2Mul struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377G2Mul) RequiredGas(input []byte) uint64 {
	return params.Bls12377G2MulGas
}

func (c *bls12377G2Mul) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2539 G2MUL precompile logic.
	// > G2 multiplication call expects `288` bytes as an input that is interpreted as byte concatenation of encoding of G2 point (`256` bytes) and encoding of a scalar value (`32` bytes).
	// > Output is an encoding of multiplication operation result - single G2 point (`256` bytes).
	if len(input) != 288 {
		return nil, gas, errBLS12377InvalidInputLength
	}
	var p0 *bls12377.PointG2

	// Initialize G2
	g := bls12377.NewG2()

	// Decode G2 point
	if p0, err = g.DecodePoint(input[:256]); err != nil {
		return nil, gas, err
	}
	// Decode scalar value
	e := new(big.Int).SetBytes(input[256:])

	// Compute r = e * p_0
	r := g.New()
	g.MulScalar(r, p0, e)

	// Encode the G2 point into 256 bytes
	return g.EncodePoint(r), gas, nil
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

func (c *bls12377G2MultiExp) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2539 G2MultiExp precompile logic
	// > G2 multiplication call expects `288*k` bytes as an input that is interpreted as byte concatenation of `k` slices each of them being a byte concatenation of encoding of G2 point (`256` bytes) and encoding of a scalar value (`32` bytes).
	// > Output is an encoding of multiexponentiation operation result - single G2 point (`256` bytes).
	k := len(input) / 288
	if len(input) == 0 || len(input)%288 != 0 {
		return nil, gas, errBLS12377InvalidInputLength
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
		if points[i], err = g.DecodePoint(input[t0:t1]); err != nil {
			return nil, gas, err
		}
		// Decode scalar value
		scalars[i] = new(big.Int).SetBytes(input[t1:t2])
	}

	// Compute r = e_0 * p_0 + e_1 * p_1 + ... + e_(k-1) * p_(k-1)
	r := g.New()
	g.MultiExp(r, points, scalars)

	// Encode the G2 point to 256 bytes.
	return g.EncodePoint(r), gas, nil
}

// bls12377Pairing implements EIP-2539 Pairing precompile.
type bls12377Pairing struct{}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bls12377Pairing) RequiredGas(input []byte) uint64 {
	return params.Bls12377PairingBaseGas + uint64(len(input)/384)*params.Bls12377PairingPerPairGas
}

func (c *bls12377Pairing) Run(input []byte, caller common.Address, evm *EVM, gas uint64) ([]byte, uint64, error) {
	gas, err := debitRequiredGas(c, input, gas)
	if err != nil {
		return nil, gas, err
	}

	// Implements EIP-2539 Pairing precompile logic.
	// > Pairing call expects `384*k` bytes as an inputs that is interpreted as byte concatenation of `k` slices. Each slice has the following structure:
	// > - `128` bytes of G1 point encoding
	// > - `256` bytes of G2 point encoding
	// > Output is a `32` bytes where last single byte is `0x01` if pairing result is equal to multiplicative identity in a pairing target field and `0x00` otherwise
	// > (which is equivalent of Big Endian encoding of Solidity values `uint256(1)` and `uin256(0)` respectively).
	k := len(input) / 384
	if len(input) == 0 || len(input)%384 != 0 {
		return nil, gas, errBLS12377InvalidInputLength
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
			return nil, gas, err
		}
		// Decode G2 point
		p2, err := g2.DecodePoint(input[t1:t2])
		if err != nil {
			return nil, gas, err
		}

		// 'point is on curve' check already done,
		// Here we need to apply subgroup checks.
		if !g1.InCorrectSubgroup(p1) {
			return nil, gas, errBLS12377G1PointSubgroup
		}
		if !g2.InCorrectSubgroup(p2) {
			return nil, gas, errBLS12377G2PointSubgroup
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
	return out, gas, nil
}
