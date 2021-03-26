package contracts

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

var (
	ErrTobinTaxZeroDenominator  = errors.New("Tobin tax denominator equal to zero")
	ErrTobinTaxInvalidNumerator = errors.New("Tobin tax numerator greater than denominator")
)

func TobinTax(evm *vm.EVM, sender common.Address) (numerator *big.Int, denominator *big.Int, reserveAddress common.Address, err error) {
	reserveAddress, err = GetRegisteredAddress(evm, params.ReserveRegistryId)
	if err != nil {
		return nil, nil, common.ZeroAddress, err
	}

	ret, _, err := evm.Call(systemCaller, reserveAddress, params.TobinTaxFunctionSelector, params.MaxGasForGetOrComputeTobinTax, big.NewInt(0))
	if err != nil {
		return nil, nil, common.ZeroAddress, err
	}

	// Expected size of ret is 64 bytes because getOrComputeTobinTax() returns two uint256 values,
	// each of which is equivalent to 32 bytes
	if binary.Size(ret) != 64 {
		return nil, nil, common.ZeroAddress, errors.New("Length of tobin tax not equal to 64 bytes")
	}
	numerator = new(big.Int).SetBytes(ret[0:32])
	denominator = new(big.Int).SetBytes(ret[32:64])
	if denominator.Cmp(common.Big0) == 0 {
		return nil, nil, common.ZeroAddress, ErrTobinTaxZeroDenominator
	}
	if numerator.Cmp(denominator) == 1 {
		return nil, nil, common.ZeroAddress, ErrTobinTaxInvalidNumerator
	}
	return numerator, denominator, reserveAddress, nil
}

func ComputeTobinTax(evm *vm.EVM, sender common.Address, transferAmount *big.Int) (tax *big.Int, taxRecipient common.Address, err error) {
	numerator, denominator, recipient, err := TobinTax(evm, sender)
	if err != nil {
		return nil, common.ZeroAddress, err
	}

	tobinTax := new(big.Int).Div(new(big.Int).Mul(numerator, transferAmount), denominator)
	return tobinTax, recipient, nil
}
