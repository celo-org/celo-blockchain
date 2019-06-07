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
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
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

	setGasPriceFloorABIString = `[{
      "constant": false,
      "inputs": [
        {
          "name": "_gasPriceFloor",
          "type": "uint256"
        }
      ],
      "name": "setGasPriceFloor",
      "outputs": [],
      "payable": false,
      "stateMutability": "nonpayable",
      "type": "function"
    }]`
)

var (
	getGasPriceFloorABI, _  = abi.JSON(strings.NewReader(getGasPriceABIString))
	gasGasPriceParamsABI, _ = abi.JSON(strings.NewReader(getGasPriceParamsABIString))
	setGasPriceFloorABI, _  = abi.JSON(strings.NewReader(setGasPriceFloorABIString))
)

func GetGasPrice(ctx context.Context, iEvmH core.EvmHandler, regAdd core.AddressRegistry) (*big.Int, error) {
	log.Info("gasprice.GetGasPrice called")

	var gasPrice *big.Int
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if gasPriceOracleAddress == nil {
		return nil, errors.New("no gasprice oracle contract address found")
	}

	_, err := iEvmH.MakeStaticCall(*gasPriceOracleAddress, getGasPriceFloorABI, "getGasPriceFloor", []interface{}{}, &gasPrice, 2000, nil, nil)

	return gasPrice, err
}

func GetGasPriceParams(ctx context.Context, iEvmH core.EvmHandler, regAdd core.AddressRegistry) ([3]*big.Int, error) {
	log.Info("gasprice.GetGasPriceParams called")
	var gasPriceParams [3]*big.Int
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if gasPriceOracleAddress == nil {
		return gasPriceParams, errors.New("no gasprice oracle contract address found")
	}

	_, err := iEvmH.MakeStaticCall(*gasPriceOracleAddress, gasGasPriceParamsABI, "getGasPriceParameters", []interface{}{}, &gasPriceParams, 200000, nil, nil)

	return gasPriceParams, err
}

func SetGasPriceFloor(ctx context.Context, iEvmH core.EvmHandler, regAdd core.AddressRegistry, newGasPriceFloor *big.Int, header *types.Header, state *state.StateDB) (uint64, error) {
	log.Info("gasprice.SetGasPriceFloor called")
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if gasPriceOracleAddress == nil {
		return 0, errors.New("no gasprice oracle contract address found")
	}

	gasLeft, err := iEvmH.MakeCall(*gasPriceOracleAddress, setGasPriceFloorABI, "setGasPriceFloor", []interface{}{newGasPriceFloor}, nil, 1000000000, big.NewInt(0), header, state)

	return gasLeft, err
}
