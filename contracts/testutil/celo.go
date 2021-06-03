package testutil

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/params"
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

	celo.Runner.RegisterContract(params.RegistrySmartContractAddress, celo.Registry)

	celo.Registry.AddContract(params.BlockchainParametersRegistryId, common.HexToAddress("0x01"))
	celo.Runner.RegisterContract(common.HexToAddress("0x01"), celo.BlockchainParameters)

	return celo
}
