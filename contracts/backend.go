package contracts

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

type evmBackend struct {
	vmConfig    vm.Config
	vmContext   vm.Context
	chainConfig *params.ChainConfig
	state       vm.StateDB

	dontMeterGas bool
}

func NewEVMBackend(vmContext vm.Context, state vm.StateDB, chainConfig *params.ChainConfig, vmConfig vm.Config) Backend {
	return &evmBackend{
		vmConfig:    vmConfig,
		vmContext:   vmContext,
		chainConfig: chainConfig,
		state:       state,
	}
}

// Call implements Backend.Call
func (ev *evmBackend) Call(caller vm.ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	evm := vm.NewEVM(ev.vmContext, ev.state, ev.chainConfig, ev.vmConfig)
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	return evm.Call(caller, addr, input, gas, value)
}

// StaticCall implements Backend.StaticCall
func (ev *evmBackend) StaticCall(caller vm.ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	evm := vm.NewEVM(ev.vmContext, ev.state, ev.chainConfig, ev.vmConfig)
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	return evm.StaticCall(caller, addr, input, gas)
}

// StopGasMetering implements Backend.StopGasMetering
func (ev *evmBackend) StopGasMetering() {
	ev.dontMeterGas = true
}

// StartGasMetering implements Backend.StartGasMetering
func (ev *evmBackend) StartGasMetering() {
	ev.dontMeterGas = false
}

// GetStateDB implements Backend.GetStateDB
func (ev *evmBackend) GetStateDB() vm.StateDB {
	return ev.state
}
