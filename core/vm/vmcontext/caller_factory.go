package vmcontext

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
)

type defaultEVMFactory struct {
	chain vm.ChainContext
}

func NewEVMFactory(chain vm.ChainContext) vm.EVMFactory {
	return &defaultEVMFactory{chain}
}

func (f *defaultEVMFactory) EVM(header *types.Header, state vm.StateDB) *vm.EVM {
	// TODO look at changing the params to include origin and gasPrice this
	// would allow this component to be used for user contract calls as well as
	// system contract calls.
	// func (f *defaultEVMFactory) EVM(origin common.Address, gasPrice *big.Int, header *types.Header, state vm.StateDB) *vm.EVM {

	// Zero address because this is for system calls only
	// And hence the gas price of 0
	context := New(common.ZeroAddress, common.Big0, header, f.chain, nil)
	return vm.NewEVM(context, state, f.chain.Config(), *f.chain.GetVMConfig())
}

func (f *defaultEVMFactory) Chain() vm.ChainContext {
	return f.chain
}

type defaultEVMProvider struct {
	factory vm.EVMFactory
	header  *types.Header
	state   vm.StateDB
}

func NewEVMProvider(factory vm.EVMFactory, header *types.Header, state vm.StateDB) *defaultEVMProvider {
	return &defaultEVMProvider{
		factory: factory,
		header:  header,
		state:   state,
	}
}

func (p *defaultEVMProvider) EVM() *vm.EVM {
	return p.factory.EVM(p.header, p.state)
}
