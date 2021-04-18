package vmcontext

import (
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
)

// ChainContext supports retrieving chain data and consensus parameters
// from the blockchain to be used during transaction processing.
type ExtendedChainContext interface {
	vm.ChainContext

	// GetVMConfig returns the node's vm configuration
	GetVMConfig() *vm.Config

	CurrentHeader() *types.Header

	State() (*state.StateDB, error)
}

type defaultEVMCallerFactory struct {
	chain ExtendedChainContext
}

func NewEVMCallerFactory(chain ExtendedChainContext) vm.EVMCallerFactory {
	return &defaultEVMCallerFactory{chain}
}

func (factory *defaultEVMCallerFactory) NewEVMCaller(header *types.Header, state vm.StateDB) (vm.EVMCaller, error) {
	if header == nil {
		return NewEVMCallerForCurrentBlock(factory.chain)
	}
	return NewEVMCaller(factory.chain, header, state), nil
}
