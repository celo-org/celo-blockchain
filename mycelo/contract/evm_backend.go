package contract

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
	"github.com/ethereum/go-ethereum/log"
)

// EVMBackend represents a contract interface that talks directly to an EVM
type EVMBackend struct {
	abi                   *abi.ABI
	runtimeConfigTemplate *runtime.Config
	Address               common.Address
	defaultCallOpts       CallOpts
}

type CallOpts struct {
	Origin common.Address
	Value  *big.Int
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
		abi:                   abi,
		runtimeConfigTemplate: runtimeConfig,
		Address:               receiver,
		defaultCallOpts: CallOpts{
			Origin: runtimeConfig.Origin,
			Value:  common.Big0,
		},
	}
}

// SimpleCallFrom makes an evm call with given sender address and just returns the error status
func (ecb *EVMBackend) SimpleCallFrom(origin common.Address, method string, args ...interface{}) error {
	_, err := ecb.Call(CallOpts{Origin: origin}, method, args...)
	return err
}

// SimpleCall makes an evm call and just returns the error status
func (ecb *EVMBackend) SimpleCall(method string, args ...interface{}) error {
	_, err := ecb.Call(ecb.defaultCallOpts, method, args...)
	return err
}

// Call makes an evm call and returns error and gasLeft
func (ecb *EVMBackend) Call(opts CallOpts, method string, args ...interface{}) (uint64, error) {
	return ecb.call(opts, method, args...)
}

func (ecb *EVMBackend) call(opts CallOpts, method string, args ...interface{}) (uint64, error) {
	log.Trace("SmartContract Call", "from", opts.Origin, "to", ecb.Address, "method", method, "arguments", args)
	calldata, err := ecb.abi.Pack(method, args...)
	if err != nil {
		return 0, err
	}

	runtimeCfg := *ecb.runtimeConfigTemplate
	runtimeCfg.Origin = opts.Origin
	runtimeCfg.Value = opts.Value

	ret, gasLeft, err := runtime.Call(ecb.Address, calldata, &runtimeCfg)

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

	runtimeCfg := *ecb.runtimeConfigTemplate
	log.Debug("method query", "method", method, "to", ecb.Address, "data", common.Bytes2Hex(calldata))
	ret, gasLeft, err := runtime.Call(ecb.Address, calldata, &runtimeCfg)

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
