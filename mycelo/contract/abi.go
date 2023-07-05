package contract

import (
	"fmt"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/vm/runtime"
)

func mustParseABI(abiStr string) *abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(abiStr))
	if err != nil {
		panic(err)
	}
	return &parsed
}

// AbiFor returns the ABI for one of the core contracts
func AbiFor(name string) *abi.ABI {
	abi, ok := abis[name]
	if !ok {
		panic("No ABI for " + name)
	}
	return abi
}

// DeployCoreContract deploys one of celo's core contracts
func DeployCoreContract(cfg *runtime.Config, contractName string, code []byte, params ...interface{}) (*EVMBackend, error) {
	backend, err := DeployEVMBackend(AbiFor(contractName), cfg, code, params...)
	if err != nil {
		return backend, fmt.Errorf("Could not deploy '%s': %w", contractName, err)
	}
	return backend, err
}

// CoreContract returns a contractBackend for a core contract
func CoreContract(cfg *runtime.Config, contractName string, address common.Address) *EVMBackend {
	return NewEVMBackend(AbiFor(contractName), cfg, address)
}

// ProxyContract returns a contractBackend for a core contract's proxy
func ProxyContract(cfg *runtime.Config, contractName string, address common.Address) *EVMBackend {
	return NewEVMBackend(AbiFor("Proxy"), cfg, address)
}
