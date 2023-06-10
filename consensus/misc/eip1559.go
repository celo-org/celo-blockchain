// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package misc

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	gpm "github.com/celo-org/celo-blockchain/contracts/gasprice_minimum"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

// CalcBaseFee calculates the basefee of the header.
func CalcBaseFee(config *params.ChainConfig, parent *types.Header, vmRunner vm.EVMRunner) (*big.Int, error) {
	// If the current block is the first GFork block, return the gasPriceMinimum on the state.
	if !config.IsGFork(parent.Number) {
		return gpm.GetRealGasPriceMinimum(vmRunner, nil)
	}

	// If an error occurs, the default block gas limit will be returned and a log statement will be produced by GetBlockGasLimitOrDefault
	gasLimit, err := blockchain_parameters.GetBlockGasLimit(vmRunner)
	if err != nil {
		return nil, err
	}

	newBaseFee, err := gpm.GetUpdatedGasPriceMinimum(vmRunner, parent.GasUsed, gasLimit)
	if err != nil {
		return nil, err
	}
	return newBaseFee, nil

	// blockDensity := new(big.Int).Div(new(big.Int).SetUint64(parent.GasUsed), new(big.Int).SetUint64(gasLimit))

	// targetDensity, err := gpm.GetTargetDensity(vmRunner)
	// if err != nil {
	// 	return nil, err
	// }
	// adjustmentSpeed, err := gpm.GetAdjustmentSpeed(vmRunner)
	// if err != nil {
	// 	return nil, err
	// }
	// baseFeeFloor, err := gpm.GetGasPriceMinimumFloor(vmRunner)
	// if err != nil {
	// 	return nil, err
	// }

	// var adjustment *big.Int
	// if blockDensity.Cmp(targetDensity) > 0 {
	// 	densityDelta := new(big.Int).Sub(blockDensity, targetDensity)
	// 	adjustment = new(big.Int).Add(common.Big1, new(big.Int).Mul(adjustmentSpeed, densityDelta))
	// } else {
	// 	densityDelta := new(big.Int).Sub(targetDensity, blockDensity)
	// 	adjustment = new(big.Int).Sub(common.Big1, new(big.Int).Mul(adjustmentSpeed, densityDelta))
	// }

	// newGasPriceMinimum := new(big.Int).Add(new(big.Int).Mul(parent.BaseFee, adjustment), common.Big1)

	// if newGasPriceMinimum.Cmp(baseFeeFloor) >= 0 {
	// 	return newGasPriceMinimum, nil
	// }
	// return baseFeeFloor, nil
}
