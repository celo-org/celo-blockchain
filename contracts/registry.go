package contracts

import (
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/errors"

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

// TODO(kevjue) - Re-Enable caching of the retrieved registered address
// See this commit for the removed code for caching:  https://github.com/celo-org/geth/commit/43a275273c480d307a3d2b3c55ca3b3ee31ec7dd.
func GetRegisteredAddressWithEvm(registryId [32]byte, evm ContractCaller) (*common.Address, error) {
	return GetRegisteredAddress(evm, registryId)
}

func GetRegisteredAddress(evm ContractCaller, registryId common.Hash) (*common.Address, error) {
	// FIXME(mcortesi)
	// evm.DontMeterGas = true
	// defer func() { evm.DontMeterGas = false }()

	// TODO(mcortesi) remove registrypoxy deployed at genesis
	if !evm.ContractDeployed(params.RegistrySmartContractAddress) {
		return nil, errors.ErrRegistryContractNotDeployed
	}

	var contractAddress common.Address
	_, err := StaticCallFromSystem(evm, params.RegistrySmartContractAddress, getAddressForFuncABI, "getAddressFor", []interface{}{registryId}, &contractAddress, params.MaxGasForGetAddressFor)

	// TODO (mcortesi) Remove ErrEmptyArguments check after we change Proxy to fail on unset impl
	// TODO(asa): Why was this change necessary?
	if err == abi.ErrEmptyArguments {

		// FIXME(mcortesi)
		// if err == abi.ErrEmptyArguments || err == vm.ErrExecutionReverted {
		return nil, errors.ErrRegistryContractNotDeployed
	} else if err != nil {
		return nil, err
	}

	if contractAddress == common.ZeroAddress {
		return nil, errors.ErrSmartContractNotDeployed
	}

	return &contractAddress, nil
}
