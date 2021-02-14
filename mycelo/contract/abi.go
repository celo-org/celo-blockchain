package contract

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm/runtime"
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
	return DeployEVMBackend(AbiFor(contractName), cfg, code, params...)
}

// CoreContract returns a contractBackend for a core contract
func CoreContract(cfg *runtime.Config, contractName string, address common.Address) *EVMBackend {
	return NewEVMBackend(AbiFor(contractName), cfg, address)
}

// ProxyContract returns a contractBackend for a core contract's proxy
func ProxyContract(cfg *runtime.Config, contractName string, address common.Address) *EVMBackend {
	return NewEVMBackend(AbiFor("Proxy"), cfg, address)
}
