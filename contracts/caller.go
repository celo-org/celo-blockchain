package contracts

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/vm"
)

func contractDeployed(evm *vm.EVM, addr common.Address) bool {
	return evm.GetStateDB().GetCodeSize(addr) == 0
}

func startNoGas(evm *vm.EVM) (reactivate func()) {
	evm.DontMeterGas = true
	return func() {
		evm.DontMeterGas = false
	}
}
