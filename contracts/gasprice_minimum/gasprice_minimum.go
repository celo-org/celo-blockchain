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
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/internal/n"
	"github.com/celo-org/celo-blockchain/core/vm"
)

// Gas price minimum serves as baseFee(EIP1559) after Espresso HF.

var (
	FallbackGasPriceMinimum *big.Int = big.NewInt(0) // gas price minimum to return if unable to fetch from contract
)

const (
	maxGasForGetGasPriceMinimum        uint64 = 2 * n.Million
	maxGasForGasPriceMinimumFloor      uint64 = 2 * n.Million
	maxGasForUpdateGasPriceMinimum     uint64 = 2 * n.Million
	maxGasForGetUpdatedGasPriceMinimum uint64 = 2 * n.Million
)

var (
	getGasPriceMinimumMethod        = contracts.NewRegisteredContractMethod(config.GasPriceMinimumRegistryId, abis.GasPriceMinimum, "getGasPriceMinimum", maxGasForGetGasPriceMinimum)
	getGasPriceMinimumFloorMethod   = contracts.NewRegisteredContractMethod(config.GasPriceMinimumRegistryId, abis.GasPriceMinimum, "gasPriceMinimumFloor", maxGasForGasPriceMinimumFloor)
	updateGasPriceMinimumMethod     = contracts.NewRegisteredContractMethod(config.GasPriceMinimumRegistryId, abis.GasPriceMinimum, "updateGasPriceMinimum", maxGasForUpdateGasPriceMinimum)
	getUpdatedGasPriceMinimumMethod = contracts.NewRegisteredContractMethod(config.GasPriceMinimumRegistryId, abis.GasPriceMinimum, "getUpdatedGasPriceMinimum", maxGasForGetUpdatedGasPriceMinimum)
)

func GetGasPriceMinimum(vmRunner vm.EVMRunner, currency *common.Address) (*big.Int, error) {
	var currencyAddress common.Address
	var err error

	if currency == nil {
		currencyAddress, err = contracts.GetRegisteredAddress(vmRunner, config.GoldTokenRegistryId)

		if err == contracts.ErrSmartContractNotDeployed || err == contracts.ErrRegistryContractNotDeployed {
			return FallbackGasPriceMinimum, nil
		}
		if err != nil {
			return FallbackGasPriceMinimum, err
		}
	} else {
		currencyAddress = *currency
	}

	var gasPriceMinimum *big.Int
	err = getGasPriceMinimumMethod.Query(vmRunner, &gasPriceMinimum, currencyAddress)

	if err == contracts.ErrSmartContractNotDeployed || err == contracts.ErrRegistryContractNotDeployed {
		return FallbackGasPriceMinimum, nil
	}
	if err != nil {
		return FallbackGasPriceMinimum, err
	}

	return gasPriceMinimum, err
}

// GetRealGasPriceMinimum is similar to GetRealGasPriceMinimum but if there is
// a problem retrieving the gas price minimum it will return the error and a
// nil gas price minimum.
func GetRealGasPriceMinimum(vmRunner vm.EVMRunner, currency *common.Address) (*big.Int, error) {
	var currencyAddress common.Address
	var err error

	if currency == nil {
		currencyAddress, err = contracts.GetRegisteredAddress(vmRunner, config.GoldTokenRegistryId)

		if err != nil {
			return nil, fmt.Errorf("failed to retrieve gold token address: %w", err)
		}
	} else {
		currencyAddress = *currency
	}

	var gasPriceMinimum *big.Int
	err = getGasPriceMinimumMethod.Query(vmRunner, &gasPriceMinimum, currencyAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve gas price minimum for currency %v, error: %w", currencyAddress.String(), err)
	}

	return gasPriceMinimum, nil
}

func GetGasPriceMinimumFloor(vmRunner vm.EVMRunner) (*big.Int, error) {
	var err error

	var gasPriceMinimumFloor *big.Int
	err = getGasPriceMinimumFloorMethod.Query(vmRunner, &gasPriceMinimumFloor)

	if err == contracts.ErrSmartContractNotDeployed || err == contracts.ErrRegistryContractNotDeployed {
		return FallbackGasPriceMinimum, nil
	}
	if err != nil {
		return FallbackGasPriceMinimum, err
	}

	return gasPriceMinimumFloor, err
}

func GetUpdatedGasPriceMinimum(vmRunner vm.EVMRunner, lastUsedGas, gasLimit uint64) (*big.Int, error) {
	var err error

	var updatedGasPriceMinimum *big.Int
	err = getUpdatedGasPriceMinimumMethod.Query(vmRunner, &updatedGasPriceMinimum, big.NewInt(int64(lastUsedGas)), big.NewInt(int64(gasLimit)))

	if err != nil {
		return nil, err
	}

	return updatedGasPriceMinimum, err
}

func UpdateGasPriceMinimum(vmRunner vm.EVMRunner, lastUsedGas uint64) (*big.Int, error) {
	var updatedGasPriceMinimum *big.Int

	// If an error occurs, the default block gas limit will be returned and a log statement will be produced by GetBlockGasLimitOrDefault
	gasLimit := blockchain_parameters.GetBlockGasLimitOrDefault(vmRunner)

	err := updateGasPriceMinimumMethod.Execute(vmRunner, &updatedGasPriceMinimum, common.Big0, big.NewInt(int64(lastUsedGas)), big.NewInt(int64(gasLimit)))

	if err != nil {
		return nil, err
	}
	return updatedGasPriceMinimum, err
}
