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
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/SortedOracles.json
	medianRateABI = `[
    {
      "constant": true,
      "inputs": [
        {
          "name": "token",
          "type": "address"
        }
      ],
      "name": "medianRate",
      "outputs": [
        {
          "name": "",
          "type": "uint128"
        },
        {
          "name": "",
          "type": "uint128"
        }
      ],
      "payable": false,
      "stateMutability": "view",
      "type": "function"
    }]`

	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/ERC20.json
	balanceOfABI = `[{"constant": true,
                          "inputs": [
                               {
                                   "name": "who",
                                   "type": "address"
                               }
                          ],
                          "name": "balanceOf",
                          "outputs": [
                               {
                                   "name": "",
                                   "type": "uint256"
                               }
                          ],
                          "payable": false,
                          "stateMutability": "view",
                          "type": "function"
                         }]`

	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/GasCurrency.json
	getWhitelistABI = `[{"constant": true,
	                     "inputs": [],
	                     "name": "getWhitelist",
	                     "outputs": [
			          {
			              "name": "",
				      "type": "address[]"
				  }
			     ],
			     "payable": false,
			     "stateMutability": "view",
			     "type": "function"
			    }]`
)

var (
	cgExchangeRateNum = big.NewInt(1)
	cgExchangeRateDen = big.NewInt(1)

	medianRateFuncABI, _   = abi.JSON(strings.NewReader(medianRateABI))
	balanceOfFuncABI, _    = abi.JSON(strings.NewReader(balanceOfABI))
	getWhitelistFuncABI, _ = abi.JSON(strings.NewReader(getWhitelistABI))

	errExchangeRateCacheMiss = errors.New("exchange rate cache miss")
)

type exchangeRate struct {
	Numerator   *big.Int
	Denominator *big.Int
}

type CurrencyOperator struct {
	gcWl               *GasCurrencyWhitelist            // Object to retrieve the set of currencies that will have their exchange rate monitored
	exchangeRates      map[common.Address]*exchangeRate // indexedCurrency:CeloGold exchange rate
	regAdd             *RegisteredAddresses
	iEvmH              *InternalEVMHandler
	currencyOperatorMu sync.RWMutex
}

func (co *CurrencyOperator) getExchangeRate(currency *common.Address) (*exchangeRate, error) {
	if currency == nil {
		return &exchangeRate{cgExchangeRateNum, cgExchangeRateDen}, nil
	} else {
		co.currencyOperatorMu.RLock()
		defer co.currencyOperatorMu.RUnlock()
		if exchangeRate, ok := co.exchangeRates[*currency]; !ok {
			return nil, errExchangeRateCacheMiss
		} else {
			return exchangeRate, nil
		}
	}
}

func (co *CurrencyOperator) ConvertToGold(val *big.Int, currencyFrom *common.Address) (*big.Int, error) {
	celoGoldAddress, err := co.regAdd.GetRegisteredAddress(params.GoldTokenRegistryId)
	if err != nil || currencyFrom == celoGoldAddress {
		log.Warn("Registry address lookup failed", "err", err)
		return val, nil
	}
	return co.Convert(val, currencyFrom, celoGoldAddress)
}

// NOTE (jarmg 4/24/18): values are rounded down which can cause
// an estimate to be off by 1 (at most)
func (co *CurrencyOperator) Convert(val *big.Int, currencyFrom *common.Address, currencyTo *common.Address) (*big.Int, error) {
	exchangeRateFrom, err1 := co.getExchangeRate(currencyFrom)
	exchangeRateTo, err2 := co.getExchangeRate(currencyTo)

	if err1 != nil || err2 != nil {
		log.Error("CurrencyOperator.Convert - Error in retreiving currency exchange rates")
		if err1 != nil {
			return nil, err1
		}
		if err2 != nil {
			return nil, err2
		}
	}

	// Given value of val and rates n1/d1 and n2/d2 the function below does
	// (val * n1 * d2) / (d1 * n2)
	numerator := new(big.Int).Mul(val, new(big.Int).Mul(exchangeRateFrom.Numerator, exchangeRateTo.Denominator))
	denominator := new(big.Int).Mul(exchangeRateFrom.Denominator, exchangeRateTo.Numerator)
	return new(big.Int).Div(numerator, denominator), nil
}

func (co *CurrencyOperator) Cmp(val1 *big.Int, currency1 *common.Address, val2 *big.Int, currency2 *common.Address) int {
	if currency1 == currency2 {
		return val1.Cmp(val2)
	}

	exchangeRate1, err1 := co.getExchangeRate(currency1)
	exchangeRate2, err2 := co.getExchangeRate(currency2)

	if err1 != nil || err2 != nil {
		currency1Output := "nil"
		if currency1 != nil {
			currency1Output = currency1.Hex()
		}
		currency2Output := "nil"
		if currency2 != nil {
			currency2Output = currency2.Hex()
		}
		log.Warn("Error in retrieving cached exchange rate.  Will do comparison of two values without exchange rate conversion.", "currency1", currency1Output, "err1", err1, "currency2", currency2Output, "err2", err2)
		return val1.Cmp(val2)
	}

	// Below code block is basically evaluating this comparison:
	// val1 * exchangeRate1.Numerator/exchangeRate1.Denominator < val2 * exchangeRate2.Numerator/exchangeRate2.Denominator
	// It will transform that comparison to this, to remove having to deal with fractional values.
	// val1 * exchangeRate1.Numerator * exchangeRate2.Denominator < val2 * exchangeRate2.Numerator * exchangeRate1.Denominator
	leftSide := new(big.Int).Mul(val1, new(big.Int).Mul(exchangeRate1.Numerator, exchangeRate2.Denominator))
	rightSide := new(big.Int).Mul(val2, new(big.Int).Mul(exchangeRate2.Numerator, exchangeRate1.Denominator))
	return leftSide.Cmp(rightSide)
}

// This function will retrieve the exchange rates from the SortedOracles contract and cache them.
// SortedOracles must have a function with the following signature:
// "function medianRate(address)"
func (co *CurrencyOperator) refreshExchangeRates() {
	gasCurrencyAddresses := co.gcWl.Whitelist()
	sortedOraclesAddress, err := co.regAdd.GetRegisteredAddress(params.SortedOraclesRegistryId)

	if err != nil {
		log.Warn("Registry address lookup failed", "err", err)
		return
	}

	celoGoldAddress, err := co.regAdd.GetRegisteredAddress(params.GoldTokenRegistryId)

	if err != nil {
		log.Warn("Registry address lookup failed", "err", err)
		return
	}

	co.currencyOperatorMu.Lock()

	for _, gasCurrencyAddress := range gasCurrencyAddresses {
		if gasCurrencyAddress == *celoGoldAddress {
			continue
		}

		var returnArray [2]*big.Int
		if leftoverGas, err := co.iEvmH.MakeStaticCall(*sortedOraclesAddress, medianRateFuncABI, "medianRate", []interface{}{gasCurrencyAddress}, &returnArray, 20000, nil, nil); err != nil {
			log.Error("medianRate invocation error", "gasCurrencyAddress", gasCurrencyAddress.Hex(), "leftoverGas", leftoverGas, "err", err)
			continue
		} else {
			log.Trace("medianRate invocation success", "gasCurrencyAddress", gasCurrencyAddress, "returnArray", returnArray, "leftoverGas", leftoverGas)

			if _, ok := co.exchangeRates[gasCurrencyAddress]; !ok {
				co.exchangeRates[gasCurrencyAddress] = &exchangeRate{}
			}

			co.exchangeRates[gasCurrencyAddress].Numerator = returnArray[0]
			co.exchangeRates[gasCurrencyAddress].Denominator = returnArray[1]
		}
	}

	co.currencyOperatorMu.Unlock()
}

// TODO (jarmg 5/30/18): Change this to cache based on block number
func (co *CurrencyOperator) mainLoop() {
	co.refreshExchangeRates()
	ticker := time.NewTicker(10 * time.Second)

	for range ticker.C {
		co.refreshExchangeRates()
	}
}

func NewCurrencyOperator(gcWl *GasCurrencyWhitelist, regAdd *RegisteredAddresses, iEvmH *InternalEVMHandler) *CurrencyOperator {
	exchangeRates := make(map[common.Address]*exchangeRate)

	co := &CurrencyOperator{
		gcWl:          gcWl,
		exchangeRates: exchangeRates,
		regAdd:        regAdd,
		iEvmH:         iEvmH,
	}

	if co.gcWl != nil {
		go co.mainLoop()
	}

	return co
}

// This function will retrieve the balance of an ERC20 token.
//
func GetBalanceOf(accountOwner common.Address, contractAddress common.Address, iEvmH *InternalEVMHandler, evm *vm.EVM, gas uint64) (result *big.Int, gasUsed uint64, err error) {

	log.Trace("GetBalanceOf() Called", "accountOwner", accountOwner.Hex(), "contractAddress", contractAddress, "gas", gas)

	var leftoverGas uint64

	if evm != nil {
		leftoverGas, err = evm.ABIStaticCall(vm.AccountRef(common.HexToAddress("0x0")), contractAddress, balanceOfFuncABI, "balanceOf", []interface{}{accountOwner}, &result, gas)
	} else if iEvmH != nil {
		leftoverGas, err = iEvmH.MakeStaticCall(contractAddress, balanceOfFuncABI, "balanceOf", []interface{}{accountOwner}, &result, gas, nil, nil)
	} else {
		err = errors.New("Either iEvmH or evm must be non-nil")
		return
	}

	if err != nil {
		log.Error("GetBalanceOf evm invocation error", "leftoverGas", leftoverGas, "err", err)
		gasUsed = gas - leftoverGas
		return
	} else {
		gasUsed = gas - leftoverGas
		log.Trace("GetBalanceOf evm invocation success", "accountOwner", accountOwner.Hex(), "Balance", result.String(), "gas used", gasUsed)
		return
	}
}

type GasCurrencyWhitelist struct {
	whitelistedAddresses   map[common.Address]bool
	whitelistedAddressesMu sync.RWMutex
	regAdd                 *RegisteredAddresses
	iEvmH                  *InternalEVMHandler
}

func (gcWl *GasCurrencyWhitelist) retrieveWhitelist(state *state.StateDB, header *types.Header) ([]common.Address, error) {
	returnList := []common.Address{}
	gasCurrencyWhiteListAddress, err := gcWl.regAdd.GetRegisteredAddress(params.GasCurrencyWhitelistRegistryId)
	if err != nil {
		log.Warn("Registry address lookup failed", "err", err)
		return returnList, err
	}

	_, err = gcWl.iEvmH.MakeStaticCall(*gasCurrencyWhiteListAddress, getWhitelistFuncABI, "getWhitelist", []interface{}{}, &returnList, 20000, header, state)
	return returnList, err
}

func (gcWl *GasCurrencyWhitelist) RefreshWhitelistAtStateAndHeader(state *state.StateDB, header *types.Header) {
	gcWl.refreshWhitelist(state, header)
}

func (gcWl *GasCurrencyWhitelist) RefreshWhitelist() {
	gcWl.refreshWhitelist(nil, nil)
}

func (gcWl *GasCurrencyWhitelist) refreshWhitelist(state *state.StateDB, header *types.Header) {
	whitelist, err := gcWl.retrieveWhitelist(state, header)
	if err != nil {
		log.Warn("Failed to get gas currency whitelist", "err", err)
		return
	}

	gcWl.whitelistedAddressesMu.Lock()

	for k := range gcWl.whitelistedAddresses {
		delete(gcWl.whitelistedAddresses, k)
	}

	for _, address := range whitelist {
		gcWl.whitelistedAddresses[address] = true
	}

	gcWl.whitelistedAddressesMu.Unlock()
}

func (gcWl *GasCurrencyWhitelist) IsWhitelisted(gasCurrencyAddress common.Address) bool {
	gcWl.RefreshWhitelist()
	gcWl.whitelistedAddressesMu.RLock()

	_, ok := gcWl.whitelistedAddresses[gasCurrencyAddress]

	gcWl.whitelistedAddressesMu.RUnlock()

	return ok
}

func (gcWl *GasCurrencyWhitelist) Whitelist() []common.Address {
	gcWl.RefreshWhitelist()
	whitelist := make([]common.Address, 0, len(gcWl.whitelistedAddresses))
	for k := range gcWl.whitelistedAddresses {
		whitelist = append(whitelist, k)
	}
	return whitelist
}

func NewGasCurrencyWhitelist(regAdd *RegisteredAddresses, iEvmH *InternalEVMHandler) *GasCurrencyWhitelist {
	gcWl := &GasCurrencyWhitelist{
		whitelistedAddresses: make(map[common.Address]bool),
		regAdd:               regAdd,
		iEvmH:                iEvmH,
	}

	return gcWl
}
