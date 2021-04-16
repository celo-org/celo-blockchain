package contracts

import (
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contract_comm/errors"
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

var getAddressForFuncABI abi.ABI

func init() {
	var err error
	getAddressForFuncABI, err = abi.JSON(strings.NewReader(getAddressForABI))
	if err != nil {
		panic("can't parse registry abi " + err.Error())
	}
}

// TODO(kevjue) - Re-Enable caching of the retrieved registered address
// See this commit for the removed code for caching:  https://github.com/celo-org/geth/commit/43a275273c480d307a3d2b3c55ca3b3ee31ec7dd.

// GetRegisteredAddress returns the address on the registry for a given id
func GetRegisteredAddress(evm *vm.EVM, registryId common.Hash) (common.Address, error) {
	// caller.StopGasMetering()
	// defer caller.StartGasMetering()

	evm.StopGasMetering()

	// TODO(mcortesi) remove registrypoxy deployed at genesis
	if evm.GetStateDB().GetCodeSize(params.RegistrySmartContractAddress) == 0 {
		return common.ZeroAddress, errors.ErrRegistryContractNotDeployed
	}

	var contractAddress common.Address
	_, err := QueryCallFromVM(
		params.RegistrySmartContractAddress,
		params.MaxGasForGetAddressFor,
		NewMessage(&getAddressForFuncABI, "getAddressFor", registryId),
	).Run(evm, &contractAddress)

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
