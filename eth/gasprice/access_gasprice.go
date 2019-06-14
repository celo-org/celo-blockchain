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

// TODO (jarmg 5/22/19): Store ABIs in a central location
const (
	gasPriceOracleABIString = `[
    {
      "constant": true,
      "inputs": [],
      "name": "infrastructureSplit",
      "outputs": [
        {
          "name": "",
          "type": "uint256"
        },
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

var gasPriceOracleABI, _ = abi.JSON(strings.NewReader(gasPriceOracleABIString))
var errNoGasPriceOracle = errors.New("no gasprice oracle contract address found")

type EvmHandler interface {
	MakeCall(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state *state.StateDB) (uint64, error)
}

type StaticEvmHandler interface {
	MakeStaticCall(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state *state.StateDB) (uint64, error)
}

type AddressRegistry interface {
	GetRegisteredAddress(registryId string) *common.Address
}

type InfrastructureFraction struct {
	Numerator   *big.Int
	Denominator *big.Int
}

func GetGasPriceFloor(iEvmH StaticEvmHandler, regAdd AddressRegistry, currencyAddress *common.Address) (*big.Int, error) {
	fallbackGasPriceFloor := big.NewInt(0) // gasprice floor to return if contracts are not found

	if currencyAddress == nil {
		currencyAddress = regAdd.GetRegisteredAddress(params.GoldTokenRegistryId)

		if currencyAddress == nil {
			log.Warn("No gold token contract address found. Returning default gold gasprice floor of 0")
			return fallbackGasPriceFloor, errors.New("no goldtoken contract address found")
		}
	}

	var gasPrice *big.Int
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if gasPriceOracleAddress == nil {
		log.Warn("No gasprice oracle contract address found. Returning default gasprice floor of 0")
		return fallbackGasPriceFloor, errNoGasPriceOracle
	}

	_, err := iEvmH.MakeStaticCall(
		*gasPriceOracleAddress,
		gasPriceOracleABI,
		"getGasPriceFloor",
		[]interface{}{currencyAddress},
		&gasPrice,
		200000,
		nil,
		nil,
	)
	return gasPrice, err
}

func UpdateGasPriceFloor(iEvmH EvmHandler, regAdd AddressRegistry, header *types.Header, state *state.StateDB) (*big.Int, error) {
	log.Trace("gasprice.UpdateGasPriceFloor called")
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if gasPriceOracleAddress == nil {
		return nil, errNoGasPriceOracle
	}

	var updatedGasPriceFloor *big.Int

	_, err := iEvmH.MakeCall(
		*gasPriceOracleAddress,
		gasPriceOracleABI,
		"updateGasPriceFloor",
		[]interface{}{big.NewInt(int64(header.GasUsed)),
			big.NewInt(int64(header.GasLimit))},
		&updatedGasPriceFloor,
		1000000000,
		big.NewInt(0),
		header,
		state,
	)
	return updatedGasPriceFloor, err
}

// This is useful when doing multiple gasprice floors in the same block number
// TODO (jarmg 6/13/19): Deprecate this by caching GetGasPriceFloor by blockhash
func GetGasPriceMapAndGold(iEvmH StaticEvmHandler, regAdd AddressRegistry, gasCurrencyMap map[common.Address]bool) (map[common.Address]*big.Int, *big.Int) {
	gasPriceFloors := make(map[common.Address]*big.Int)
	goldGasPriceFloor, _ := GetGasPriceFloor(iEvmH, regAdd, nil)
	for address, isValidForGas := range gasCurrencyMap {
		if isValidForGas {
			gp, err := GetGasPriceFloor(iEvmH, regAdd, &address)
			if err == nil {
				gasPriceFloors[address] = gp
			}
		}
	}
	return gasPriceFloors, goldGasPriceFloor
}

// Returns the fraction of the gasprice floor that should be allocated to the infrastructure fund
func GetInfrastructureFraction(iEvmH StaticEvmHandler, regAdd AddressRegistry) (*InfrastructureFraction, error) {
	infraFraction := [2]*big.Int{big.NewInt(0), big.NewInt(1)} // Give everything to the miner as fallback
	gasPriceOracleAddress := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	var err error

	if gasPriceOracleAddress != nil {
		_, err = iEvmH.MakeStaticCall(
			*gasPriceOracleAddress,
			gasPriceOracleABI,
			"infrastructureSplit",
			[]interface{}{},
			&infraFraction,
			200000,
			nil,
			nil,
		)
	} else {
		err = errNoGasPriceOracle
	}
	return &InfrastructureFraction{infraFraction[0], infraFraction[1]}, err
}
