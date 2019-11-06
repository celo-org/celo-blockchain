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

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
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
		"name": "intrinsicGasForAlternativeGasCurrency",
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

var blockchainParametersABI abi.ABI

func init() {
	var err error
	blockchainParametersABI, err = abi.JSON(strings.NewReader(blockchainParametersABIString))
	if err != nil {
		log.Crit("Error reading ABI for BlockchainParameters", "err", err)
	}
}

func GetMinimumVersion(header *types.Header, state vm.StateDB) (*params.VersionInfo, error) {
	version := [3]*big.Int{big.NewInt(0), big.NewInt(0), big.NewInt(0)}
	var err error
	_, err = contract_comm.MakeStaticCall(
		params.BlockchainParametersRegistryId,
		blockchainParametersABI,
		"getMinimumClientVersion",
		[]interface{}{},
		&version,
		params.MaxGasForReadBlockchainParameter,
		header,
		state,
	)
	if err != nil {
		return nil, err
	}
	return &params.VersionInfo{version[0].Uint64(), version[1].Uint64(), version[2].Uint64()}, nil
}

func GetGasCost(header *types.Header, state vm.StateDB, defaultGas uint64, method string) uint64 {
	var gas *big.Int
	var err error
	_, err = contract_comm.MakeStaticCall(
		params.BlockchainParametersRegistryId,
		blockchainParametersABI,
		method,
		[]interface{}{},
		&gas,
		params.MaxGasForReadBlockchainParameter,
		header,
		state,
	)
	if err != nil {
		log.Trace("Default gas", "gas", defaultGas, "method", method)
		return defaultGas
	}
	log.Trace("Reading gas", "gas", gas)
	return gas.Uint64()
}

func GetIntrinsicGasForAlternativeGasCurrency(header *types.Header, state vm.StateDB) uint64 {
	return GetGasCost(header, state, params.IntrinsicGasForAlternativeGasCurrency, "intrinsicGasForAlternativeGasCurrency")
}

func CheckMinimumVersion(header *types.Header, state vm.StateDB) {
	version, err := GetMinimumVersion(header, state)

	if err != nil {
		log.Warn("Error checking client version", "err", err, "contract id", params.BlockchainParametersRegistryId)
		return
	}

	if params.CurrentVersionInfo.Cmp(version) == -1 {
		time.Sleep(10 * time.Second)
		log.Crit("Client version older than required", "current", params.Version, "required", version)
	}

}

func SpawnCheck() {
	go func() {
		for {
			time.Sleep(60 * time.Second)
			CheckMinimumVersion(nil, nil)
		}
	}()
}

func GetBlockGasLimit(header *types.Header, state vm.StateDB) (uint64, error) {
	var gasLimit *big.Int
	_, err := contract_comm.MakeStaticCall(
		params.BlockchainParametersRegistryId,
		blockchainParametersABI,
		"blockGasLimit",
		[]interface{}{},
		&gasLimit,
		params.MaxGasForReadBlockchainParameter,
		header,
		state,
	)
	if err != nil {
		return params.DefaultGasLimit, err
	}
	return gasLimit.Uint64(), err
}
