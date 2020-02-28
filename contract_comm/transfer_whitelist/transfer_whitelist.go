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

package transfer_whitelist

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/contract_comm/errors"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/TransferWhitelist.json
	getWhitelistABI = `[{
      "constant": true,
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
	getWhitelistFuncABI, _ = abi.JSON(strings.NewReader(getWhitelistABI))
)

func retrieveWhitelist(header *types.Header, state vm.StateDB) ([]common.Address, error) {
	var whitelist []common.Address

	if _, err := contract_comm.MakeStaticCall(params.TransferWhitelistRegistryId, getWhitelistFuncABI, "getWhitelist", []interface{}{}, &whitelist, params.MaxGasForGetTransferWhitelist, header, state); err != nil {
		if err == errors.ErrSmartContractNotDeployed {
			log.Warn("Registry address lookup failed", "err", err)
		} else {
			log.Error("getWhitelist invocation on TransferWhitelist failed", "err", err)
		}
		return nil, err
	}

	log.Trace("getWhitelist invocation on TransferWhitelist succeeded")
	return whitelist, nil
}

func IsWhitelisted(to common.Address, from common.Address, header *types.Header, state vm.StateDB) bool {
	whitelist, err := retrieveWhitelist(header, state)
	if err != nil {
		log.Warn("Failed to get transfer whitelist", "err", err)
		return true
	}
	return containsAddress(to, whitelist) || containsAddress(from, whitelist)
}

func containsAddress(target common.Address, whitelist []common.Address) bool {
	for _, addr := range whitelist {
		if addr == target {
			return true
		}
	}
	return false
}

func GetWhitelist(header *types.Header, state vm.StateDB) ([]common.Address, error) {
	whitelist, err := retrieveWhitelist(header, state)
	if err != nil {
		log.Warn("Failed to get transfer whitelist", "err", err)
	}
	return whitelist, err
}
