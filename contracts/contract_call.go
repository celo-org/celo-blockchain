package contracts

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

// Method represents a contract's method
type Method struct {
	abi    *abi.ABI
	method string
	maxGas uint64
}

// NewMethod creates a new contract message
func NewMethod(abi *abi.ABI, method string, maxGas uint64) Method {
	return Method{
		abi:    abi,
		method: method,
		maxGas: maxGas,
	}
}

func (am Method) Bind(contractAddress common.Address) *BoundMethod {
	return &BoundMethod{
		Method:         am,
		resolveAddress: noopResolver(contractAddress),
	}
}

// encodeCall will encodes the msg into []byte format for EVM consumption
func (am Method) encodeCall(args ...interface{}) ([]byte, error) {
	return am.abi.Pack(am.method, args...)
}

// decodeResult will decode the result of msg execution into the result parameter
func (am Method) decodeResult(result interface{}, output []byte) error {
	if result == nil {
		return nil
	}
	return am.abi.Unpack(result, am.method, output)
}

func NewBoundMethod(contractAddress common.Address, abi *abi.ABI, methodName string, maxGas uint64) *BoundMethod {
	return NewMethod(abi, methodName, maxGas).Bind(contractAddress)
}

func NewRegisteredContractMethod(registryId common.Hash, abi *abi.ABI, methodName string, maxGas uint64) *BoundMethod {
	return &BoundMethod{
		Method: NewMethod(abi, methodName, maxGas),
		resolveAddress: func(caller vm.SystemEVM) (common.Address, error) {
			return resolveAddressForCall(caller, registryId, methodName)
		},
	}
}

type BoundMethod struct {
	Method
	resolveAddress func(vm.SystemEVM) (common.Address, error)
}

func (bm *BoundMethod) VMQuery(caller vm.SystemEVM, result interface{}, args ...interface{}) error {
	return bm.Query(caller, result, VMAddress, args...)
}

func (bm *BoundMethod) Query(caller vm.SystemEVM, result interface{}, sender common.Address, args ...interface{}) error {
	return bm.run(caller, result, true, sender, nil, args...)
}

func (bm *BoundMethod) VMExecute(caller vm.SystemEVM, result interface{}, value *big.Int, args ...interface{}) error {
	return bm.run(caller, result, false, VMAddress, value, args...)
}

func (bm *BoundMethod) Execute(caller vm.SystemEVM, result interface{}, sender common.Address, value *big.Int, args ...interface{}) error {
	return bm.run(caller, result, false, sender, value, args...)
}

func (bm *BoundMethod) run(caller vm.SystemEVM, result interface{}, readOnly bool, sender common.Address, value *big.Int, args ...interface{}) error {
	defer meterExecutionTime(bm.method)()

	contractAddress, err := bm.resolveAddress(caller)
	if err != nil {
		return err
	}

	logger := log.New("to", contractAddress, "method", bm.method, "args", args, "maxgas", bm.maxGas)

	input, err := bm.encodeCall(args...)
	if err != nil {
		logger.Error("Error invoking evm function: can't encode method arguments", "err", err)
		return err
	}

	var output []byte
	var leftoverGas uint64
	if readOnly {
		output, leftoverGas, err = caller.Query(sender, contractAddress, input, bm.maxGas)
	} else {
		output, leftoverGas, err = caller.Execute(sender, contractAddress, input, bm.maxGas, value)
	}

	if err != nil {
		message, _ := unpackError(output)
		logger.Error("Error invoking evm function: EVM call failure", "input", hexutil.Encode(input), "err", err, "message", message)
		return err
	}

	if err := bm.decodeResult(result, output); err != nil {
		logger.Error("Error invoking evm function: can't unpack result", "err", err, "gasLeft", leftoverGas)
		return err
	}

	logger.Trace("EVM call successful", "input", hexutil.Encode(input), "output", hexutil.Encode(output))
	return nil
}
