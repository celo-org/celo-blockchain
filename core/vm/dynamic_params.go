package vm

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contract_comm/errors"
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

var getAddressForFuncABI, _ = abi.JSON(strings.NewReader(getAddressForABI))

// TODO(kevjue) - Re-Enable caching of the retrieved registered address
// See this commit for the removed code for caching:  https://github.com/celo-org/geth/commit/43a275273c480d307a3d2b3c55ca3b3ee31ec7dd.

type regAddrCacheEntry struct {
	address             *common.Address
	registryStorageHash common.Hash
	registryCodeHash    common.Hash
}

func GetRegisteredAddressWithEvm(registryId string, evm *EVM) (*common.Address, error) {
	evm.DontMeterGas = true
	defer func() { evm.DontMeterGas = false }()

	if evm.GetStateDB().GetCodeSize(params.RegistrySmartContractAddress) == 0 {
		return nil, errors.ErrRegistryContractNotDeployed
	}

	var contractAddress common.Address
	_, err := evm.StaticCallFromSystem(params.RegistrySmartContractAddress, getAddressForFuncABI, "getAddressFor", []interface{}{registryId}, &contractAddress, 20000)

	if err == abi.ErrEmptyOutput {
		log.Trace("Registry contract not deployed")
		return nil, errors.ErrRegistryContractNotDeployed
	} else if err != nil {
		return nil, err
	}

	if contractAddress == common.ZeroAddress {
		return nil, errors.ErrSmartContractNotDeployed
	}

	return &contractAddress, nil
}
