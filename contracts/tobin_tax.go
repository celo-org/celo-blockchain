package contracts

import (
	"encoding/binary"
	goerrors "errors"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/params"
)

func TobinTax(evm ContractCaller, sender common.Address) (numerator *big.Int, denominator *big.Int, reserveAddress *common.Address, err error) {
	reserveAddress, err = GetRegisteredAddress(evm, params.ReserveRegistryId)
	if err != nil {
		return nil, nil, nil, err
	}

	ret, _, err := evm.Call(*reserveAddress, params.TobinTaxFunctionSelector, params.MaxGasForGetOrComputeTobinTax, big.NewInt(0))
	if err != nil {
		return nil, nil, nil, err
	}

	// Expected size of ret is 64 bytes because getOrComputeTobinTax() returns two uint256 values,
	// each of which is equivalent to 32 bytes
	if binary.Size(ret) != 64 {
		return nil, nil, nil, goerrors.New("Length of tobin tax not equal to 64 bytes")
	}
	numerator = new(big.Int).SetBytes(ret[0:32])
	denominator = new(big.Int).SetBytes(ret[32:64])
	if denominator.Cmp(common.Big0) == 0 {
		return nil, nil, nil, goerrors.New("Tobin tax denominator equal to zero")
	}
	if numerator.Cmp(denominator) == 1 {
		return nil, nil, nil, goerrors.New("Tobin tax numerator greater than denominator")
	}
	return numerator, denominator, reserveAddress, nil
}
