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

package gasprice_minimum

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// TODO (jarmg 5/22/19): Store ABIs in a central location
const (
	gasPriceMinimumABIString = `[
    {
      "constant": true,
      "inputs": [],
      "name": "infrastructureFraction",
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
      "constant": true,
      "inputs": [
        {
          "name": "_tokenAddress",
          "type": "address"
        }
      ],
      "name": "getGasPriceMinimum",
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
      "name": "updateGasPriceMinimum",
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

const defaultGasAmount = 2000000

var (
	gasPriceMinimumABI, _                           = abi.JSON(strings.NewReader(gasPriceMinimumABIString))
	FallbackInfraFraction   *InfrastructureFraction = &InfrastructureFraction{big.NewInt(0), big.NewInt(1)}
	FallbackGasPriceMinimum *big.Int                = big.NewInt(0) // gasprice min to return if contracts are not found
	suggestionMultiplier    *big.Int                = big.NewInt(5) // The multiplier that we apply to the minimum when suggesting gas price
)

type InfrastructureFraction struct {
	Numerator   *big.Int
	Denominator *big.Int
}

func GetGasPriceSuggestion(currency *common.Address, header *types.Header, state *state.StateDB) (*big.Int, error) {
	gasPriceMinimum, err := GetGasPriceMinimum(currency, header, state)
	return new(big.Int).Mul(gasPriceMinimum, suggestionMultiplier), err
}

func GetGasPriceMinimum(currency *common.Address, header *types.Header, state *state.StateDB) (*big.Int, error) {
	var currencyAddress *common.Address
	var err error

	if currency == nil {
		currencyAddress, err = contract_comm.GetContractAddress(params.GoldTokenRegistryId, header, state)

		if err != nil {
			return FallbackGasPriceMinimum, nil
		}
	} else {
		currencyAddress = currency
	}

	var gasPriceMinimum *big.Int
	_, err = contract_comm.MakeStaticCall(
		params.GasPriceMinimumRegistryId,
		gasPriceMinimumABI,
		"getGasPriceMinimum",
		[]interface{}{currencyAddress},
		&gasPriceMinimum,
		defaultGasAmount,
		header,
		state,
	)

	if err != nil {
		return FallbackGasPriceMinimum, err
	}

	return gasPriceMinimum, err
}

func UpdateGasPriceMinimum(header *types.Header, state *state.StateDB) (*big.Int, error) {
	var updatedGasPriceMinimum *big.Int

	_, err := contract_comm.MakeCall(
		params.GasPriceMinimumRegistryId,
		gasPriceMinimumABI,
		"updateGasPriceMinimum",
		[]interface{}{big.NewInt(int64(header.GasUsed)),
			big.NewInt(int64(header.GasLimit))},
		&updatedGasPriceMinimum,
		defaultGasAmount,
		big.NewInt(0),
		header,
		state,
	)
	if err != nil {
		return nil, err
	}
	return updatedGasPriceMinimum, err
}

// Returns the fraction of the gasprice min that should be allocated to the infrastructure fund
func GetInfrastructureFraction(header *types.Header, state *state.StateDB) (*InfrastructureFraction, error) {
	infraFraction := [2]*big.Int{big.NewInt(0), big.NewInt(1)} // Give everything to the miner as Fallback

	_, err := contract_comm.MakeStaticCall(
		params.GasPriceMinimumRegistryId,
		gasPriceMinimumABI,
		"infrastructureFraction",
		[]interface{}{},
		&infraFraction,
		200000,
		header,
		state,
	)
	if err != nil {
		return FallbackInfraFraction, err
	}

	return &InfrastructureFraction{infraFraction[0], infraFraction[1]}, err
}
