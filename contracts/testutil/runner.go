package testutil

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/core/vm"
)

var (
	ErrUnkownMethod   = errors.New("unknown method")
	ErrUnkownContract = errors.New("unknown contract")
)

// Check we actually implement EVMRunner
var _ vm.EVMRunner = &MockEVMRunner{}

type Contract interface {
	Call(input []byte) (ret []byte, err error)
}

type MockEVMRunner struct {
	contracts map[common.Address]Contract
}

func NewMockEVMRunner() *MockEVMRunner {
	return &MockEVMRunner{
		contracts: make(map[common.Address]Contract),
	}
}

func (ev *MockEVMRunner) RegisterContract(address common.Address, mock Contract) {
	ev.contracts[address] = mock
}

func (ev *MockEVMRunner) Execute(recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, err error) {
	mock, ok := ev.contracts[recipient]
	if !ok {
		return nil, ErrUnkownContract
	}

	return mock.Call(input)
}

func (ev *MockEVMRunner) Query(recipient common.Address, input []byte, gas uint64) (ret []byte, err error) {
	mock, ok := ev.contracts[recipient]
	if !ok {
		return nil, ErrUnkownContract
	}

	return mock.Call(input)
}

func (ev *MockEVMRunner) StopGasMetering() {
	// noop
}

func (ev *MockEVMRunner) StartGasMetering() {
	// noop
}

// GetStateDB implements Backend.GetStateDB
func (ev *MockEVMRunner) GetStateDB() vm.StateDB {
	return &mockStateDB{}
}

type ContractMock struct {
	methods []MethodMock
}

func NewContractMock(parsedAbi *abi.ABI, handler interface{}) ContractMock {
	methodMocks := make([]MethodMock, 0)

	handlerType := reflect.TypeOf(handler)
	handlerValue := reflect.ValueOf(handler)
	for i := 0; i < handlerValue.NumMethod(); i++ {
		methodVal := handlerValue.Method(i)
		methodType := handlerType.Method(i)

		if abiMethod, ok := parsedAbi.Methods[decapitalise(methodType.Name)]; ok {
			fmt.Printf("Registering handler for %s\n", abiMethod.Name)
			methodMocks = append(
				methodMocks,
				*NewMethod(&abiMethod, methodVal),
			)
		}

	}

	return ContractMock{methods: methodMocks}
}

func (cm *ContractMock) methodById(id []byte) (*MethodMock, error) {
	for _, method := range cm.methods {
		if bytes.Equal(method.Id(), id[:4]) {
			return &method, nil
		}
	}

	return nil, ErrUnkownMethod
}

func (cm *ContractMock) Call(input []byte) (ret []byte, err error) {
	method, err := cm.methodById(input[:4])
	if err != nil {
		return nil, err
	}

	return method.Call(input)
}

type MethodMock struct {
	method *abi.Method
	fn     reflect.Value
}

func NewMethod(m *abi.Method, fnVal reflect.Value) *MethodMock {
	fnType := fnVal.Type()

	if fnType.Kind() != reflect.Func {
		panic("fn must be a function")
	}

	if fnType.NumIn() != len(m.Inputs) {
		panic(fmt.Sprintf("fn %s() must match number of input arguments [fn: %d, abi: %d]", m.Name, fnType.NumIn(), len(m.Inputs)))
	}
	if !(fnType.NumOut() == len(m.Outputs) || fnType.NumOut() == 1+len(m.Outputs)) {
		panic(fmt.Sprintf("fn %s() must match number of output arguments [fn: %d, abi: %d]", m.Name, fnType.NumOut(), len(m.Outputs)))
	}

	return &MethodMock{
		method: m,
		fn:     fnVal,
	}
}

func (mm *MethodMock) Id() []byte {
	return mm.method.ID
}

func (mm *MethodMock) Call(input []byte) (ret []byte, err error) {
	inputs, err := mm.method.Inputs.UnpackValues(input[4:])
	if err != nil {
		return nil, err
	}

	ins := make([]reflect.Value, len(inputs))
	for i, arg := range inputs {
		ins[i] = reflect.ValueOf(arg)
	}

	outs := mm.fn.Call(ins)
	retValues := make([]interface{}, len(outs))

	// check if we have an error
	if len(outs) == len(mm.method.Outputs)+1 {
		errValue := outs[len(outs)-1]
		if errValue.IsNil() {
			return nil, nil
		}
		return nil, (errValue.Interface()).(error)
	}

	for i, outArg := range outs {
		retValues[i] = outArg.Interface()
	}

	return mm.method.Outputs.PackValues(retValues)
}

func NewSingleMethodContract(registryId common.Hash, methodName string, mockFn interface{}) *ContractMock {
	contractAbi := abis.AbiFor(registryId)
	if contractAbi == nil {
		panic(fmt.Sprintf("no abi for id: %s", registryId.Hex()))
	}

	method, ok := contractAbi.Methods[methodName]
	if !ok {
		panic(fmt.Sprintf("no method named: %s", methodName))
	}

	contract := ContractMock{
		methods: []MethodMock{
			*NewMethod(&method, reflect.ValueOf(mockFn)),
		},
	}
	return &contract
}
