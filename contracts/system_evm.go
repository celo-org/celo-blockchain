package contracts

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/core/vm/vmcontext"
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

type SystemEVMProvider func(sender common.Address) *vm.EVM

func newEVMProvider(chain vm.ChainContext, vmConfig vm.Config, header *types.Header, state vm.StateDB) SystemEVMProvider {
	return func(sender common.Address) *vm.EVM {
		// The EVM Context requires a msg, but the actual field values don't really matter for this case.
		// Putting in zero values.
		context := vmcontext.New(sender, common.Big0, header, chain, nil)
		return vm.NewEVM(context, state, chain.Config(), vmConfig)
	}

}

type systemEVM struct {
	provider SystemEVMProvider
	state    vm.StateDB

	dontMeterGas bool
}

func NewSystemEVMForCurrentBlock(chain ExtendedChainContext) (vm.SystemEVM, error) {
	header := chain.CurrentHeader()
	// FIXME small race condition here (Head changes between get header & get state)
	state, err := chain.State()
	if err != nil {
		return nil, err
	}
	return NewSystemEVM(chain, header, state), nil
}

func NewSystemEVM(chain ExtendedChainContext, header *types.Header, state vm.StateDB) vm.SystemEVM {

	return &systemEVM{
		provider: newEVMProvider(chain, *chain.GetVMConfig(), header, state),
		state:    state,
	}
}

func (ev *systemEVM) Execute(sender, recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	evm := ev.provider(sender)
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	return evm.Call(vm.AccountRef(sender), recipient, input, gas, value)
}

func (ev *systemEVM) Query(sender, recipient common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	evm := ev.provider(sender)
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	return evm.StaticCall(vm.AccountRef(sender), recipient, input, gas)
}

func (ev *systemEVM) StopGasMetering() {
	ev.dontMeterGas = true
}

func (ev *systemEVM) StartGasMetering() {
	ev.dontMeterGas = false
}

// GetStateDB implements Backend.GetStateDB
func (ev *systemEVM) GetStateDB() vm.StateDB {
	return ev.state
}

// SharedSystemEVM is an evm caller that REUSES an evm
// This MUST NOT BE USED, but it's here for backward compatibility
// purposes
type SharedSystemEVM struct{ *vm.EVM }

func (sev *SharedSystemEVM) Execute(sender, recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	return sev.Call(vm.AccountRef(sender), recipient, input, gas, value)
}

func (sev *SharedSystemEVM) Query(sender, recipient common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	return sev.StaticCall(vm.AccountRef(sender), recipient, input, gas)
}
