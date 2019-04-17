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
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
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

	// selector is first 4 bytes of keccak256 of "balanceOf(address)"
	// Source:
	// pip3 install pyethereum
	// python3 -c 'from ethereum.utils import sha3; print(sha3("balanceOf(address)")[0:4].hex())'
	getBalanceFuncABI = hexutil.MustDecode("0x70a08231")

	errExchangeRateCacheMiss = errors.New("exchange rate cache miss")

	getWhitelistFuncABI, _ = abi.JSON(strings.NewReader(getWhitelistABI))
)

type exchangeRate struct {
	Numerator   *big.Int
	Denominator *big.Int
}

type PriceComparator struct {
	gcWl          *GasCurrencyWhitelist            // Object to retrieve the set of currencies that will have their exchange rate monitored
	exchangeRates map[common.Address]*exchangeRate // indexedCurrency:CeloGold exchange rate
	preAdd        *PredeployedAddresses
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
	log.Trace("PriceComparator.retrieveExchangeRates called", "gasCurrencyAddresses", fmt.Sprintf("%v", gasCurrencyAddresses))

	medianatorAddress := pc.preAdd.GetPredeployedAddress(MedianatorName)

	if medianatorAddress == nil {
		log.Error("Can't get the medianator smart contract address from the registry")
		return
	}

	celoGoldAddress := pc.preAdd.GetPredeployedAddress(GoldTokenName)

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

		if err := pc.iEvmH.makeCall(*medianatorAddress, getExchangeRateFuncABI, "getExchangeRate", []interface{}{celoGoldAddress, gasCurrencyAddress}, &returnArray); err != nil {
			log.Error("PriceComparator.retrieveExchangeRates - Medianator.getExchangeRate invocation error", "err", err)
			continue
		} else {
			log.Trace("PriceComparator.retrieveExchangeRates - Medianator.getExchangeRate invocation success", "returnArray", fmt.Sprintf("%v", returnArray))

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

func NewPriceComparator(gcWl *GasCurrencyWhitelist, preAdd *PredeployedAddresses, iEvmH *InternalEVMHandler) *PriceComparator {
	exchangeRates := make(map[common.Address]*exchangeRate)

	pc := &PriceComparator{
		gcWl:          gcWl,
		exchangeRates: exchangeRates,
		preAdd:        preAdd,
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
func GetBalanceOf(accountOwner common.Address, contractAddress *common.Address, evm *vm.EVM, gas uint64) (
	balance *big.Int, gasUsed uint64, err error) {

	transactionData := common.GetEncodedAbi(getBalanceFuncABI, [][]byte{common.AddressToAbi(accountOwner)})
	anyCaller := vm.AccountRef(common.HexToAddress("0x0")) // any caller will work
	log.Trace("getBalanceOf", "caller", anyCaller, "customTokenContractAddress",
		*contractAddress, "gas", gas, "transactionData", hexutil.Encode(transactionData))
	ret, leftoverGas, err := evm.StaticCall(anyCaller, *contractAddress, transactionData, gas)
	gasUsed = gas - leftoverGas
	if err != nil {
		log.Debug("getBalanceOf error occurred", "Error", err)
		return nil, gasUsed, err
	}
	result := big.NewInt(0)
	result.SetBytes(ret)
	log.Trace("getBalanceOf balance", "account", accountOwner.Hash(), "Balance", result.String(),
		"gas used", gasUsed)
	return result, gasUsed, nil
}

type GasCurrencyWhitelist struct {
	whitelistedAddresses   map[common.Address]bool
	whitelistedAddressesMu sync.RWMutex
	preAdd                 *PredeployedAddresses
	iEvmH                  *InternalEVMHandler
}

func (gcWl *GasCurrencyWhitelist) retrieveWhitelist() []common.Address {
	log.Trace("GasCurrencyWhitelist.retrieveWhitelist called")

	returnList := []common.Address{}

	gasCurrencyWhiteListAddress := gcWl.preAdd.GetPredeployedAddress(GasCurrencyWhitelistName)
	if gasCurrencyWhiteListAddress == nil {
		log.Error("Can't get the gas currency whitelist smart contract address from the registry")
		return returnList
	}

	log.Trace("GasCurrencyWhiteList.retrieveWhiteList() - Calling retrieveWhiteList", "address", gasCurrencyWhiteListAddress.Hex())

	if err := gcWl.iEvmH.makeCall(*gasCurrencyWhiteListAddress, getWhitelistFuncABI, "getWhitelist", []interface{}{}, &returnList); err != nil {
		log.Error("GasCurrencyWhitelist.retrieveWhitelist - GasCurrencyWhitelist.getWhitelist invocation error", "err", err)
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

func NewGasCurrencyWhitelist(preAdd *PredeployedAddresses, iEvmH *InternalEVMHandler) *GasCurrencyWhitelist {
	gcWl := &GasCurrencyWhitelist{
		whitelistedAddresses: make(map[common.Address]bool),
		preAdd:               preAdd,
		iEvmH:                iEvmH,
	}

	return gcWl
}
