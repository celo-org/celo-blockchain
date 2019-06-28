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

package core

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

type GasPriceMinimum struct {
	regAdd *RegisteredAddresses
	iEvmH  *InternalEVMHandler
}

func (gpm *GasPriceMinimum) GetGasPriceSuggestion(currency *common.Address, state *state.StateDB, header *types.Header) (*big.Int, error) {
	gasPriceMinimum, err := gpm.GetGasPriceMinimum(currency, state, header)
	return new(big.Int).Mul(gasPriceMinimum, suggestionMultiplier), err
}

func (gpm *GasPriceMinimum) GetGasPriceMinimum(currency *common.Address, state *state.StateDB, header *types.Header) (*big.Int, error) {

	if gpm.iEvmH == nil || gpm.regAdd == nil {
		log.Error("gasprice.GetGasPriceMinimum - nil parameters. Returning default gasprice min of 0")
		return FallbackGasPriceMinimum, errors.New("nil iEvmH or addressRegistry")
	}

	var currencyAddress *common.Address
	var err error

	if currency == nil {
		currencyAddress, err = gpm.regAdd.GetRegisteredAddressAtStateAndHeader(params.GoldTokenRegistryId, state, header)

		if err != nil {
			return FallbackGasPriceMinimum, errors.New("no goldtoken contract address found")
		}
	} else {
		currencyAddress = currency
	}

	var gasPriceMinimum *big.Int
	gasPriceMinimumAddress, err := gpm.regAdd.GetRegisteredAddressAtStateAndHeader(params.GasPriceMinimumRegistryId, state, header)

	if err != nil {
		return FallbackGasPriceMinimum, err
	}

	_, err = gpm.iEvmH.MakeStaticCall(
		*gasPriceMinimumAddress,
		gasPriceMinimumABI,
		"getGasPriceMinimum",
		[]interface{}{currencyAddress},
		&gasPriceMinimum,
		defaultGasAmount,
		nil,
		nil,
	)
	return gasPriceMinimum, err
}

func (gpm *GasPriceMinimum) UpdateGasPriceMinimum(header *types.Header, state *state.StateDB) (*big.Int, error) {
	gasPriceMinimumAddress, err := gpm.regAdd.GetRegisteredAddressAtStateAndHeader(params.GasPriceMinimumRegistryId, state, header)

	if err != nil {
		return nil, err
	}

	var updatedGasPriceMinimum *big.Int

	_, err = gpm.iEvmH.MakeCall(
		*gasPriceMinimumAddress,
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
	return updatedGasPriceMinimum, err
}

// Returns the fraction of the gasprice min that should be allocated to the infrastructure fund
func (gpm *GasPriceMinimum) GetInfrastructureFraction(state *state.StateDB, header *types.Header) (*InfrastructureFraction, error) {
	infraFraction := [2]*big.Int{big.NewInt(0), big.NewInt(1)} // Give everything to the miner as Fallback

	if gpm.iEvmH == nil || gpm.regAdd == nil {
		return FallbackInfraFraction, errors.New("nil iEvmH or addressRegistry")
	}

	gasPriceMinimumAddress, err := gpm.regAdd.GetRegisteredAddressAtStateAndHeader(params.GasPriceMinimumRegistryId, state, header)
	if err != nil {
		return FallbackInfraFraction, err
	}

	_, err = gpm.iEvmH.MakeStaticCall(
		*gasPriceMinimumAddress,
		gasPriceMinimumABI,
		"infrastructureFraction",
		[]interface{}{},
		&infraFraction,
		200000,
		nil,
		nil,
	)

	return &InfrastructureFraction{infraFraction[0], infraFraction[1]}, err
}

func NewGasPriceMinimum(iEvmH *InternalEVMHandler, regAdd *RegisteredAddresses) *GasPriceMinimum {
	gpm := &GasPriceMinimum{
		iEvmH:  iEvmH,
		regAdd: regAdd,
	}

	return gpm
}
