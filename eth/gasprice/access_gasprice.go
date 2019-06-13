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
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// TODO (jarmg 5/22/18): Store contract function ABIs in a central location
const (
	gasPriceOracleABIString = `[{
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

var gasPriceOracleABI, _  = abi.JSON(strings.NewReader(gasPriceOracleABIString))


type EvmHandler interface {
	MakeStaticCall(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state *state.StateDB) (uint64, error)
	MakeCall(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state *state.StateDB) (uint64, error)
}

type StaticEvmHandler interface {
	MakeStaticCall(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state *state.StateDB) (uint64, error)
}

type AddressRegistry interface {
	GetRegisteredAddress(registryId string) *common.Address
}

func GetGoldGasPrice(iEvmH StaticEvmHandler, regAdd AddressRegistry) (*big.Int, error) {
	log.Info("gasprice.GetGoldGasPrice called")
  goldTokenAddress := regAdd.GetRegisteredAddress(params.GoldTokenRegistryId)

  if goldTokenAddress == nil {
    log.Warn("No gasprice oracle contract address found. Returning default gold gasprice floor of 0")
		return big.NewInt(0), errors.New("no gasprice oracle contract address found")
  }

  return getGasPrice(iEvmH, regAdd, goldTokenAddress)
}


func GetGasPrice(iEvmH StaticEvmHandler, regAdd AddressRegistry, currencyAddress *common.Address) (*big.Int, error) {
	log.Info("gasprice.GetGasPrice called")
  return getGasPrice(iEvmH, regAdd, currencyAddress)
}


func getGasPrice(iEvmH StaticEvmHandler, regAdd AddressRegistry, currencyAddress *common.Address) (*big.Int, error) {
	log.Info("gasprice.getGasPrice called")

	var gasPrice *big.Int
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if gasPriceOracleAddress == nil {
    log.Warn("No gasprice oracle contract address found. Returning default gasprice floor of 0")
		return big.NewInt(0), errors.New("no gasprice oracle contract address found")
	}

	_, err := iEvmH.MakeStaticCall(*gasPriceOracleAddress, gasPriceOracleABI, "getGasPriceFloor", []interface{}{currencyAddress}, &gasPrice, 200000, nil, nil)
	return gasPrice, err
}


func UpdateGasPriceFloor(iEvmH EvmHandler, regAdd AddressRegistry, header *types.Header, state *state.StateDB) (*big.Int, error) {
	log.Info("gasprice.UpdateGasPriceFloor called")
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	var updatedGasPriceFloor *big.Int

	if gasPriceOracleAddress == nil {
		return nil, errors.New("no gasprice oracle contract address found")
	}

	_, err := iEvmH.MakeCall(*gasPriceOracleAddress, gasPriceOracleABI, "updateGasPriceFloor", []interface{}{big.NewInt(int64(header.GasUsed)), big.NewInt(int64(header.GasLimit))}, &updatedGasPriceFloor, 1000000000, big.NewInt(0), header, state)
	return updatedGasPriceFloor, err
}


// GetGasPriceMapAndGold returns a map of gasprice floors for all whitelisted currencies and the gold gasprice floor
func GetGasPriceMapAndGold(iEvmH StaticEvmHandler, regAdd AddressRegistry, gasCurrencyMap map[common.Address]bool) (map[common.Address]*big.Int, *big.Int ){
  gasPriceFloors := make(map[common.Address]*big.Int)
  goldGasPriceFloor, _ := GetGoldGasPrice(iEvmH, regAdd)
  for address, isValidForGas := range gasCurrencyMap {
    if isValidForGas {
      gp, err := GetGasPrice(iEvmH, regAdd, &address)
      if err == nil {
        gasPriceFloors[address] = gp
      }
    }
  }
  return gasPriceFloors, goldGasPriceFloor
}
