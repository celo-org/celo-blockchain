package testutil

import (
	"math/big"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/params"
)

type BlockchainParametersMock struct {
	contractMock

	MinimumVersion                        params.VersionInfo
	BlockGasLimit                         *big.Int
	LookbackWindow                        *big.Int
	IntrinsicGasForAlternativeFeeCurrency *big.Int
}

func NewBlockchainParametersMock() *BlockchainParametersMock {
	parsedAbi, err := abi.JSON(strings.NewReader(blockchain_parameters.ABIString))
	if err != nil {
		panic(err)
	}

	var mock BlockchainParametersMock
	mock = BlockchainParametersMock{
		contractMock: contractMock{
			abi: parsedAbi,
			methods: map[string]solidityMethod{
				"getMinimumClientVersion": func(inputs []interface{}) (outputs []interface{}, err error) {
					return []interface{}{mock.MinimumVersion.Major, mock.MinimumVersion.Minor, mock.MinimumVersion.Patch}, nil
				},
				"blockGasLimit": func(inputs []interface{}) (outputs []interface{}, err error) {
					return []interface{}{mock.BlockGasLimit}, nil
				},
				"getUptimeLookbackWindow": func(inputs []interface{}) (outputs []interface{}, err error) {
					return []interface{}{mock.LookbackWindow}, nil
				},
				"intrinsicGasForAlternativeFeeCurrency": func(inputs []interface{}) (outputs []interface{}, err error) {
					return []interface{}{mock.IntrinsicGasForAlternativeFeeCurrency}, nil
				},
			},
		},
		MinimumVersion:                        params.VersionInfo{Major: 1, Minor: 0, Patch: 0},
		BlockGasLimit:                         big.NewInt(20000000),
		LookbackWindow:                        big.NewInt(3),
		IntrinsicGasForAlternativeFeeCurrency: big.NewInt(10000),
	}

	return &mock
}
