// Copyright 2021 The Celo Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package contract_comm

import (
	"math/big"
	"time"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	ccerrors "github.com/celo-org/celo-blockchain/contract_comm/errors"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
)

func GetRegisteredAddress(registryId common.Hash, header *types.Header, state vm.StateDB) (*common.Address, error) {
	cc := NewCaller(header, state)
	return cc.GetRegisteredAddress(registryId)
}

// Contract Communicator Implementation
func (c contractCommunicator) MakeStaticCallWithAddress(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error) {
	evm, err := c.builder.createEVM()
	if err != nil {
		return gas, err // TODO(Joshua): 0 or gas for gas left?
	}
	return evm.StaticCallFromSystem(contractAddress, abi, funcName, args, returnObj, gas)
}

func (c contractCommunicator) MakeCallWithAddress(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error) {
	evm, err := c.builder.createEVM()
	if err != nil {
		return gas, err // TODO(Joshua): 0 or gas for gas left?
	}
	return evm.CallFromSystem(contractAddress, abi, funcName, args, returnObj, gas, value)
}

func (c contractCommunicator) GetRegisteredAddress(registryId common.Hash) (*common.Address, error) {
	evm, err := c.builder.createEVM()
	if err != nil {
		return nil, err // TODO(Joshua): 0 or gas for gas left?
	}
	return vm.GetRegisteredAddressWithEvm(registryId, evm)
}

func (c contractCommunicator) MakeStaticCall(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error) {
	scAddress, err := c.getRegisteredAddress(registryId, funcName)
	if err != nil {
		return 0, err // TODO(joshua): should this be gas instead of 0?
	}
	// Record a metrics data point about execution time.
	timer := metrics.GetOrRegisterTimer("contract_comm/systemcall/"+funcName, nil)
	start := time.Now()
	defer timer.UpdateSince(start)

	gasLeft, err := c.MakeStaticCallWithAddress(scAddress, abi, funcName, args, returnObj, gas)
	if err != nil {
		log.Error("Error in executing function on registered contract", "function", funcName, "registryId", hexutil.Encode(registryId[:]), "err", err)
	}
	return gasLeft, err

}

func (c contractCommunicator) MakeCall(registryId common.Hash, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int) (uint64, error) {
	scAddress, err := c.getRegisteredAddress(registryId, funcName)
	if err != nil {
		return 0, err // TODO(joshua): should this be gas instead of 0?
	}
	// Record a metrics data point about execution time.
	timer := metrics.GetOrRegisterTimer("contract_comm/systemcall/"+funcName, nil)
	start := time.Now()
	defer timer.UpdateSince(start)

	gasLeft, err := c.MakeCallWithAddress(scAddress, abi, funcName, args, returnObj, gas, value)
	if err != nil {
		log.Error("Error in executing function on registered contract", "function", funcName, "registryId", hexutil.Encode(registryId[:]), "err", err)
	}
	return gasLeft, err
}

// getRegisteredAddress gets the address from the registry and logs if the lookup fails
func (c contractCommunicator) getRegisteredAddress(registryId common.Hash, funcName string) (common.Address, error) {
	scAddress, err := c.GetRegisteredAddress(registryId)

	if err != nil {
		if err == ccerrors.ErrSmartContractNotDeployed {
			log.Debug("Contract not yet registered", "function", funcName, "registryId", hexutil.Encode(registryId[:]))
			return common.Address{}, err
		} else if err == ccerrors.ErrRegistryContractNotDeployed {
			log.Debug("Registry contract not yet deployed", "function", funcName, "registryId", hexutil.Encode(registryId[:]))
			return common.Address{}, err
		} else {
			log.Error("Error in getting registered address", "function", funcName, "registryId", hexutil.Encode(registryId[:]), "err", err)
			return common.Address{}, err
		}
	}
	return *scAddress, nil

}
