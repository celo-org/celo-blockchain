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

package blockchain_parameters

import (
	"math/big"
	"time"

	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/contracts/internal/config"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
)

var (
	getMinimumClientVersionMethod               = contracts.NewRegisteredContractMethod(params.BlockchainParametersRegistryId, abis.BlockchainParameters, "getMinimumClientVersion", params.MaxGasForReadBlockchainParameter)
	intrinsicGasForAlternativeFeeCurrencyMethod = contracts.NewRegisteredContractMethod(params.BlockchainParametersRegistryId, abis.BlockchainParameters, "intrinsicGasForAlternativeFeeCurrency", params.MaxGasForReadBlockchainParameter)
	blockGasLimitMethod                         = contracts.NewRegisteredContractMethod(params.BlockchainParametersRegistryId, abis.BlockchainParameters, "blockGasLimit", params.MaxGasForReadBlockchainParameter)
	getUptimeLookbackWindowMethod               = contracts.NewRegisteredContractMethod(params.BlockchainParametersRegistryId, abis.BlockchainParameters, "getUptimeLookbackWindow", params.MaxGasForReadBlockchainParameter)
)

// getMinimumVersion retrieves the client required minimum version
// If a node is running a version smaller than this, it should exit/stop
func getMinimumVersion(vmRunner vm.EVMRunner) (*config.VersionInfo, error) {
	version := [3]*big.Int{big.NewInt(0), big.NewInt(0), big.NewInt(0)}
	err := getMinimumClientVersionMethod.Query(vmRunner, &version)
	if err != nil {
		return nil, err
	}
	return &config.VersionInfo{
		Major: version[0].Uint64(),
		Minor: version[1].Uint64(),
		Patch: version[2].Uint64(),
	}, nil
}

// GetIntrinsicGasForAlternativeFeeCurrencyOrDefault retrieves the intrisic gas for transactions that pay gas in
// with an alternative currency (not CELO).
// In case of error, it returns the default value
func GetIntrinsicGasForAlternativeFeeCurrencyOrDefault(vmRunner vm.EVMRunner) uint64 {
	gas, err := getIntrinsicGasForAlternativeFeeCurrency(vmRunner)
	if err != nil {
		log.Trace("Default gas", "gas", params.IntrinsicGasForAlternativeFeeCurrency, "method", "intrinsicGasForAlternativeFeeCurrency")
		return params.IntrinsicGasForAlternativeFeeCurrency
	}
	log.Trace("Reading gas", "gas", gas)
	return gas
}

// getIntrinsicGasForAlternativeFeeCurrency retrieves the intrisic gas for transactions that pay gas in
// with an alternative currency (not CELO)
func getIntrinsicGasForAlternativeFeeCurrency(vmRunner vm.EVMRunner) (uint64, error) {
	var gas *big.Int
	err := intrinsicGasForAlternativeFeeCurrencyMethod.Query(vmRunner, &gas)

	if err != nil {
		return 0, err
	}

	return gas.Uint64(), nil
}

// GetBlockGasLimitOrDefault retrieves the block max gas limit
// In case of error, it returns the default value
func GetBlockGasLimitOrDefault(vmRunner vm.EVMRunner) uint64 {
	val, err := GetBlockGasLimit(vmRunner)
	if err != nil {
		logError("blockGasLimit", err)
		return params.DefaultGasLimit
	}
	return val
}

// GetBlockGasLimit retrieves the block max gas limit
func GetBlockGasLimit(vmRunner vm.EVMRunner) (uint64, error) {
	var gasLimit *big.Int
	err := blockGasLimitMethod.Query(vmRunner, &gasLimit)
	if err != nil {
		return 0, err
	}
	return gasLimit.Uint64(), nil
}

// GetLookbackWindow retrieves the lookback window parameter to be used
// for uptime score computations
func GetLookbackWindow(vmRunner vm.EVMRunner) (uint64, error) {
	var lookbackWindow *big.Int
	err := getUptimeLookbackWindowMethod.Query(vmRunner, &lookbackWindow)

	if err != nil {
		logError("getUptimeLookbackWindow", err)
		return 0, err
	}
	return lookbackWindow.Uint64(), nil
}

// checkMinimumVersion performs a check on the client's minimum version
// In case of not passing hte check it will exit the node
func checkMinimumVersion(vmRunner vm.EVMRunner) {
	version, err := getMinimumVersion(vmRunner)

	if err != nil {
		logError("getMinimumClientVersion", err)
		return
	}

	if config.CurrentVersionInfo.Cmp(version) == -1 {
		time.Sleep(10 * time.Second)
		// TODO this should exist gracefully, not like this
		log.Crit("Client version older than required", "current", params.Version, "required", version)
	}

}

func logError(method string, err error) {
	if err == contracts.ErrRegistryContractNotDeployed {
		log.Debug("Error calling "+method, "err", err, "contract", hexutil.Encode(params.BlockchainParametersRegistryId[:]))
	} else {
		log.Warn("Error calling "+method, "err", err, "contract", hexutil.Encode(params.BlockchainParametersRegistryId[:]))
	}
}

// SpawnCheck starts a goroutine that will periodically check the client's minimun version
// In case of not passing hte check it will exit the node
func SpawnCheck(runnerFactory func() (vm.EVMRunner, error)) {
	go func() {
		for {
			time.Sleep(60 * time.Second)

			vmRunner, err := runnerFactory()
			if err != nil {
				continue
			}
			checkMinimumVersion(vmRunner)
		}
	}()
}
