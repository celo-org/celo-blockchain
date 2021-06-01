package testutil

import (
	"fmt"
	"reflect"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/params"
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
	runner.RegisterContract(params.RegistrySmartContractAddress, registry)

	contractAbi := abis.AbiFor(registryId)
	if contractAbi == nil {
		panic(fmt.Sprintf("no abi for id: %s", registryId.Hex()))
	}

	method, ok := contractAbi.Methods[methodName]
	if !ok {
		panic(fmt.Sprintf("no method named: %s", methodName))
	}

	contract := ContractMock{
		methods: []MethodMock{
			*NewMethod(&method, reflect.ValueOf(mockFn)),
		},
	}

	someAdddress := common.HexToAddress("0x045454545")
	registry.AddContract(registryId, someAdddress)
	runner.RegisterContract(someAdddress, &contract)

	return runner
}
