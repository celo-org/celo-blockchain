package testutil

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/config"
)

type CeloMock struct {
	Runner               *MockEVMRunner
	Registry             *RegistryMock
	BlockchainParameters *BlockchainParametersMock
}

func NewCeloMock() CeloMock {
	celo := CeloMock{
		Runner:               NewMockEVMRunner(),
		Registry:             NewRegistryMock(),
		BlockchainParameters: NewBlockchainParametersMock(),
	}

	celo.Runner.RegisterContract(config.RegistrySmartContractAddress, celo.Registry)

	celo.Registry.AddContract(config.BlockchainParametersRegistryId, common.HexToAddress("0x01"))
	celo.Runner.RegisterContract(common.HexToAddress("0x01"), celo.BlockchainParameters)

	return celo
}
