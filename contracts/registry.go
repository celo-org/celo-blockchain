package contracts

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts/abis"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

var getAddressMethod = NewBoundMethod(params.RegistrySmartContractAddress, abis.Registry, "getAddressFor", params.MaxGasForGetAddressFor)

// TODO(kevjue) - Re-Enable caching of the retrieved registered address
// See this commit for the removed code for caching:  https://github.com/celo-org/geth/commit/43a275273c480d307a3d2b3c55ca3b3ee31ec7dd.

// GetRegisteredAddress returns the address on the registry for a given id
func GetRegisteredAddress(vmRunner vm.EVMRunner, registryId common.Hash) (common.Address, error) {

	vmRunner.StopGasMetering()
	defer vmRunner.StartGasMetering()

	// TODO(mcortesi) remove registrypoxy deployed at genesis
	if vmRunner.GetStateDB().GetCodeSize(params.RegistrySmartContractAddress) == 0 {
		return common.ZeroAddress, ErrRegistryContractNotDeployed
	}

	var contractAddress common.Address
	err := getAddressMethod.Query(vmRunner, &contractAddress, registryId)

	// TODO (mcortesi) Remove ErrEmptyArguments check after we change Proxy to fail on unset impl
	// TODO(asa): Why was this change necessary?
	if err == abi.ErrEmptyArguments || err == vm.ErrExecutionReverted {
		return common.ZeroAddress, ErrRegistryContractNotDeployed
	} else if err != nil {
		return common.ZeroAddress, err
	}

	if contractAddress == common.ZeroAddress {
		return common.ZeroAddress, ErrSmartContractNotDeployed
	}

	return contractAddress, nil
}
