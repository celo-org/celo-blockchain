package vmcontext

import (
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
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
}

type evmRunner struct {
	newEVM   func(from common.Address) *vm.EVM
	state    vm.StateDB
	blockNum uint64

	dontMeterGas bool
}

func NewEVMRunner(chain evmRunnerContext, header *types.Header, state vm.StateDB) vm.EVMRunner {
	return &evmRunner{
		blockNum: header.Number.Uint64(),
		state:    state,
		newEVM: func(from common.Address) *vm.EVM {
			// The EVM Context requires a msg, but the actual field values don't really matter for this case.
			// Putting in zero values for gas price and tx fee recipient
			blockContext := NewBlockContext(header, chain, nil)
			txContext := vm.TxContext{
				Origin:   from,
				GasPrice: common.Big0,
			}
			return vm.NewEVM(blockContext, txContext, state, chain.Config(), *chain.GetVMConfig())
		},
	}
}

func (ev *evmRunner) Execute(recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, err error) {
	evm := ev.newEVM(VMAddress)
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	ret, _, err = evm.Call(vm.AccountRef(evm.Origin), recipient, input, gas, value)
	if err != nil {
		err = fmt.Errorf("execute failed at block %d: %w", ev.blockNum, err)
	}
	return ret, err
}

func (ev *evmRunner) ExecuteFrom(sender, recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, err error) {
	evm := ev.newEVM(sender)
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	ret, _, err = evm.Call(vm.AccountRef(sender), recipient, input, gas, value)
	if err != nil {
		err = fmt.Errorf("execute from failed at block %d: %w", ev.blockNum, err)
	}
	return ret, err
}

func (ev *evmRunner) ExecuteAndDiscardChanges(recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, err error) {
	evm := ev.newEVM(VMAddress)
	var snapshot = evm.StateDB.Snapshot()
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	ret, _, err = evm.Call(vm.AccountRef(evm.Origin), recipient, input, gas, value)
	if err != nil {
		err = fmt.Errorf("execute and discard changes failed at block %d: %w", ev.blockNum, err)
	}
	evm.StateDB.RevertToSnapshot(snapshot)
	return ret, err
}

func (ev *evmRunner) Query(recipient common.Address, input []byte, gas uint64) (ret []byte, err error) {
	evm := ev.newEVM(VMAddress)
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	ret, _, err = evm.StaticCall(vm.AccountRef(evm.Origin), recipient, input, gas)
	if err != nil {
		err = fmt.Errorf("query failed at block %d: %w", ev.blockNum, err)
	}
	return ret, err
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

func (sev *SharedEVMRunner) Execute(recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, err error) {
	ret, _, err = sev.Call(vm.AccountRef(VMAddress), recipient, input, gas, value)
	return ret, err
}

func (sev *SharedEVMRunner) ExecuteFrom(sender, recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, err error) {
	ret, _, err = sev.Call(vm.AccountRef(sender), recipient, input, gas, value)
	return ret, err
}

func (sev *SharedEVMRunner) ExecuteAndDiscardChanges(recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, err error) {
	var snapshot = sev.StateDB.Snapshot()
	ret, _, err = sev.Call(vm.AccountRef(VMAddress), recipient, input, gas, value)
	sev.StateDB.RevertToSnapshot(snapshot)
	return ret, err
}

func (sev *SharedEVMRunner) Query(recipient common.Address, input []byte, gas uint64) (ret []byte, err error) {
	ret, _, err = sev.StaticCall(vm.AccountRef(VMAddress), recipient, input, gas)
	return ret, err
}
