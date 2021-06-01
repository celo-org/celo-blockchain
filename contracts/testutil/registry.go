package testutil

import (
	"errors"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/abis"
)

type RegistryMock struct {
	ContractMock
	Contracts map[common.Hash]common.Address
}

func (rm *RegistryMock) GetAddressFor(id common.Hash) (common.Address, error) {
	addr, ok := rm.Contracts[id]
	if !ok {
		return common.ZeroAddress, errors.New("contract not in registry")
	}
	return addr, nil
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
