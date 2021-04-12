package vmcontext

import (
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
)

type defaultEVMCallerFactory struct {
	chain vm.ChainContext
}

func NewEVMCallerFactory(chain vm.ChainContext) vm.EVMCallerFactory {
	return &defaultEVMCallerFactory{chain}
}

func (factory *defaultEVMCallerFactory) NewEVMCaller(header *types.Header, state vm.StateDB) (vm.EVMCaller, error) {
	if header == nil {
		return NewEVMCallerForCurrentBlock(factory.chain)
	}
	return NewEVMCaller(factory.chain, header, state), nil
}
