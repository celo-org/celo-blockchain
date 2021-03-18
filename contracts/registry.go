package contracts

import (
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/errors"
	"github.com/celo-org/celo-blockchain/core/vm"

	"github.com/celo-org/celo-blockchain/params"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Registry.json
	getAddressForABI = `[{"constant": true,
                              "inputs": [
                                   {
                                       "name": "identifier",
                                       "type": "bytes32"
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

func GetRegisteredAddress(evm *vm.EVM, registryId common.Hash) (common.Address, error) {
	defer startNoGas(evm)()

	// TODO(mcortesi) remove registrypoxy deployed at genesis
	if !contractDeployed(evm, params.RegistrySmartContractAddress) {
		return common.ZeroAddress, errors.ErrRegistryContractNotDeployed
	}

	var contractAddress common.Address
	_, err := StaticCallFromSystem(evm, params.RegistrySmartContractAddress, getAddressForFuncABI, "getAddressFor", []interface{}{registryId}, &contractAddress, params.MaxGasForGetAddressFor)

	// TODO (mcortesi) Remove ErrEmptyArguments check after we change Proxy to fail on unset impl
	// TODO(asa): Why was this change necessary?
	if err == abi.ErrEmptyArguments || err == vm.ErrExecutionReverted {
		return common.ZeroAddress, errors.ErrRegistryContractNotDeployed
	} else if err != nil {
		return common.ZeroAddress, err
	}

	if contractAddress == common.ZeroAddress {
		return common.ZeroAddress, errors.ErrSmartContractNotDeployed
	}

	return contractAddress, nil
}
