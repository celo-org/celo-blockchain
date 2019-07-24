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

package currency

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/contract_comm/errors"
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
)

type exchangeRate struct {
	Numerator   *big.Int
	Denominator *big.Int
}

func ConvertToGold(val *big.Int, currencyFrom *common.Address) (*big.Int, error) {
	celoGoldAddress, err := contract_comm.GetContractAddress(params.GoldTokenRegistryId, nil, nil)
	if err == errors.ErrSmartContractNotDeployed || err == errors.ErrRegistryContractNotDeployed {
		log.Warn("Registry address lookup failed", "err", err)
		return val, err
	}

	if currencyFrom == celoGoldAddress {
		return val, err
	} else if err != nil {
		log.Error(err.Error())
		return val, err
	}
	return Convert(val, currencyFrom, celoGoldAddress)
}

// NOTE (jarmg 4/24/18): values are rounded down which can cause
// an estimate to be off by 1 (at most)
func Convert(val *big.Int, currencyFrom *common.Address, currencyTo *common.Address) (*big.Int, error) {
	exchangeRateFrom, err1 := getExchangeRate(currencyFrom)
	exchangeRateTo, err2 := getExchangeRate(currencyTo)

	if err1 != nil || err2 != nil {
		log.Error("Convert - Error in retreiving currency exchange rates")
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

func Cmp(val1 *big.Int, currency1 *common.Address, val2 *big.Int, currency2 *common.Address) int {
	if currency1 == currency2 {
		return val1.Cmp(val2)
	}

	exchangeRate1, err1 := getExchangeRate(currency1)
	exchangeRate2, err2 := getExchangeRate(currency2)

	if err1 != nil || err2 != nil {
		currency1Output := "nil"
		if currency1 != nil {
			currency1Output = currency1.Hex()
		}
		currency2Output := "nil"
		if currency2 != nil {
			currency2Output = currency2.Hex()
		}
		log.Warn("Error in retrieving exchange rate.  Will do comparison of two values without exchange rate conversion.", "currency1", currency1Output, "err1", err1, "currency2", currency2Output, "err2", err2)
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

func getExchangeRate(currencyAddress *common.Address) (*exchangeRate, error) {
	var (
		returnArray [2]*big.Int
		leftoverGas uint64
	)

	if currencyAddress == nil {
		return &exchangeRate{cgExchangeRateNum, cgExchangeRateDen}, nil
	} else {
		if leftoverGas, err := contract_comm.MakeStaticCall(params.SortedOraclesRegistryId, medianRateFuncABI, "medianRate", []interface{}{currencyAddress}, &returnArray, 20000, nil, nil); err != nil {
			if err == errors.ErrSmartContractNotDeployed {
				log.Warn("Registry address lookup failed", "err", err)
				return &exchangeRate{big.NewInt(1), big.NewInt(1)}, err
			} else {
				log.Error("medianRate invocation error", "gasCurrencyAddress", currencyAddress.Hex(), "leftoverGas", leftoverGas, "err", err)
				return &exchangeRate{big.NewInt(1), big.NewInt(1)}, err
			}
		}
	}
	log.Trace("medianRate invocation success", "gasCurrencyAddress", currencyAddress, "returnArray", returnArray, "leftoverGas", leftoverGas)
	return &exchangeRate{returnArray[0], returnArray[1]}, nil
}

// This function will retrieve the balance of an ERC20 token.
//
func GetBalanceOf(accountOwner common.Address, contractAddress common.Address, evm *vm.EVM, gas uint64) (result *big.Int, gasUsed uint64, err error) {

	log.Trace("GetBalanceOf() Called", "accountOwner", accountOwner.Hex(), "contractAddress", contractAddress, "gas", gas)

	var leftoverGas uint64

	if evm != nil {
		leftoverGas, err = evm.StaticCallFromSystem(contractAddress, balanceOfFuncABI, "balanceOf", []interface{}{accountOwner}, &result, gas)
	} else {
		leftoverGas, err = contract_comm.MakeStaticCallWithAddress(contractAddress, balanceOfFuncABI, "balanceOf", []interface{}{accountOwner}, &result, gas, nil, nil)
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

// ------------------------------
// GasCurrencyWhiteList Functions
//-------------------------------
func retrieveWhitelist(header *types.Header, state *state.StateDB) ([]common.Address, error) {
	returnList := []common.Address{}
	gasCurrencyWhiteListAddress, err := contract_comm.GetContractAddress(params.GasCurrencyWhitelistRegistryId, nil, nil)
	if err != nil {
		if err == errors.ErrSmartContractNotDeployed {
			log.Warn("Registry address lookup failed", "err", err)
		} else {
			log.Error("Registry address lookup failed", "err", err)
		}
		return returnList, err
	}

	_, err = contract_comm.MakeStaticCallWithAddress(*gasCurrencyWhiteListAddress, getWhitelistFuncABI, "getWhitelist", []interface{}{}, &returnList, 20000, header, state)
	return returnList, err
}

func IsWhitelisted(currencyAddress common.Address, header *types.Header, state *state.StateDB) bool {
	whitelist, err := retrieveWhitelist(header, state)
	if err != nil {
		log.Warn("Failed to get gas currency whitelist", "err", err)
		return true
	}
	return containsCurrency(currencyAddress, whitelist)
}

func containsCurrency(currencyAddr common.Address, currencyList []common.Address) bool {
	for _, addr := range currencyList {
		if addr == currencyAddr {
			return true
		}
	}
	return false
}

func CurrencyWhitelist(header *types.Header, state *state.StateDB) ([]common.Address, error) {
	whitelist, err := retrieveWhitelist(header, state)
	if err != nil {
		log.Warn("Failed to get gas currency whitelist", "err", err)
	}
	return whitelist, err
}
