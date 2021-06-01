package testutil

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/params"
)

type BlockchainParametersMock struct {
	ContractMock

	MinimumVersion                             params.VersionInfo
	BlockGasLimitValue                         *big.Int
	LookbackWindow                             *big.Int
	IntrinsicGasForAlternativeFeeCurrencyValue *big.Int
}

func NewBlockchainParametersMock() *BlockchainParametersMock {
	mock := &BlockchainParametersMock{
		MinimumVersion:     params.VersionInfo{Major: 1, Minor: 0, Patch: 0},
		BlockGasLimitValue: big.NewInt(20000000),
		LookbackWindow:     big.NewInt(3),
		IntrinsicGasForAlternativeFeeCurrencyValue: big.NewInt(10000),
	}

	contract := NewContractMock(abis.BlockchainParameters, mock)
	mock.ContractMock = contract
	return mock
}

func (bp *BlockchainParametersMock) GetMinimumClientVersion() (uint64, uint64, uint64) {
	return bp.MinimumVersion.Major, bp.MinimumVersion.Minor, bp.MinimumVersion.Patch
}
func (bp *BlockchainParametersMock) BlockGasLimit() *big.Int {
	return bp.BlockGasLimitValue
}
func (bp *BlockchainParametersMock) GetUptimeLookbackWindow() *big.Int {
	return bp.LookbackWindow
}
func (bp *BlockchainParametersMock) IntrinsicGasForAlternativeFeeCurrency() *big.Int {
	return bp.IntrinsicGasForAlternativeFeeCurrencyValue
}
