package backend

import (
	"sync/atomic"

	"github.com/celo-org/celo-blockchain/common"
)

type ValidatorChecker interface {
	IsElectedOrNearValidator() (bool, error)
	IsValidating() bool
}

type checker struct {
	aWallets                 *atomic.Value
	retrieveValidatorConnSet func() (map[common.Address]bool, error)
	isValidating             func() bool
}

func NewValidatorChecker(
	wallets *atomic.Value,
	retrieveValidatorConnSetFn func() (map[common.Address]bool, error),
	isValidatingFn func() bool) ValidatorChecker {
	return &checker{
		aWallets:                 wallets,
		retrieveValidatorConnSet: retrieveValidatorConnSetFn,
		isValidating:             isValidatingFn,
	}
}

func (c *checker) IsElectedOrNearValidator() (bool, error) {
	// Check if this node is in the validator connection set
	validatorConnSet, err := c.retrieveValidatorConnSet()
	if err != nil {
		return false, err
	}
	w := c.aWallets.Load().(*Wallets)
	return validatorConnSet[w.Ecdsa.Address], nil
}

func (c *checker) IsValidating() bool {
	return c.isValidating()
}
