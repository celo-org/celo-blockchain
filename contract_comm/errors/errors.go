package errors

import (
	"errors"
)

var (
	// ErrSmartContractNotDeployed is returned when the RegisteredAddresses mapping does not contain the specified contract
	ErrSmartContractNotDeployed    = errors.New("registered contract not deployed")
	ErrRegistryContractNotDeployed = errors.New("contract registry not deployed")
	ErrNoIevmHSingleton            = errors.New("No IevmHSingleton set for contract communication")
)
