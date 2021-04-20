// Copyright 2017 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package blockchain_parameters

import (
	"math/big"
	"strings"
	"time"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/contract_comm/errors"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
)

const (
	blockchainParametersABIString = `[{
			"constant": true,
			"inputs": [],
			"name": "getMinimumClientVersion",
			"outputs": [
			  {
				"name": "major",
				"type": "uint256"
			  },
			  {
				"name": "minor",
				"type": "uint256"
			  },
			  {
				"name": "patch",
				"type": "uint256"
			  }
			],
			"payable": false,
			"stateMutability": "view",
			"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "blockGasLimit",
		"outputs": [
		  {
			"name": "",
			"type": "uint256"
		  }
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "getUptimeLookbackWindow",
		"outputs": [
			{
				"internalType": "uint256",
				"name": "lookbackWindow",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "intrinsicGasForAlternativeFeeCurrency",
		"outputs": [
			{
				"name": "",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`
)

var (
	getMinimumClientVersionMethod               *contracts.BoundMethod
	intrinsicGasForAlternativeFeeCurrencyMethod *contracts.BoundMethod
	blockGasLimitMethod                         *contracts.BoundMethod
	getUptimeLookbackWindowMethod               *contracts.BoundMethod
)

func init() {
	parsedAbi, err := abi.JSON(strings.NewReader(blockchainParametersABIString))
	if err != nil {
		log.Crit("Error reading ABI for BlockchainParameters", "err", err)
	}

	getMinimumClientVersionMethod = contracts.NewRegisteredContractMethod(params.BlockchainParametersRegistryId, &parsedAbi, "getMinimumClientVersion", params.MaxGasForReadBlockchainParameter)
	intrinsicGasForAlternativeFeeCurrencyMethod = contracts.NewRegisteredContractMethod(params.BlockchainParametersRegistryId, &parsedAbi, "intrinsicGasForAlternativeFeeCurrency", params.MaxGasForReadBlockchainParameter)
	blockGasLimitMethod = contracts.NewRegisteredContractMethod(params.BlockchainParametersRegistryId, &parsedAbi, "blockGasLimit", params.MaxGasForReadBlockchainParameter)
	getUptimeLookbackWindowMethod = contracts.NewRegisteredContractMethod(params.BlockchainParametersRegistryId, &parsedAbi, "getUptimeLookbackWindow", params.MaxGasForReadBlockchainParameter)
}

func GetMinimumVersion(vmRunner vm.EVMRunner) (*params.VersionInfo, error) {
	version := [3]*big.Int{big.NewInt(0), big.NewInt(0), big.NewInt(0)}
	err := getMinimumClientVersionMethod.Query(vmRunner, &version)
	if err != nil {
		return nil, err
	}
	return &params.VersionInfo{
		Major: version[0].Uint64(),
		Minor: version[1].Uint64(),
		Patch: version[2].Uint64(),
	}, nil
}

func GetIntrinsicGasForAlternativeFeeCurrency(vmRunner vm.EVMRunner) uint64 {
	var gas *big.Int
	err := intrinsicGasForAlternativeFeeCurrencyMethod.Query(vmRunner, &gas)

	if err != nil {
		log.Trace("Default gas", "gas", params.IntrinsicGasForAlternativeFeeCurrency, "method", "intrinsicGasForAlternativeFeeCurrency")
		return params.IntrinsicGasForAlternativeFeeCurrency
	}
	log.Trace("Reading gas", "gas", gas)
	return gas.Uint64()
}

func GetBlockGasLimit(vmRunner vm.EVMRunner) (uint64, error) {
	var gasLimit *big.Int
	err := blockGasLimitMethod.Query(vmRunner, &gasLimit)

	if err != nil {
		logError("blockGasLimit", err)
		return params.DefaultGasLimit, err
	}
	return gasLimit.Uint64(), nil
}

func GetLookbackWindow(vmRunner vm.EVMRunner) (uint64, error) {
	var lookbackWindow *big.Int
	err := getUptimeLookbackWindowMethod.Query(vmRunner, &lookbackWindow)

	if err != nil {
		logError("getUptimeLookbackWindow", err)
		return 0, err
	}
	return lookbackWindow.Uint64(), nil
}

func CheckMinimumVersion(vmRunner vm.EVMRunner) {
	version, err := GetMinimumVersion(vmRunner)

	if err != nil {
		logError("getMinimumClientVersion", err)
		return
	}

	if params.CurrentVersionInfo.Cmp(version) == -1 {
		time.Sleep(10 * time.Second)
		log.Crit("Client version older than required", "current", params.Version, "required", version)
	}

}

func logError(method string, err error) {
	if err == errors.ErrRegistryContractNotDeployed {
		log.Debug("Error calling "+method, "err", err, "contract", hexutil.Encode(params.BlockchainParametersRegistryId[:]))
	} else {
		log.Warn("Error calling "+method, "err", err, "contract", hexutil.Encode(params.BlockchainParametersRegistryId[:]))
	}
}

func SpawnCheck(callerFactory func() (vm.EVMRunner, error)) {
	go func() {
		for {
			time.Sleep(60 * time.Second)

			caller, err := callerFactory()
			if err != nil {
				continue
			}
			CheckMinimumVersion(caller)
		}
	}()
}
