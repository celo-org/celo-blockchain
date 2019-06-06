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

package gasprice

import (
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// TODO (jarmg 5/22/18): Store contract function ABIs in a central location
const (
	getGasPriceABIString = `[{
    "constant": true,
    "inputs": [],
    "name": "getGasPriceFloor",
    "outputs": [
      {
      "name": "",
      "type": "uint256"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
	}]`

	getGasPriceParamsABIString = `[{
		"constant": true,
		"inputs": [],
		"name": "getGasPriceParameters",
		"outputs": [
			{
				"name": "",
				"type": "uint256[3]"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}]`
)

var (
	getGasPriceFloorABI, _  = abi.JSON(strings.NewReader(getGasPriceABIString))
	gasGasPriceParamsABI, _ = abi.JSON(strings.NewReader(getGasPriceParamsABIString))
)

func GetGasPrice(ctx context.Context, iEvmH core.EvmHandler, regAdd core.AddressRegistry) (*big.Int, error) {

	var gasPrice *big.Int
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if gasPriceOracleAddress == nil {
		return nil, errors.New("no gasprice oracle contract address found")
	}

	_, err := iEvmH.MakeCall(*gasPriceOracleAddress, getGasPriceFloorABI, "getGasPriceFloor", []interface{}{}, &gasPrice, 2000, nil, nil)

	return gasPrice, err
}

func GetGasPriceParams(ctx context.Context, iEvmH core.EvmHandler, regAdd core.AddressRegistry) ([3]*big.Int, error) {
	var gasPriceParams [3]*big.Int
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if gasPriceOracleAddress == nil {
		return gasPriceParams, errors.New("no gasprice oracle contract address found")
	}

	_, err := iEvmH.MakeCall(*gasPriceOracleAddress, gasGasPriceParamsABI, "getGasPriceParameters", []interface{}{}, &gasPriceParams, 200000, nil, nil)

	return gasPriceParams, err
}
