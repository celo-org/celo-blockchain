package uptime

import (
	"math/big"

	"errors"

	"github.com/celo-org/celo-blockchain/core/types"
)

var (
	// ErrMissingPreviousHeaders is returned when the builder cannot continue due
	// to the missing headers from the epoch.
	ErrMissingPreviousHeaders = errors.New("missing previous headers to compute uptime")

	// ErrHeaderRewinded is returned when the builder cannot continue due to the uptime chain.
	// being rewinded.
	ErrHeaderRewinded = errors.New("header provided is behind current tip in uptime builder")

	// ErrWrongEpoch is returned when a header is provided to a Builder from the wrong epoch.
	ErrWrongEpoch = errors.New("header provided was from the wrong epoch")

	// ErrUnpreparedCompute is returned if ComputeUptime is called without enough preparation for the instance.
	ErrUnpreparedCompute = errors.New("compute uptime is not ready due to missing preparation")
)

type Computer interface {
	// ComputeUptime computes the validators uptime score and returns it as an array.
	// The last header of the epoch must be provided, to ensure that the score is calculated from the
	// correct subchain.
	ComputeUptime(epochLastHeader *types.Header) ([]*big.Int, error)
}

type Builder interface {
	// ProcessHeader adds a header to the Builder. Headers must be provided in order.
	// Some implementations may return ErrHeaderRewinded if a header is given that
	// is behind a previous header provided.
	// Some implementations may return a ErrMissingPreviousHeaders if that specific implementation
	// required additional setup before calling Compute
	ProcessHeader(header *types.Header) error

	// Clear resets this builder
	Clear()

	// GetLastProcessedHeader returns the last processed header by this Builder.
	GetLastProcessedHeader() *types.Header

	// GetEpochSize returns the epoch size for the current epoch in this Builder.
	GetEpochSize() uint64

	// GetEpoch returns the epoch for this uptime Builder.
	GetEpoch() uint64

	Computer // Not 100% sure Builder should include Computer or if they can be completely separated.
}
