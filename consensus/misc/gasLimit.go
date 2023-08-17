package misc

import (
	"fmt"

	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/core/vm"
)

// VerifyGaslimit verifies the header gas limit according increase/decrease
// in relation to the parent gas limit.
func VerifyGaslimit(headerGasLimit uint64, vmRunnerParent vm.EVMRunner) error {
	actualGasLimit := blockchain_parameters.GetBlockGasLimitOrDefault(vmRunnerParent)
	if actualGasLimit != headerGasLimit {
		return fmt.Errorf("invalid gas limit: have %d, want %d", headerGasLimit, actualGasLimit)
	}
	return nil
}
