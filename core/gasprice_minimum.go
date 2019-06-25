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
	gasPriceOracleABIString = `[
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

const defaultGasAmount = 2000000

var (
  gasPriceOracleABI, _ = abi.JSON(strings.NewReader(gasPriceOracleABIString))
  errNoGasPriceOracle = errors.New("no gasprice oracle contract address found")
  gasPriceFloorCache = make(map[common.Address]*big.Int)
  cacheHeaderHash common.Hash
  cacheMu = new(sync.RWMutex)
  FallbackInfraFraction InfrastructureFraction = InfrastructureFraction{big.NewInt(0), big.NewInt(1)}
  FallbackGasPriceFloor *big.Int = big.NewInt(0) // gasprice floor to return if contracts are not found
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


func GetGasPriceFloor(iEvmH StaticEvmHandler, regAdd AddressRegistry, currency *common.Address) (*big.Int, error) {

  if iEvmH == nil || regAdd == nil {
    log.Error("gasprice.GetGasPriceFloor - nil parameters. Returning default gasprice floor of 0")
    return FallbackGasPriceFloor, errors.New("nil iEvmH or addressRegistry")
  }

  var currencyAddress *common.Address
  var err error

	if currency == nil {
    currencyAddress, err = regAdd.GetRegisteredAddress(params.GoldTokenRegistryId)

		if err != nil {
			log.Error("No gold token contract address found. Returning default gold gasprice floor of 0")
			return FallbackGasPriceFloor, errors.New("no goldtoken contract address found")
		}
	} else {
    currencyAddress = currency
  }

  cacheMu.Lock()
  defer cacheMu.Unlock()

  currentHeaderHash := iEvmH.CurrentHeader().Hash()
  if cacheHeaderHash != currentHeaderHash{
    gasPriceFloorCache = make(map[common.Address]*big.Int)
    cacheHeaderHash = currentHeaderHash
  }

  var gasPriceFloor *big.Int
  if gasPriceFloor, ok := gasPriceFloorCache[*currencyAddress]; ok {
    return gasPriceFloor, nil
  }

	gasPriceOracleAddress, err := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if err == ErrSmartContractNotDeployed {
		log.Warn("Registry address lookup failed", "err", err)
    return FallbackGasPriceFloor, err
	} else if err != nil {
		log.Error(err.Error())
    return FallbackGasPriceFloor, err
	}

  _, err = iEvmH.MakeStaticCall(
    *gasPriceOracleAddress,
    gasPriceOracleABI,
    "getGasPriceFloor",
    []interface{}{currencyAddress},
    &gasPriceFloor,
    defaultGasAmount,
    nil,
    nil,
  )
  if err == nil {
    gasPriceFloorCache[*currencyAddress] = gasPriceFloor
  }
	return gasPriceFloor, err
}

func UpdateGasPriceFloor(iEvmH EvmHandler, regAdd AddressRegistry, header *types.Header, state *state.StateDB) (*big.Int, error) {
	log.Trace("gasprice.UpdateGasPriceFloor called")
	gasPriceOracleAddress, err := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)

	if err == ErrSmartContractNotDeployed {
		log.Warn("Registry address lookup failed", "err", err)
		return nil, err
	} else if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	var updatedGasPriceFloor *big.Int

	_, err = iEvmH.MakeCall(
		*gasPriceOracleAddress,
		gasPriceOracleABI,
		"updateGasPriceFloor",
		[]interface{}{big.NewInt(int64(header.GasUsed)),
			big.NewInt(int64(header.GasLimit))},
		&updatedGasPriceFloor,
		defaultGasAmount,
		big.NewInt(0),
		header,
		state,
	)
	return updatedGasPriceFloor, err
}

// Returns the fraction of the gasprice floor that should be allocated to the infrastructure fund
func GetInfrastructureFraction(iEvmH StaticEvmHandler, regAdd AddressRegistry) (*InfrastructureFraction, error) {
	infraFraction := [2]*big.Int{big.NewInt(0), big.NewInt(1)} // Give everything to the miner as Fallback

  if iEvmH == nil || regAdd == nil {
    log.Error("gasprice.GetGasPriceFloor - nil parameters. Returning default infra fraction of 0")
    return &FallbackInfraFraction, errors.New("nil iEvmH or addressRegistry")
  }

	gasPriceOracleAddress, err := regAdd.GetRegisteredAddress(params.GasPriceOracleRegistryId)
	if err == ErrSmartContractNotDeployed {
		log.Warn("Registry address lookup failed", "err", err)
		return &FallbackInfraFraction, err
	} else if err != nil {
		log.Error(err.Error())
		return &FallbackInfraFraction, err
	}

  _, err = iEvmH.MakeStaticCall(
    *gasPriceOracleAddress,
    gasPriceOracleABI,
    "infrastructureFraction",
    []interface{}{},
    &infraFraction,
    200000,
    nil,
    nil,
  )

	return &InfrastructureFraction{infraFraction[0], infraFraction[1]}, err
}
