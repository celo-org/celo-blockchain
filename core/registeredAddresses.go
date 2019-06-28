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

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// ErrSmartContractNotDeployed is returned when the RegisteredAddresses mapping does not contain the specified contract
var ErrSmartContractNotDeployed = errors.New("registered contract not deployed")

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
		params.GasPriceMinimumRegistryId,
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
	iEvmH *InternalEVMHandler
}

func (ra *RegisteredAddresses) getRegisteredAddress(registryId string, state *state.StateDB, header *types.Header) (*common.Address, error) {
	var contractAddress common.Address
	_, err := ra.iEvmH.MakeStaticCallNoRegisteredAddressMap(registrySmartContractAddress, getAddressForFuncABI, "getAddressFor", []interface{}{registryId}, &contractAddress, 20000, header, state)
	if (contractAddress == common.Address{}) {
		return nil, ErrSmartContractNotDeployed
	}
	return &contractAddress, err
}

func (ra *RegisteredAddresses) GetRegisteredAddressAtCurrentHeader(registryId string) (*common.Address, error) {
	if ra == nil {
		return nil, errors.New("Method called on nil interface of type RegisteredAddresses")
	}
	return ra.getRegisteredAddress(registryId, nil, nil)
}

func (ra *RegisteredAddresses) GetRegisteredAddressAtStateAndHeader(registryId string, state *state.StateDB, header *types.Header) (*common.Address, error) {
	if ra == nil {
		return nil, errors.New("Method called on nil interface of type RegisteredAddresses")
	}
	return ra.getRegisteredAddress(registryId, state, header)
}

func (ra *RegisteredAddresses) GetRegisteredAddressMapAtCurrentHeader() map[string]*common.Address {
	returnMap := make(map[string]*common.Address)

	for _, contractRegistryId := range registeredContractIds {
		contractAddress, _ := ra.getRegisteredAddress(contractRegistryId, nil, nil)
		returnMap[contractRegistryId] = contractAddress
	}

	return returnMap
}

func (ra *RegisteredAddresses) GetRegisteredAddressMapAtStateAndHeader(state *state.StateDB, header *types.Header) map[string]*common.Address {
	returnMap := make(map[string]*common.Address)

	for _, contractRegistryId := range registeredContractIds {
		contractAddress, _ := ra.getRegisteredAddress(contractRegistryId, state, header)
		returnMap[contractRegistryId] = contractAddress
	}

	return returnMap
}

func NewRegisteredAddresses(iEvmH *InternalEVMHandler) *RegisteredAddresses {
	ra := &RegisteredAddresses{
		iEvmH: iEvmH,
	}

	return ra
}
