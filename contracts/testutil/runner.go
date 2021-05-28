package testutil

import (
	"errors"
	"math/big"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/vm"
)

var (
	ErrUnkownMethod   = errors.New("unknown method")
	ErrUnkownContract = errors.New("unknown contract")
)

// Check we actually implement EVMRunner
var _ vm.EVMRunner = &MockEVMRunner{}

type solidityMethod func(inputs []interface{}) (outputs []interface{}, err error)

type contractMock struct {
	abi     abi.ABI
	methods map[string]solidityMethod
}

func (cm *contractMock) Call(input []byte) (ret []byte, err error) {
	methodId := input[:4]
	method, err := cm.abi.MethodById(methodId)
	if err != nil {
		return nil, err
	}

	methodFn, ok := cm.methods[method.Name]
	if !ok {
		return nil, ErrUnkownMethod
	}

	inputs, err := method.Inputs.UnpackValues(input[4:])
	if err != nil {
		return nil, err
	}

	outputs, err := methodFn(inputs)
	if err != nil {
		return nil, err
	}

	return method.Outputs.PackValues(outputs)
}

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

type mockStateDB struct{ vm.StateDB }

func (msdb *mockStateDB) GetCodeSize(common.Address) int {
	return 100
}
