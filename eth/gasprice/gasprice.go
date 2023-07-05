// Copyright 2015 The go-ethereum Authors
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

package gasprice

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	cm "github.com/celo-org/celo-blockchain/contracts/currency"
	gpm "github.com/celo-org/celo-blockchain/contracts/gasprice_minimum"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

var (
	suggestionMultiplier *big.Int = big.NewInt(5) // The multiplier that we apply to the minimum when suggesting gas price
)

func GetBaseFeeForCurrency(vmRunner vm.EVMRunner, currencyAddress *common.Address, baseFee *big.Int) (*big.Int, error) {
	if baseFee == nil {
		return gpm.GetGasPriceMinimum(vmRunner, currencyAddress)
	}
	// Gingerbread Fork
	if currencyAddress == nil {
		return baseFee, nil
	} else {
		exchangeRate, err := cm.GetExchangeRate(vmRunner, currencyAddress)
		if err != nil {
			// Assign zero if the exchangeRate fails to mimic the same behaviour as Espresso
			return big.NewInt(0), err
		}
		return exchangeRate.FromBase(baseFee), nil
	}
}

// GetRealBaseFeeForCurrency is similar to GetBaseFeeForCurrency but if there is
// a problem retrieving the gas price minimum it will return the error and a
// nil gas price minimum.
func GetRealBaseFeeForCurrency(vmRunner vm.EVMRunner, currencyAddress *common.Address, baseFee *big.Int) (*big.Int, error) {
	if baseFee == nil {
		return gpm.GetRealGasPriceMinimum(vmRunner, currencyAddress)
	}
	// Gingerbread Fork
	if currencyAddress == nil {
		return baseFee, nil
	} else {
		exchangeRate, err := cm.GetExchangeRate(vmRunner, currencyAddress)
		if err != nil {
			return nil, err
		}
		return exchangeRate.FromBase(baseFee), nil
	}
}

// GetGasPriceSuggestion suggests a gas price the suggestionMultiplier times higher than the GPM in the appropriate currency.
// TODO: Switch to using a caching GPM manager under high load.
func GetGasPriceSuggestion(vmRunner vm.EVMRunner, currencyAddress *common.Address, baseFee *big.Int) (*big.Int, error) {
	gasPriceMinimum, err := GetBaseFeeForCurrency(vmRunner, currencyAddress, baseFee)
	return new(big.Int).Mul(gasPriceMinimum, suggestionMultiplier), err
}

// GetGasTipCapSuggestion suggests a max tip of 2GWei in the appropriate currency.
// TODO: Switch to using a caching currency manager under high load.
func GetGasTipCapSuggestion(vmRunner vm.EVMRunner, currencyAddress *common.Address) (*big.Int, error) {
	celoTipSuggestion := new(big.Int).Mul(common.Big2, big.NewInt(params.GWei))
	exchangeRate, err := cm.GetExchangeRate(vmRunner, currencyAddress)
	if err != nil {
		return nil, err
	}
	return exchangeRate.FromBase(celoTipSuggestion), nil
}
