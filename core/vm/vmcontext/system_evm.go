package vmcontext

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

type systemEVM struct {
	vmConfig    vm.Config
	vmContext   vm.Context
	chainConfig *params.ChainConfig
	state       vm.StateDB

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
	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	context := New(common.ZeroAddress, common.Big0, header, chain, nil)

	return &systemEVM{
		vmConfig:    *chain.GetVMConfig(),
		vmContext:   context,
		chainConfig: chain.Config(),
		state:       state,
	}
}

func (ev *systemEVM) Execute(sender, recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	evm := vm.NewEVM(ev.vmContext, ev.state, ev.chainConfig, ev.vmConfig)
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	return evm.Call(vm.AccountRef(sender), recipient, input, gas, value)
}

func (ev *systemEVM) Query(sender, recipient common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	evm := vm.NewEVM(ev.vmContext, ev.state, ev.chainConfig, ev.vmConfig)
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
