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

// evmRunnerContext defines methods required to create an EVMRunner
type evmRunnerContext interface {
	chainContext

	// GetVMConfig returns the node's vm configuration
	GetVMConfig() *vm.Config

	CurrentHeader() *types.Header

	State() (*state.StateDB, error)
}

type evmRunner struct {
	newEVM func() *vm.EVM
	state  vm.StateDB

	dontMeterGas bool
}

func NewEVMRunner(chain evmRunnerContext, header *types.Header, state vm.StateDB) vm.EVMRunner {

	return &evmRunner{
		state: state,
		newEVM: func() *vm.EVM {
			// The EVM Context requires a msg, but the actual field values don't really matter for this case.
			// Putting in zero values.
			context := New(VMAddress, common.Big0, header, chain, nil)
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