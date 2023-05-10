package election

import (
	"testing"

	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/testutil"
)

func TestGetElectedValidators(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, GetElectedValidators)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, GetElectedValidators)

}
