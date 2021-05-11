package vmcontext

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
)

// VMAddress is the address the VM uses to make internal calls to contracts
var VMAddress = common.ZeroAddress

// ExtendedChainContext extends ChainContext providing relevant methods
// for EVMRunner creation
type ExtendedChainContext interface {
	ChainContext

	// GetVMConfig returns the node's vm configuration
	GetVMConfig() *vm.Config

	CurrentHeader() *types.Header

	State() (*state.StateDB, error)
}

// SystemEVMRunnerBuilder returns a builder factory for a System's EVMRunner
// DO NOT USE, only for backwards compatibility
func SystemEVMRunnerBuilder(chain ExtendedChainContext) func(header *types.Header, state vm.StateDB) (vm.EVMRunner, error) {
	return func(header *types.Header, state vm.StateDB) (vm.EVMRunner, error) {
		if header == nil {
			return NewSystemEVMRunnerForCurrentBlock(chain)
		}

		return NewSystemEVMRunner(chain, header, state), nil
	}
}

type evmRunner struct {
	newEVM func() *vm.EVM
	state  vm.StateDB

	dontMeterGas bool
}

// NewSystemEVMRunnerForCurrentBlock creates an EVMRunner where calls are originated by the VMAddress (uses chain's current block)
func NewSystemEVMRunnerForCurrentBlock(chain ExtendedChainContext) (vm.EVMRunner, error) {
	return newEVMRunnerForCurrentBlock(chain, VMAddress, common.Big0)
}

// NewSystemEVMRunner creates an EVMRunner where calls are originated by the VMAddress
func NewSystemEVMRunner(chain ExtendedChainContext, header *types.Header, state vm.StateDB) vm.EVMRunner {
	return newEVMRunner(chain, VMAddress, common.Big0, header, state)
}

func newEVMRunnerForCurrentBlock(chain ExtendedChainContext, sender common.Address, gasPrice *big.Int) (vm.EVMRunner, error) {
	header := chain.CurrentHeader()
	// FIXME small race condition here (Head changes between get header & get state)
	state, err := chain.State()
	if err != nil {
		return nil, err
	}
	return newEVMRunner(chain, sender, gasPrice, header, state), nil
}

func newEVMRunner(chain ExtendedChainContext, sender common.Address, gasPrice *big.Int, header *types.Header, state vm.StateDB) vm.EVMRunner {

	return &evmRunner{
		state: state,
		newEVM: func() *vm.EVM {
			// The EVM Context requires a msg, but the actual field values don't really matter for this case.
			// Putting in zero values.
			context := New(sender, gasPrice, header, chain, nil)
			return vm.NewEVM(context, state, chain.Config(), *chain.GetVMConfig())
		},
	}
}

func (ev *evmRunner) Execute(recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	evm := ev.newEVM()
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	return evm.Call(vm.AccountRef(evm.Origin), recipient, input, gas, value)
}

func (ev *evmRunner) Query(recipient common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	evm := ev.newEVM()
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	return evm.StaticCall(vm.AccountRef(evm.Origin), recipient, input, gas)
}

func (ev *evmRunner) StopGasMetering() {
	ev.dontMeterGas = true
}

func (ev *evmRunner) StartGasMetering() {
	ev.dontMeterGas = false
}

// GetStateDB implements Backend.GetStateDB
func (ev *evmRunner) GetStateDB() vm.StateDB {
	return ev.state
}

// SharedEVMRunner is an evm runner that REUSES an evm
// This MUST NOT BE USED, but it's here for backward compatibility
// purposes
type SharedEVMRunner struct{ *vm.EVM }

func (sev *SharedEVMRunner) Execute(recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	return sev.Call(vm.AccountRef(VMAddress), recipient, input, gas, value)
}

func (sev *SharedEVMRunner) Query(recipient common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	return sev.StaticCall(vm.AccountRef(VMAddress), recipient, input, gas)
}
