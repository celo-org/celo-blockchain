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

// NewSystemEVM creates a new SystemEVM pointing at (header,state)
func (factory *defaultSystemEVMFactory) NewSystemEVM(header *types.Header, state vm.StateDB) vm.SystemEVM {
	return NewSystemEVM(factory.chain, header, state)
}

// NewCurrentHeadSystemEVM creates a new SystemEVM pointing a current's blockchain Head
func (factory *defaultSystemEVMFactory) NewCurrentHeadSystemEVM() (vm.SystemEVM, error) {
	return NewSystemEVMForCurrentBlock(factory.chain)
}
