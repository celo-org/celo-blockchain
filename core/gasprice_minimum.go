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
	"sync"

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
	gasPriceMinimumABI, _   = abi.JSON(strings.NewReader(gasPriceMinimumABIString))
	errNoGasPriceMinimum    = errors.New("no gasprice minimum contract address found")
	gasPriceMinimumCache    = make(map[common.Address]*big.Int)
	cacheHeaderHash         common.Hash
	cacheMu                                         = new(sync.RWMutex)
	FallbackInfraFraction   *InfrastructureFraction = &InfrastructureFraction{big.NewInt(0), big.NewInt(1)}
	FallbackGasPriceMinimum *big.Int                = big.NewInt(0) // gasprice min to return if contracts are not found
)

type EvmHandler interface {
	MakeCall(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state *state.StateDB) (uint64, error)
}

type StaticEvmHandler interface {
	MakeStaticCall(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state *state.StateDB) (uint64, error)
	CurrentHeader() *types.Header
}

type AddressRegistry interface {
	GetRegisteredAddress(registryId string) (*common.Address, error)
}

type InfrastructureFraction struct {
	Numerator   *big.Int
	Denominator *big.Int
}

func GetGasPriceMinimum(iEvmH StaticEvmHandler, regAdd AddressRegistry, currency *common.Address) (*big.Int, error) {

	if iEvmH == nil || regAdd == nil {
		log.Error("gasprice.GetGasPriceMinimum - nil parameters. Returning default gasprice min of 0")
		return FallbackGasPriceMinimum, errors.New("nil iEvmH or addressRegistry")
	}

	var currencyAddress *common.Address
	var err error

	if currency == nil {
		currencyAddress, err = regAdd.GetRegisteredAddress(params.GoldTokenRegistryId)

		if err != nil {
			log.Error("No gold token contract address found. Returning default gold gasprice min of 0")
			return FallbackGasPriceMinimum, errors.New("no goldtoken contract address found")
		}
	} else {
		currencyAddress = currency
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	currentHeaderHash := iEvmH.CurrentHeader().Hash()
	if cacheHeaderHash != currentHeaderHash {
		gasPriceMinimumCache = make(map[common.Address]*big.Int)
		cacheHeaderHash = currentHeaderHash
	}

	var gasPriceMinimum *big.Int
	if gasPriceMinimum, ok := gasPriceMinimumCache[*currencyAddress]; ok {
		return gasPriceMinimum, nil
	}

	gasPriceMinimumAddress, err := regAdd.GetRegisteredAddress(params.GasPriceMinimumRegistryId)

	if err == ErrSmartContractNotDeployed {
		log.Warn("Registry address lookup failed", "err", err)
		return FallbackGasPriceMinimum, err
	} else if err != nil {
		log.Error(err.Error())
		return FallbackGasPriceMinimum, err
	}

	_, err = iEvmH.MakeStaticCall(
		*gasPriceMinimumAddress,
		gasPriceMinimumABI,
		"getGasPriceMinimum",
		[]interface{}{currencyAddress},
		&gasPriceMinimum,
		defaultGasAmount,
		nil,
		nil,
	)
	if err == nil {
		gasPriceMinimumCache[*currencyAddress] = gasPriceMinimum
	}
	return gasPriceMinimum, err
}

func UpdateGasPriceMinimum(iEvmH EvmHandler, regAdd AddressRegistry, header *types.Header, state *state.StateDB) (*big.Int, error) {
	log.Trace("gasprice.UpdateGasPriceMinimum called")
	gasPriceMinimumAddress, err := regAdd.GetRegisteredAddress(params.GasPriceMinimumRegistryId)

	if err == ErrSmartContractNotDeployed {
		log.Warn("Registry address lookup failed", "err", err)
		return nil, err
	} else if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	var updatedGasPriceMinimum *big.Int

	_, err = iEvmH.MakeCall(
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
func GetInfrastructureFraction(iEvmH StaticEvmHandler, regAdd AddressRegistry) (*InfrastructureFraction, error) {
	infraFraction := [2]*big.Int{big.NewInt(0), big.NewInt(1)} // Give everything to the miner as Fallback

	if iEvmH == nil || regAdd == nil {
		log.Error("gasprice.GetGasPriceMinimum - nil parameters. Returning default infra fraction of 0")
		return FallbackInfraFraction, errors.New("nil iEvmH or addressRegistry")
	}

	gasPriceMinimumAddress, err := regAdd.GetRegisteredAddress(params.GasPriceMinimumRegistryId)
	if err == ErrSmartContractNotDeployed {
		log.Warn("Registry address lookup failed", "err", err)
		return FallbackInfraFraction, err
	} else if err != nil {
		log.Error(err.Error())
		return FallbackInfraFraction, err
	}

	_, err = iEvmH.MakeStaticCall(
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
