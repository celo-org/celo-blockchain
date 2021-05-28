package testutil

import (
	"errors"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
)

type RegistryMock struct {
	contractMock

	Contracts map[common.Hash]common.Address
}

func NewRegistryMock() *RegistryMock {
	parsedAbi, err := abi.JSON(strings.NewReader(contracts.RegistryABIString))
	if err != nil {
		panic(err)
	}

	registryContracts := make(map[common.Hash]common.Address)

	getAddressFor := func(inputs []interface{}) (outputs []interface{}, err error) {
		id := inputs[0].([32]uint8)
		addr, ok := registryContracts[id]
		if !ok {
			return nil, errors.New("contract not in registry")
		}
		return []interface{}{addr}, nil
	}

	return &RegistryMock{
		contractMock: contractMock{
			abi: parsedAbi,
			methods: map[string]solidityMethod{
				"getAddressFor": getAddressFor,
			},
		},
		Contracts: registryContracts,
	}
}

func (rm *RegistryMock) AddContract(id common.Hash, address common.Address) {
	rm.Contracts[id] = address
}
