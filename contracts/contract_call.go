package contracts

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

type ContractMethod struct {
	address common.Address
	method  string
	abi     *abi.ABI
	maxGas  uint64
}

func NewContractMethod(address common.Address, abi *abi.ABI, method string, maxGas uint64) *ContractMethod {
	return &ContractMethod{
		address: address,
		abi:     abi,
		method:  method,
		maxGas:  maxGas,
	}
}

func (cm *ContractMethod) Query(evm *vm.EVM, args []interface{}, result interface{}) (leftoverGas uint64, err error) {
	return call(evm, true, nil, cm.address, cm.maxGas, cm.abi, cm.method, args, result)
}

func (cm *ContractMethod) Write(evm *vm.EVM, args []interface{}, value *big.Int, result interface{}) (leftoverGas uint64, err error) {
	return call(evm, false, value, cm.address, cm.maxGas, cm.abi, cm.method, args, result)
}

func call(
	evm *vm.EVM,
	isQuery bool,
	value *big.Int,
	to common.Address,
	maxGas uint64,
	abi *abi.ABI,
	method string,
	args []interface{},
	result interface{},
) (leftoverGas uint64, err error) {

	defer meterExecutionTime(method)()
	logger := log.New("to", to, "method", method, "args", args, "maxgas", maxGas)

	input, err := abi.Pack(method, args...)
	if err != nil {
		logger.Error("Error invoking evm function: can't encode method arguments", "err", err)
		return 0, err
	}

	var output []byte
	if isQuery {
		output, leftoverGas, err = evm.StaticCall(vm.AccountRef(VMAddress), to, input, maxGas)
	} else {
		output, leftoverGas, err = evm.Call(vm.AccountRef(VMAddress), to, input, maxGas, value)
	}

	if err != nil {
		msg, _ := unpackError(output)
		logger.Error("Error invoking evm function: EVM call failure", "input", hexutil.Encode(input), "err", err, "msg", msg)
		return leftoverGas, err
	}

	if result != nil {
		if err := abi.Unpack(result, method, output); err != nil {
			logger.Error("Error invoking evm function: can't unpack result", "err", err, "gasLeft", leftoverGas)
			return leftoverGas, err
		}
	}

	logger.Trace("EVM call successful", "input", hexutil.Encode(input), "output", hexutil.Encode(output))
	return leftoverGas, nil
}
