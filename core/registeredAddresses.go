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
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Registry.json
	getAddressForABI = `[{"constant": true,
                              "inputs": [
                                   {
                                       "name": "identifier",
                                       "type": "string"
                                   }
                              ],
                              "name": "getAddressFor",
                              "outputs": [
                                   {
                                       "name": "",
                                       "type": "address"
                                   }
                              ],
                              "payable": false,
                              "stateMutability": "view",
                              "type": "function"
                             }]`
)

var (
	registrySmartContractAddress = common.HexToAddress("0x000000000000000000000000000000000000ce10")
	registeredContractIds        = []string{
		params.GoldTokenRegistryId,
		params.AddressBasedEncryptionRegistryId,
		params.ReserveRegistryId,
		params.SortedOraclesRegistryId,
		params.GasCurrencyWhitelistRegistryId,
		params.ValidatorsRegistryId,
		params.GasPriceOracleRegistryId,
		params.GovernanceRegistryId,
		params.BondedDepositsRegistryId,
		params.RandomRegistryId,
	}
	getAddressForFuncABI, _ = abi.JSON(strings.NewReader(getAddressForABI))
	zeroAddress             = common.Address{}
)

type RegisteredAddresses struct {
	registeredAddresses   map[string]common.Address
	registeredAddressesMu sync.RWMutex
	iEvmH                 *InternalEVMHandler
}

func (ra *RegisteredAddresses) retrieveRegisteredAddresses() map[string]common.Address {
	log.Trace("RegisteredAddresses.retrieveRegisteredAddresses called")

	returnMap := make(map[string]common.Address)

	for _, contractRegistryId := range registeredContractIds {
		var contractAddress common.Address
		log.Trace("RegisteredAddresses.retrieveRegisteredAddresses - Calling Registry.getAddressFor", "contractRegistryId", contractRegistryId)
		if leftoverGas, err := ra.iEvmH.MakeStaticCall(registrySmartContractAddress, getAddressForFuncABI, "getAddressFor", []interface{}{contractRegistryId}, &contractAddress, 20000, nil, nil); err != nil {
			log.Error("RegisteredAddresses.retrieveRegisteredAddresses - Registry.getAddressFor invocation error", "leftoverGas", leftoverGas, "err", err)
			continue
		} else {
			log.Trace("RegisteredAddresses.retrieveRegisteredAddresses - Registry.getAddressFor invocation success", "contractAddress", contractAddress.Hex(), "leftoverGas", leftoverGas)

			if contractAddress != zeroAddress {
				returnMap[contractRegistryId] = contractAddress
			}
		}
	}

	return returnMap
}

func (ra *RegisteredAddresses) RefreshAddresses() {
	registeredAddresses := ra.retrieveRegisteredAddresses()

	ra.registeredAddressesMu.Lock()
	ra.registeredAddresses = registeredAddresses
	ra.registeredAddressesMu.Unlock()
}

func (ra *RegisteredAddresses) GetRegisteredAddress(registryId string) *common.Address {
	if len(ra.registeredAddresses) == 0 { // This refresh is for a light client that failed to refresh (did not have a network connection) during node construction
		ra.RefreshAddresses()
	}

	ra.registeredAddressesMu.RLock()
	defer ra.registeredAddressesMu.RUnlock()

	if address, ok := ra.registeredAddresses[registryId]; !ok {
		log.Error("RegisteredAddresses.GetRegisteredAddress - Error in address retrieval for ", "registry", registryId)
		return nil
	} else {
		return &address
	}
}

func (ra *RegisteredAddresses) GetRegisteredAddressMap() map[string]*common.Address {
	returnMap := make(map[string]*common.Address)

	ra.registeredAddressesMu.RLock()
	defer ra.registeredAddressesMu.RUnlock()

	for _, registryId := range registeredContractIds {
		if address, ok := ra.registeredAddresses[registryId]; !ok {
			returnMap[registryId] = nil
		} else {
			returnMap[registryId] = &address
		}
	}

	return returnMap
}

func NewRegisteredAddresses(iEvmH *InternalEVMHandler) *RegisteredAddresses {
	ra := &RegisteredAddresses{
		registeredAddresses: make(map[string]common.Address),
		iEvmH:               iEvmH,
	}

	return ra
}
