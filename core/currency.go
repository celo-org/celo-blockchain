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
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Medianator.json
	getExchangeRateABI = `[{"constant": true,
                                "inputs": [
                                     {
                                         "name": "base",
                                         "type": "address"
                                     },
                                     {
                                         "name": "counter",
                                         "type": "address"
                                     }
                                ],
                                "name": "getExchangeRate",
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

	getExchangeRateFuncABI, _ = abi.JSON(strings.NewReader(getExchangeRateABI))
	balanceOfFuncABI, _       = abi.JSON(strings.NewReader(balanceOfABI))
	getWhitelistFuncABI, _    = abi.JSON(strings.NewReader(getWhitelistABI))

	errExchangeRateCacheMiss = errors.New("exchange rate cache miss")
)

type exchangeRate struct {
	Numerator   *big.Int
	Denominator *big.Int
}

type PriceComparator struct {
	gcWl          *GasCurrencyWhitelist            // Object to retrieve the set of currencies that will have their exchange rate monitored
	exchangeRates map[common.Address]*exchangeRate // indexedCurrency:CeloGold exchange rate
	regAdd        *RegisteredAddresses
	iEvmH         *InternalEVMHandler
}

func (pc *PriceComparator) getExchangeRate(currency *common.Address) (*big.Int, *big.Int, error) {
	if currency == nil {
		return cgExchangeRateNum, cgExchangeRateDen, nil
	} else {
		if exchangeRate, ok := pc.exchangeRates[*currency]; !ok {
			return nil, nil, errExchangeRateCacheMiss
		} else {
			return exchangeRate.Numerator, exchangeRate.Denominator, nil
		}
	}
}

func (pc *PriceComparator) Cmp(val1 *big.Int, currency1 *common.Address, val2 *big.Int, currency2 *common.Address) int {
	if currency1 == currency2 {
		return val1.Cmp(val2)
	}

	exchangeRate1Num, exchangeRate1Den, err1 := pc.getExchangeRate(currency1)
	exchangeRate2Num, exchangeRate2Den, err2 := pc.getExchangeRate(currency2)

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
	// val1 * exchangeRate1Num/exchangeRate1Den < val2 * exchangeRate2Num/exchangeRate2Den
	// It will transform that comparison to this, to remove having to deal with fractional values.
	// val1 * exchangeRate1Num * exchangeRate2Den < val2 * exchangeRate2Num * exchangeRate1Den
	leftSide := new(big.Int).Mul(val1, new(big.Int).Mul(exchangeRate1Num, exchangeRate2Den))
	rightSide := new(big.Int).Mul(val2, new(big.Int).Mul(exchangeRate2Num, exchangeRate1Den))
	return leftSide.Cmp(rightSide)
}

// This function will retrieve the exchange rates from the Medianator contract and cache them.
// Medianator must have a function with the following signature:
// "function getExchangeRate(address, address)"
func (pc *PriceComparator) retrieveExchangeRates() {
	gasCurrencyAddresses := pc.gcWl.retrieveWhitelist()
	log.Trace("PriceComparator.retrieveExchangeRates called", "gasCurrencyAddresses", gasCurrencyAddresses)

	medianatorAddress := pc.regAdd.GetRegisteredAddress(params.MedianatorRegistryId)

	if medianatorAddress == nil {
		log.Error("Can't get the medianator smart contract address from the registry")
		return
	}

	celoGoldAddress := pc.regAdd.GetRegisteredAddress(params.GoldTokenRegistryId)

	if celoGoldAddress == nil {
		log.Error("Can't get the celo gold smart contract address from the registry")
		return
	}

	for _, gasCurrencyAddress := range gasCurrencyAddresses {
		if gasCurrencyAddress == *celoGoldAddress {
			continue
		}

		var returnArray [2]*big.Int

		log.Trace("PriceComparator.retrieveExchangeRates - Calling getExchangeRate", "medianatorAddress", medianatorAddress.Hex(),
			"gas currency", gasCurrencyAddress.Hex())

		if leftoverGas, err := pc.iEvmH.makeCall(*medianatorAddress, getExchangeRateFuncABI, "getExchangeRate", []interface{}{celoGoldAddress, gasCurrencyAddress}, &returnArray, 20000); err != nil {
			log.Error("PriceComparator.retrieveExchangeRates - Medianator.getExchangeRate invocation error", "leftoverGas", leftoverGas, "err", err)
			continue
		} else {
			log.Trace("PriceComparator.retrieveExchangeRates - Medianator.getExchangeRate invocation success", "returnArray", returnArray, "leftoverGas", leftoverGas)

			if _, ok := pc.exchangeRates[gasCurrencyAddress]; !ok {
				pc.exchangeRates[gasCurrencyAddress] = &exchangeRate{}
			}

			pc.exchangeRates[gasCurrencyAddress].Numerator = returnArray[0]
			pc.exchangeRates[gasCurrencyAddress].Denominator = returnArray[1]
		}
	}
}

func (pc *PriceComparator) mainLoop() {
	pc.retrieveExchangeRates()
	ticker := time.NewTicker(10 * time.Second)

	for range ticker.C {
		pc.retrieveExchangeRates()
	}
}

func NewPriceComparator(gcWl *GasCurrencyWhitelist, regAdd *RegisteredAddresses, iEvmH *InternalEVMHandler) *PriceComparator {
	exchangeRates := make(map[common.Address]*exchangeRate)

	pc := &PriceComparator{
		gcWl:          gcWl,
		exchangeRates: exchangeRates,
		regAdd:        regAdd,
		iEvmH:         iEvmH,
	}

	if pc.gcWl != nil {
		go pc.mainLoop()
	}

	return pc
}

// This function will retrieve the balance of an ERC20 token.  Specifically, the contract must have the
// following function.
// "function balanceOf(address _owner) public view returns (uint256)"
func GetBalanceOf(accountOwner common.Address, contractAddress common.Address, iEvmH *InternalEVMHandler, evm *vm.EVM, gas uint64) (*big.Int, uint64, error) {

	log.Trace("GetBalanceOf() Called", "accountOwner", accountOwner.Hex(), "contractAddress", contractAddress, "gas", gas)

	var result *big.Int
	var leftoverGas uint64
	var err error

	if evm != nil {
		leftoverGas, err = evm.ABIStaticCall(vm.AccountRef(common.HexToAddress("0x0")), contractAddress, balanceOfFuncABI, "balanceOf", []interface{}{accountOwner}, &result, gas)
	} else if iEvmH != nil {
		leftoverGas, err = iEvmH.makeCall(contractAddress, balanceOfFuncABI, "balanceOf", []interface{}{accountOwner}, &result, gas)
	} else {
		return nil, 0, errors.New("Either iEvmH or evm must be non-nil")
	}

	if err != nil {
		log.Error("GetBalanceOf evm invocation error", "leftoverGas", leftoverGas, "err", err)
		return nil, gas - leftoverGas, err
	} else {
		gasUsed := gas - leftoverGas
		log.Trace("GetBalanceOf evm invocation success", "accountOwner", accountOwner.Hex(), "Balance", result.String(), "gas used", gasUsed)
		return result, gasUsed, nil
	}
}

type GasCurrencyWhitelist struct {
	whitelistedAddresses   map[common.Address]bool
	whitelistedAddressesMu sync.RWMutex
	regAdd                 *RegisteredAddresses
	iEvmH                  *InternalEVMHandler
}

func (gcWl *GasCurrencyWhitelist) retrieveWhitelist() []common.Address {
	log.Trace("GasCurrencyWhitelist.retrieveWhitelist called")

	returnList := []common.Address{}

	gasCurrencyWhiteListAddress := gcWl.regAdd.GetRegisteredAddress(params.GasCurrencyWhitelistRegistryId)
	if gasCurrencyWhiteListAddress == nil {
		log.Error("Can't get the gas currency whitelist smart contract address from the registry")
		return returnList
	}

	log.Trace("GasCurrencyWhiteList.retrieveWhiteList() - Calling retrieveWhiteList", "address", gasCurrencyWhiteListAddress.Hex())

	if leftoverGas, err := gcWl.iEvmH.makeCall(*gasCurrencyWhiteListAddress, getWhitelistFuncABI, "getWhitelist", []interface{}{}, &returnList, 20000); err != nil {
		log.Error("GasCurrencyWhitelist.retrieveWhitelist - GasCurrencyWhitelist.getWhitelist invocation error", "leftoverGas", leftoverGas, "err", err)
		return []common.Address{}
	}

	outputWhiteList := make([]string, len(returnList))
	for _, address := range returnList {
		outputWhiteList = append(outputWhiteList, address.Hex())
	}

	log.Trace("GasCurrencyWhitelist.retrieveWhitelist - GasCurrencyWhitelist.getWhitelist invocation success", "whitelisted currencies", outputWhiteList)
	return returnList
}

func (gcWl *GasCurrencyWhitelist) RefreshWhitelist() {
	addresses := gcWl.retrieveWhitelist()

	gcWl.whitelistedAddressesMu.Lock()

	for k := range gcWl.whitelistedAddresses {
		delete(gcWl.whitelistedAddresses, k)
	}

	for _, address := range addresses {
		gcWl.whitelistedAddresses[address] = true
	}

	gcWl.whitelistedAddressesMu.Unlock()
}

func (gcWl *GasCurrencyWhitelist) IsWhitelisted(gasCurrencyAddress common.Address) bool {
	gcWl.whitelistedAddressesMu.RLock()

	_, ok := gcWl.whitelistedAddresses[gasCurrencyAddress]

	gcWl.whitelistedAddressesMu.RUnlock()

	return ok
}

func NewGasCurrencyWhitelist(regAdd *RegisteredAddresses, iEvmH *InternalEVMHandler) *GasCurrencyWhitelist {
	gcWl := &GasCurrencyWhitelist{
		whitelistedAddresses: make(map[common.Address]bool),
		regAdd:               regAdd,
		iEvmH:                iEvmH,
	}

	return gcWl
}
