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
	"github.com/ethereum/go-ethereum/contract_comm/errors"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// TODO (jarmg 5/22/19): Store ABIs in a central location
const (
	gasPriceMinimumABIString = `[
    {
      "constant": true,
      "inputs": [],
      "name": "proposerFraction_",
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

var (
	gasPriceMinimumABI, _                 = abi.JSON(strings.NewReader(gasPriceMinimumABIString))
	FallbackBurnFraction    *BurnFraction = &BurnFraction{big.NewInt(0), big.NewInt(1)}
	FallbackGasPriceMinimum *big.Int      = big.NewInt(0) // gasprice min to return if contracts are not found
	suggestionMultiplier    *big.Int      = big.NewInt(5) // The multiplier that we apply to the minimum when suggesting gas price
)

type BurnFraction struct {
	Numerator   *big.Int
	Denominator *big.Int
}

func GetGasPriceSuggestion(currency *common.Address, header *types.Header, state vm.StateDB) (*big.Int, error) {
	gasPriceMinimum, err := GetGasPriceMinimum(currency, header, state)
	return new(big.Int).Mul(gasPriceMinimum, suggestionMultiplier), err
}

func GetGasPriceMinimum(currency *common.Address, header *types.Header, state vm.StateDB) (*big.Int, error) {
	var currencyAddress *common.Address
	var err error

	if currency == nil {
		currencyAddress, err = contract_comm.GetRegisteredAddress(params.GoldTokenRegistryId, header, state)

		if err == errors.ErrSmartContractNotDeployed || err == errors.ErrRegistryContractNotDeployed {
			return FallbackGasPriceMinimum, nil
		}
		if err == errors.ErrNoInternalEvmHandlerSingleton {
			log.Error(err.Error())
			return FallbackGasPriceMinimum, nil
		}
		if err != nil {
			log.Error(err.Error())
			return FallbackGasPriceMinimum, err
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
		params.MaxGasForGetGasPriceMinimum,
		header,
		state,
	)

	if err != nil {
		return FallbackGasPriceMinimum, err
	}

	return gasPriceMinimum, err
}

func UpdateGasPriceMinimum(header *types.Header, state vm.StateDB) (*big.Int, error) {
	var updatedGasPriceMinimum *big.Int

	_, err := contract_comm.MakeCall(
		params.GasPriceMinimumRegistryId,
		gasPriceMinimumABI,
		"updateGasPriceMinimum",
		[]interface{}{big.NewInt(int64(header.GasUsed)),
			big.NewInt(int64(header.GasLimit))},
		&updatedGasPriceMinimum,
		params.MaxGasForUpdateGasPriceMinimum,
		big.NewInt(0),
		header,
		state,
	)
	if err != nil {
		return nil, err
	}
	return updatedGasPriceMinimum, err
}

// Returns the fraction of the gasprice min should be burned.
func GetBurnFraction(header *types.Header, state vm.StateDB) (*BurnFraction, error) {
	burnFraction := [2]*big.Int{FallbackBurnFraction.Numerator, FallbackBurnFraction.Denominator} // As a fallback, do not burn gas fees.

	_, err := contract_comm.MakeStaticCall(
		params.GasPriceMinimumRegistryId,
		gasPriceMinimumABI,
		"proposerFraction_",
		[]interface{}{},
		&burnFraction,
		params.MaxGasForBurnFraction,
		header,
		state,
	)

	if err != nil {
		return FallbackBurnFraction, err
	}

	return &BurnFraction{burnFraction[0], burnFraction[1]}, err
}
