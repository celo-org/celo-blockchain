package contracts

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/vm"
)

type DefaultContext struct{}

func (dc DefaultContext) GetRegisteredAddressFunc(evm *vm.EVM, registryId common.Hash) (common.Address, error) {
	return GetRegisteredAddress(evm, registryId)
}

var Context DefaultContext
