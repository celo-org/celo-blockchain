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
	// TODO(kevjue) - Replace with the actual predeployed address for the registry smart contract
	registrySmartContractAddress   = common.HexToAddress("0x000000000000000000000000000000000000aaaa")
	predeployedContractRegistryIds = []string{params.GoldTokenRegistryId, params.AddressBasedEncryptionRegistryId, params.ReserveRegistryId, params.MedianatorRegistryId, params.GasCurrencyWhitelistRegistryId}
	getAddressForFuncABI, _        = abi.JSON(strings.NewReader(getAddressForABI))
)

type PredeployedAddresses struct {
	predeployedAddresses   map[string]common.Address
	predeployedAddressesMu sync.RWMutex
	iEvmH                  *InternalEVMHandler
}

func (pa *PredeployedAddresses) retrievePredeployedAddresses() map[string]common.Address {
	log.Trace("PredeployedAddresses.retrievePredeployedAddresses called")

	returnMap := make(map[string]common.Address)

	for _, contractRegistryId := range predeployedContractRegistryIds {
		var contractAddress common.Address
		log.Trace("PredeployedAddresses.retrievePredeployedAddresses - Calling Registry.getAddressFor", "contractRegistryId", contractRegistryId)
		if leftoverGas, err := pa.iEvmH.makeCall(registrySmartContractAddress, getAddressForFuncABI, "getAddressFor", []interface{}{contractRegistryId}, &contractAddress, 20000); err != nil {
			log.Error("PredeployedAddresses.retrievePredeployedAddresses - Registry.getAddressFor invocation error", "leftoverGas", leftoverGas, "err", err)
			continue
		} else {
			log.Trace("PredeployedAddresses.retrievePredeployedAddresses - Registry.getAddressFor invocation success", "contractAddress", contractAddress.Hex(), "leftoverGas", leftoverGas)
			returnMap[contractRegistryId] = contractAddress
		}
	}

	return returnMap
}

func (pa *PredeployedAddresses) RefreshAddresses() {
	predeployedAddresses := pa.retrievePredeployedAddresses()

	pa.predeployedAddressesMu.Lock()
	pa.predeployedAddresses = predeployedAddresses
	pa.predeployedAddressesMu.Unlock()
}

func (pa *PredeployedAddresses) GetPredeployedAddress(registryId string) *common.Address {
	pa.predeployedAddressesMu.RLock()
	defer pa.predeployedAddressesMu.RUnlock()

	if address, ok := pa.predeployedAddresses[registryId]; !ok {
		return nil
	} else {
		return &address
	}
}

func (pa *PredeployedAddresses) GetPredeployedAddressMap() map[string]*common.Address {
	returnMap := make(map[string]*common.Address)

	pa.predeployedAddressesMu.RLock()
	defer pa.predeployedAddressesMu.RUnlock()

	for _, registryId := range predeployedContractRegistryIds {
		if address, ok := pa.predeployedAddresses[registryId]; !ok {
			returnMap[registryId] = nil
		} else {
			returnMap[registryId] = &address
		}
	}

	return returnMap
}

func NewPredeployedAddresses(iEvmH *InternalEVMHandler) *PredeployedAddresses {
	pa := &PredeployedAddresses{
		predeployedAddresses: make(map[string]common.Address),
		iEvmH:                iEvmH,
	}

	return pa
}
