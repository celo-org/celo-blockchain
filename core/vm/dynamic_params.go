package vm

import (
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contract_comm/errors"
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
func GetRegisteredAddressWithEvm(registryId [32]byte, evm *EVM) (*common.Address, error) {
	evm.DontMeterGas = true
	defer func() { evm.DontMeterGas = false }()

	// TODO(mcortesi) remove registrypoxy deployed at genesis
	if evm.GetStateDB().GetCodeSize(params.RegistrySmartContractAddress) == 0 {
		return nil, errors.ErrRegistryContractNotDeployed
	}

	var contractAddress common.Address
	_, err := evm.StaticCallFromSystem(params.RegistrySmartContractAddress, getAddressForFuncABI, "getAddressFor", []interface{}{registryId}, &contractAddress, params.MaxGasForGetAddressFor)

	// TODO (mcortesi) Remove ErrEmptyArguments check after we change Proxy to fail on unset impl
	// TODO(asa): Why was this change necessary?
	if err == abi.ErrEmptyArguments || err == ErrExecutionReverted {
		return nil, errors.ErrRegistryContractNotDeployed
	} else if err != nil {
		return nil, err
	}

	if contractAddress == common.ZeroAddress {
		return nil, errors.ErrSmartContractNotDeployed
	}

	return &contractAddress, nil
}
