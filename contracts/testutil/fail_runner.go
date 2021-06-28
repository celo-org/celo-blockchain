package testutil

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
)

// ErrFailingRunner error for FailingVmRunner
var ErrFailingRunner = errors.New("failing VMRunner")

// FailingVmRunner is a VMRunner that always fails with a ErrFailingRunner
type FailingVmRunner struct{}

func (fvm FailingVmRunner) Execute(recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, err error) {
	return nil, ErrFailingRunner
}

func (fvm FailingVmRunner) Query(recipient common.Address, input []byte, gas uint64) (ret []byte, err error) {
	return nil, ErrFailingRunner
}

func (fvm FailingVmRunner) StopGasMetering()  {}
func (fvm FailingVmRunner) StartGasMetering() {}

func (fvm FailingVmRunner) GetStateDB() vm.StateDB {
	return &mockStateDB{
		isContract: func(a common.Address) bool {
			return true
		},
	}
}
