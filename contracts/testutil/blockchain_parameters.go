package testutil

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/contracts/config"
)

type BlockchainParametersMock struct {
	ContractMock

	MinimumVersion                             config.VersionInfo
	BlockGasLimitValue                         *big.Int
	LookbackWindow                             *big.Int
	IntrinsicGasForAlternativeFeeCurrencyValue *big.Int
}

func NewBlockchainParametersMock() *BlockchainParametersMock {
	mock := &BlockchainParametersMock{
		MinimumVersion:     config.VersionInfo{Major: 1, Minor: 0, Patch: 0},
		BlockGasLimitValue: big.NewInt(20000000),
		LookbackWindow:     big.NewInt(3),
		IntrinsicGasForAlternativeFeeCurrencyValue: big.NewInt(10000),
	}

	contract := NewContractMock(abis.BlockchainParameters, mock)
	mock.ContractMock = contract
	return mock
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

type WhitelistMock struct {
	ContractMock
}

func NewWhitelistMock() *WhitelistMock {
	mock := &WhitelistMock{}

	contract := NewContractMock(abis.FeeCurrency, mock)
	mock.ContractMock = contract
	return mock
}

func (bp *WhitelistMock) GetWhitelist() []common.Address {
	ok := common.HexToAddress("02")
	return []common.Address{ok}
}

type TokenMock struct {
	ContractMock
}

func NewTokenMock() *TokenMock {
	mock := &TokenMock{}

	contract := NewContractMock(abis.ERC20, mock)
	mock.ContractMock = contract
	return mock
}

func (bp *TokenMock) BalanceOf(addr common.Address) *big.Int {
	return big.NewInt(1_000_000_000_000_000)
}
