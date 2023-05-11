package testutil

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/contracts/config"
)

type RegistryMock struct {
	ContractMock
	Contracts map[common.Hash]common.Address
}

func (rm *RegistryMock) GetAddressFor(id common.Hash) common.Address {
	addr, ok := rm.Contracts[id]
	if !ok {
		return common.ZeroAddress
	}
	return addr
}

func NewRegistryMock() *RegistryMock {
	registryMock := &RegistryMock{
		Contracts: make(map[common.Hash]common.Address),
	}
	contract := NewContractMock(abis.Registry, registryMock)
	registryMock.ContractMock = contract
	return registryMock
}

func (rm *RegistryMock) AddContract(id common.Hash, address common.Address) {
	rm.Contracts[id] = address
}

func NewSingleMethodRunner(registryId common.Hash, methodName string, mockFn interface{}) *MockEVMRunner {
	runner := NewMockEVMRunner()
	registry := NewRegistryMock()
	runner.RegisterContract(config.RegistrySmartContractAddress, registry)

	contract := NewSingleMethodContract(registryId, methodName, mockFn)

	someAdddress := common.HexToAddress("0x045454545")
	registry.AddContract(registryId, someAdddress)
	runner.RegisterContract(someAdddress, contract)

	return runner
}
