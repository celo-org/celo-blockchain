package testutil

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/config"
)

type CeloMock struct {
	Runner               *MockEVMRunner
	Registry             *RegistryMock
	BlockchainParameters *BlockchainParametersMock
	Whitelist            *WhitelistMock
	Token                *TokenMock
}

func NewCeloMock() CeloMock {
	celo := CeloMock{
		Runner:               NewMockEVMRunner(),
		Registry:             NewRegistryMock(),
		BlockchainParameters: NewBlockchainParametersMock(),
		Whitelist:            NewWhitelistMock(),
		Token:                NewTokenMock(),
	}

	celo.Runner.RegisterContract(config.RegistrySmartContractAddress, celo.Registry)

	celo.Registry.AddContract(config.BlockchainParametersRegistryId, common.HexToAddress("0x01"))
	celo.Runner.RegisterContract(common.HexToAddress("0x01"), celo.BlockchainParameters)

	celo.Registry.AddContract(config.FeeCurrencyWhitelistRegistryId, common.HexToAddress("0x03"))
	celo.Runner.RegisterContract(common.HexToAddress("0x03"), celo.Whitelist)

	celo.Runner.RegisterContract(common.HexToAddress("0x02"), celo.Token)
	celo.Runner.RegisterContract(common.HexToAddress("0x05"), celo.Token)

	return celo
}
