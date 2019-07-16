package contract_comm

import (
	"errors"
)

// ErrSmartContractNotDeployed is returned when the RegisteredAddresses mapping does not contain the specified contract
var ErrSmartContractNotDeployed = errors.New("registered contract not deployed")
