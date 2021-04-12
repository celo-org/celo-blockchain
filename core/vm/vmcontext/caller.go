package vmcontext

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

type evmCaller struct {
	vmConfig    vm.Config
	vmContext   vm.Context
	chainConfig *params.ChainConfig
	state       vm.StateDB

	dontMeterGas bool
}

func NewEVMCallerForCurrentBlock(chain vm.ChainContext) (vm.EVMCaller, error) {
	header := chain.CurrentHeader()
	// FIXME small race condition here (Head changes between get header & get state)
	state, err := chain.State()
	if err != nil {
		return nil, err
	}
	return NewEVMCaller(chain, header, state), nil
}

func NewEVMCaller(chain vm.ChainContext, header *types.Header, state vm.StateDB) vm.EVMCaller {
	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	context := New(common.ZeroAddress, common.Big0, header, chain, nil)

	return &evmCaller{
		vmConfig:    *chain.GetVMConfig(),
		vmContext:   context,
		chainConfig: chain.Config(),
		state:       state,
	}
}

func (ev *evmCaller) Call(caller vm.ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	evm := vm.NewEVM(ev.vmContext, ev.state, ev.chainConfig, ev.vmConfig)
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	return evm.Call(caller, addr, input, gas, value)
}

func (ev *evmCaller) StaticCall(caller vm.ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	evm := vm.NewEVM(ev.vmContext, ev.state, ev.chainConfig, ev.vmConfig)
	if ev.dontMeterGas {
		evm.StopGasMetering()
	}
	return evm.StaticCall(caller, addr, input, gas)
}

func (ev *evmCaller) StopGasMetering() {
	ev.dontMeterGas = true
}

func (ev *evmCaller) StartGasMetering() {
	ev.dontMeterGas = false
}

// GetStateDB implements Backend.GetStateDB
func (ev *evmCaller) GetStateDB() vm.StateDB {
	return ev.state
}
