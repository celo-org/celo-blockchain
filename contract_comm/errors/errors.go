package errors

import (
	"errors"
)

var (
	// ErrSmartContractNotDeployed is returned when the RegisteredAddresses mapping does not contain the specified contract
	ErrSmartContractNotDeployed      = errors.New("Contract not in Registry")
	ErrRegistryContractNotDeployed   = errors.New("Registry not deployed")
	ErrNoInternalEvmHandlerSingleton = errors.New("No internalEvmHandlerSingleton set for contract communication")
	ErrExchangeRateZero              = errors.New("Exchange rate returned from the network is zero")
)
