package reserve

import (
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

var (
	ErrTobinTaxZeroDenominator  = errors.New("tobin tax denominator equal to zero")
	ErrTobinTaxInvalidNumerator = errors.New("tobin tax numerator greater than denominator")
)

type Ratio struct {
	numerator, denominator *big.Int
}

func (r *Ratio) Apply(value *big.Int) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(r.numerator, value), r.denominator)
}

func TobinTax(vmRunner vm.EVMRunner, sender common.Address) (tax Ratio, reserveAddress common.Address, err error) {

	reserveAddress, err = contracts.GetRegisteredAddress(vmRunner, params.ReserveRegistryId)
	if err != nil {
		return Ratio{}, common.ZeroAddress, err
	}

	ret, _, err := vmRunner.Execute(reserveAddress, params.TobinTaxFunctionSelector, params.MaxGasForGetOrComputeTobinTax, big.NewInt(0))
	if err != nil {
		return Ratio{}, common.ZeroAddress, err
	}

	// Expected size of ret is 64 bytes because getOrComputeTobinTax() returns two uint256 values,
	// each of which is equivalent to 32 bytes
	if binary.Size(ret) != 64 {
		return Ratio{}, common.ZeroAddress, errors.New("length of tobin tax not equal to 64 bytes")
	}
	numerator := new(big.Int).SetBytes(ret[0:32])
	denominator := new(big.Int).SetBytes(ret[32:64])
	if denominator.Cmp(common.Big0) == 0 {
		return Ratio{}, common.ZeroAddress, ErrTobinTaxZeroDenominator
	}
	if numerator.Cmp(denominator) == 1 {
		return Ratio{}, common.ZeroAddress, ErrTobinTaxInvalidNumerator
	}
	return Ratio{numerator, denominator}, reserveAddress, nil
}

func ComputeTobinTax(vmRunner vm.EVMRunner, sender common.Address, transferAmount *big.Int) (tax *big.Int, taxRecipient common.Address, err error) {
	taxRatio, recipient, err := TobinTax(vmRunner, sender)
	if err != nil {
		return nil, common.ZeroAddress, err
	}

	return taxRatio.Apply(transferAmount), recipient, nil
}
