package vm

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
)

// systemCaller is the caller when the EVM is invoked from the within the blockchain system.
var systemCaller = AccountRef(common.HexToAddress("0x0"))

type EVMCaller struct {
	evm *EVM
}

func NewCaller(evm *EVM) *EVMCaller {
	return &EVMCaller{evm}
}

func (caller *EVMCaller) StaticCall(addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	return caller.evm.StaticCall(systemCaller, addr, input, gas)
}

func (caller *EVMCaller) Call(addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	return caller.evm.Call(systemCaller, addr, input, gas, value)
}

func (caller *EVMCaller) ContractDeployed(addr common.Address) bool {
	return caller.evm.GetStateDB().GetCodeSize(addr) == 0
}
