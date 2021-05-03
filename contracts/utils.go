package contracts

import (
	"bytes"
	"errors"
	"time"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	cerrors "github.com/celo-org/celo-blockchain/contract_comm/errors"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
)

// VMAddress is the address the VM uses to make internal calls to contracts
var VMAddress = common.ZeroAddress

var (
	errorSig     = []byte{0x08, 0xc3, 0x79, 0xa0} // Keccak256("Error(string)")[:4]
	abiString, _ = abi.NewType("string", "", nil)
)

// meterExecutionTime tracks contract execution time for a given contract method identifier
func meterExecutionTime(method string) func() {
	// Record a metrics data point about execution time.
	timer := metrics.GetOrRegisterTimer("contract_comm/systemcall/"+method, nil)
	start := time.Now()
	return func() { timer.UpdateSince(start) }
}

func unpackError(result []byte) (string, error) {
	if len(result) < 4 || !bytes.Equal(result[:4], errorSig) {
		return "<tx result not Error(string)>", errors.New("TX result not of type Error(string)")
	}
	vs, err := abi.Arguments{{Type: abiString}}.UnpackValues(result[4:])
	if err != nil {
		return "<invalid tx result>", err
	}
	return vs[0].(string), nil
}

func resolveAddressForCall(caller vm.SystemEVM, registryId common.Hash, method string) (common.Address, error) {
	contractAddress, err := GetRegisteredAddress(caller, registryId)

	if err != nil {
		hexRegistryId := hexutil.Encode(registryId[:])
		if err == cerrors.ErrSmartContractNotDeployed {
			log.Debug("Contract not yet registered", "function", method, "registryId", hexRegistryId)
		} else if err == cerrors.ErrRegistryContractNotDeployed {
			log.Debug("Registry contract not yet deployed", "function", method, "registryId", hexRegistryId)
		} else {
			log.Error("Error in getting registered address", "function", method, "registryId", hexRegistryId, "err", err)
		}
		return common.ZeroAddress, err
	}
	return contractAddress, nil
}

// noopResolver returns a address resolver function that always resolve to the same address
func noopResolver(addr common.Address) func(vm.SystemEVM) (common.Address, error) {
	return func(e vm.SystemEVM) (common.Address, error) { return addr, nil }
}
