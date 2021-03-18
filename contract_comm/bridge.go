package contract_comm

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
)

func MakeStaticCall(registryId [32]byte, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	caller, err := createEVM(header, state)
	if err != nil {
		return 0, err
	}

	return contracts.MakeCallWithContractId(caller, registryId, abi, funcName, args, returnObj, gas, nil, true)
}

func MakeCall(registryId [32]byte, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state vm.StateDB, finaliseState bool) (uint64, error) {
	caller, err := createEVM(header, state)
	if err != nil {
		return 0, err
	}

	gasLeft, err := contracts.MakeCallWithContractId(caller, registryId, abi, funcName, args, returnObj, gas, value, false)
	if err == nil && finaliseState {
		state.Finalise(true)
	}
	return gasLeft, err
}

func MakeStaticCallWithAddress(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state vm.StateDB) (uint64, error) {
	caller, err := createEVM(header, state)
	if err != nil {
		return 0, err
	}

	return contracts.MakeCallFromSystem(caller, scAddress, abi, funcName, args, returnObj, gas, nil, true)
}

func GetRegisteredAddress(registryId [32]byte, header *types.Header, state vm.StateDB) (*common.Address, error) {
	vmevm, err := createEVM(header, state)
	if err != nil {
		return nil, err
	}
	addr, err := contracts.GetRegisteredAddress(vmevm, registryId)
	return &addr, err
}
