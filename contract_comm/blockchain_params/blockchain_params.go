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

package blockchain_params

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/hashicorp/go-version"
)

const (
	blockchainParamsABIString = `[{
		"constant": true,
		"inputs": [],
		"name": "minimumClientVersion",
		"outputs": [
		  {
			"name": "",
			"type": "string"
		  }
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	  }]`
)

const defaultGasAmount = 2000000

var (
	blockchainParamsABI, _ = abi.JSON(strings.NewReader(blockchainParamsABIString))
)

func CheckMinimumVersion(header *types.Header, state vm.StateDB) error {
	var minVersion string
	var err error
	_, err = contract_comm.MakeStaticCall(
		params.BlockchainParamsRegistryId,
		blockchainParamsABI,
		"minimumClientVersion",
		[]interface{}{},
		&minVersion,
		defaultGasAmount,
		header,
		state,
	)

	if err != nil {
		log.Warn("Error checking client version", "err", err, "contract id", params.BlockchainParamsRegistryId)
		return nil
	}

	log.Warn("Version", "version", minVersion)
	v1, err := version.NewVersion(minVersion)
	v2, err := version.NewVersion(params.Version)

	if v2.LessThan(v1) {
		log.Crit("Client version older than required", "current", params.Version, "required", minVersion)
	}

	return nil
}
