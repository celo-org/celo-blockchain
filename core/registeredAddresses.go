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
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
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
		params.AttestationsRegistryId,
		params.BondedDepositsRegistryId,
		params.GasCurrencyWhitelistRegistryId,
		params.GasPriceOracleRegistryId,
		params.GoldTokenRegistryId,
		params.GovernanceRegistryId,
		params.RandomRegistryId,
		params.ReserveRegistryId,
		params.SortedOraclesRegistryId,
		params.ValidatorsRegistryId,
	}
	getAddressForFuncABI, _ = abi.JSON(strings.NewReader(getAddressForABI))
)

type RegisteredAddresses struct {
	registeredAddresses   map[string]common.Address
	registeredAddressesMu sync.RWMutex
	iEvmH                 *InternalEVMHandler
}

func (ra *RegisteredAddresses) retrieveRegisteredAddresses(state *state.StateDB, header *types.Header) map[string]common.Address {
	log.Trace("RegisteredAddresses.retrieveRegisteredAddresses called")

	returnMap := make(map[string]common.Address)

	for _, contractRegistryId := range registeredContractIds {
		var contractAddress common.Address
		if leftoverGas, err := ra.iEvmH.MakeStaticCall(registrySmartContractAddress, getAddressForFuncABI, "getAddressFor", []interface{}{contractRegistryId}, &contractAddress, 20000, header, state); err != nil {
			log.Error("Registry.getAddressFor invocation error", "registryId", contractRegistryId, "leftoverGas", leftoverGas, "err", err)
			continue
		} else {
			log.Debug("Registry.getAddressFor invocation success", "registryId", contractRegistryId, "contractAddress", contractAddress.Hex(), "leftoverGas", leftoverGas)

			if contractAddress != common.ZeroAddress {
				returnMap[contractRegistryId] = contractAddress
			}
		}
	}

	return returnMap
}

func (ra *RegisteredAddresses) RefreshAddressesAtStateAndHeader(state *state.StateDB, header *types.Header) {
	ra.refreshAddresses(state, header)
}

func (ra *RegisteredAddresses) RefreshAddresses() {
	ra.refreshAddresses(nil, nil)
}

func (ra *RegisteredAddresses) refreshAddresses(state *state.StateDB, header *types.Header) {
	registeredAddresses := ra.retrieveRegisteredAddresses(state, header)

	ra.registeredAddressesMu.Lock()
	ra.registeredAddresses = registeredAddresses
	ra.registeredAddressesMu.Unlock()
}

func (ra *RegisteredAddresses) GetRegisteredAddress(registryId string) (*common.Address, error) {
	ra.RefreshAddresses()

	ra.registeredAddressesMu.RLock()
	defer ra.registeredAddressesMu.RUnlock()

	if address, ok := ra.registeredAddresses[registryId]; !ok {
		return nil, errors.New("RegisteredAddresses.GetRegisteredAddress - " + registryId + " contract not deployed")
	} else {
		return &address, nil
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
