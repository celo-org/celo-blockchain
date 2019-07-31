package errors

import (
	"errors"
)

var (
	// ErrSmartContractNotDeployed is returned when the RegisteredAddresses mapping does not contain the specified contract
	ErrSmartContractNotDeployed      = errors.New("registered contract not deployed")
	ErrRegistryContractNotDeployed   = errors.New("contract registry not deployed")
	ErrNoInternalEvmHandlerSingleton = errors.New("No internalEvmHandlerSingleton set for contract communication")
)
