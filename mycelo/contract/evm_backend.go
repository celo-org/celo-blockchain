package contract

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/ethereum/go-ethereum/log"
)

// EVMBackend represents a contract interface that talks directly to an EVM
type EVMBackend struct {
	abi           *abi.ABI
	runtimeConfig *runtime.Config
	Address       common.Address
}

func DeployEVMBackend(abi *abi.ABI, runtimeConfig *runtime.Config, code []byte, params ...interface{}) (*EVMBackend, error) {
	constructorArgs, err := abi.Pack("", params...)
	if err != nil {
		return nil, err
	}

	_, address, _, err := runtime.Create(append(code, constructorArgs...), runtimeConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating contract: %w", err)
	}

	contract := NewEVMBackend(abi, runtimeConfig, address)
	return contract, nil
}

// NewEVMBackend creates a new EVM based contract
func NewEVMBackend(abi *abi.ABI, runtimeConfig *runtime.Config, receiver common.Address) *EVMBackend {
	return &EVMBackend{
		abi:           abi,
		runtimeConfig: runtimeConfig,
		Address:       receiver,
	}
}

// SimpleCall makes an evm call and just returns the error status
func (ecb *EVMBackend) SimpleCall(method string, args ...interface{}) error {
	_, err := ecb.Call(method, args...)
	return err
}

// Call makes an evm call and returns error and gasLeft
func (ecb *EVMBackend) Call(method string, args ...interface{}) (uint64, error) {
	log.Trace("SmartContract Call", "method", method, "arguments", args)
	calldata, err := ecb.abi.Pack(method, args...)
	if err != nil {
		return 0, err
	}

	ret, gasLeft, err := runtime.Call(ecb.Address, calldata, ecb.runtimeConfig)

	if err != nil {
		// try unpacking the revert (if it is one)
		revertReason, err2 := abi.UnpackRevert(ret)
		if err2 == nil {
			return gasLeft, fmt.Errorf("Revert: %s", revertReason)
		}
	}

	return gasLeft, err
}

// Query makes an evm call, populates the result into returnValue and returns error and gasLeft
func (ecb *EVMBackend) Query(returnValue interface{}, method string, args ...interface{}) (uint64, error) {
	calldata, err := ecb.abi.Pack(method, args...)
	if err != nil {
		return 0, err
	}

	log.Debug("method query", "method", method, "to", ecb.Address, "data", common.Bytes2Hex(calldata))
	ret, gasLeft, err := runtime.Call(ecb.Address, calldata, ecb.runtimeConfig)

	if err != nil {
		// try unpacking the revert (if it is one)
		revertReason, err2 := abi.UnpackRevert(ret)
		if err2 == nil {
			return gasLeft, fmt.Errorf("Revert: %s", revertReason)
		}
	}

	err = ecb.abi.Unpack(returnValue, method, ret)

	return gasLeft, err
}
