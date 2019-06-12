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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// TODO (jarmg 5/22/18): Store contract function ABIs in a central location
const (
	getGasPriceFloorABIString = `[{
      "constant": true,
      "inputs": [
        {
          "name": "_tokenAddress",
          "type": "address"
        }
      ],
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
    },
   {
    "constant": true,
    "inputs": [
      {
        "name": "_blockGasTotal",
        "type": "uint256"
      },
      {
        "name": "_blockGasLimit",
        "type": "uint256"
      }
    ],
    "name": "calculateGasPriceFloor",
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
      "constant": false,
      "inputs": [
        {
          "name": "_blockGasTotal",
          "type": "uint256"
        },
        {
          "name": "_blockGasLimit",
          "type": "uint256"
        }
      ],
      "name": "updateGasPriceFloor",
      "outputs": [
        {
          "name": "",
          "type": "uint256"
        }
      ],
      "payable": false,
      "stateMutability": "nonpayable",
      "type": "function"
    } 
  
  ]`
)

var (
	getGasPriceFloorABI, _  = abi.JSON(strings.NewReader(getGasPriceFloorABIString))
	setGasPriceFloorABI, _  = abi.JSON(strings.NewReader(setGasPriceFloorABIString))
)


func GetGoldGasPrice(ctx context.Context, iEvmH core.EvmHandler, regAdd core.AddressRegistry) (*big.Int, error) {
	log.Info("gasprice.GetGoldGasPrice called")
  goldTokenAddress := regAdd.GetRegisteredAddress(params.GoldTokenRegistryId)

  if goldTokenAddress == nil {
		return nil, errors.New("no gasprice oracle contract address found")
  }

  return getGasPrice(ctx, iEvmH, regAdd, goldTokenAddress)

}


func GetGasPrice(ctx context.Context, iEvmH core.EvmHandler, regAdd core.AddressRegistry, currencyAddress *common.Address) (*big.Int, error) {
	log.Info("gasprice.GetGasPrice called")
  return getGasPrice(ctx, iEvmH, regAdd, currencyAddress)
}


func getGasPrice(ctx context.Context, iEvmH core.EvmHandler, regAdd core.AddressRegistry, currencyAddress *common.Address) (*big.Int, error) {
	log.Info("gasprice.getGasPrice called")

	var gasPrice *big.Int
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if gasPriceOracleAddress == nil {
		return big.NewInt(0), errors.New("no gasprice oracle contract address found")
	}

	_, err := iEvmH.MakeStaticCall(*gasPriceOracleAddress, getGasPriceFloorABI, "getGasPriceFloor", []interface{}{currencyAddress}, &gasPrice, 200000, nil, nil)

	return gasPrice, err
}


func UpdateGasPriceFloor(iEvmH core.EvmHandler, regAdd core.AddressRegistry, header *types.Header, state *state.StateDB) (*big.Int, error) {
	log.Info("gasprice.UpdateGasPriceFloor called")
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	var updatedGasPriceFloor *big.Int

	if gasPriceOracleAddress == nil {
		return nil, errors.New("no gasprice oracle contract address found")
	}

	_, err := iEvmH.MakeCall(*gasPriceOracleAddress, setGasPriceFloorABI, "updateGasPriceFloor", []interface{}{big.NewInt(int64(header.GasUsed)), big.NewInt(int64(header.GasLimit))}, &updatedGasPriceFloor, 1000000000, big.NewInt(0), header, state)

	return updatedGasPriceFloor, err
}
