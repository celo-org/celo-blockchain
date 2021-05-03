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

type defaultSystemEVMFactory struct {
	chain ExtendedChainContext
}

func NewSystemEVMFactory(chain ExtendedChainContext) vm.SystemEVMFactory {
	return &defaultSystemEVMFactory{chain}
}

func (factory *defaultSystemEVMFactory) NewSystemEVM(header *types.Header, state vm.StateDB) (vm.SystemEVM, error) {
	if header == nil {
		return NewSystemEVMForCurrentBlock(factory.chain)
	}
	return NewSystemEVM(factory.chain, header, state), nil
}
