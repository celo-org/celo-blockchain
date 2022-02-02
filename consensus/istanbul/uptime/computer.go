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

	// ErrHeaderRewinded is returned when the builder cannot continue due to the uptime chain
	// being rewinded.
	ErrHeaderRewinded = errors.New("header provided is behind current tip in uptime builder")

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

	Computer // Not 100% sure Builder should include Computer or if they can be completely separated.
}

type EpochHeadersProvider interface {
	// GetEpochHeadersUpTo returns all headers from the same epoch as the header provided (included) ordered.
	GetEpochHeadersUpTo(upToHeader *types.Header) ([]*types.Header, error)
}

// AutoFixBuilder is an uptime Builder that will fix rewinds and missing headers.
type AutoFixBuilder struct {
	builder  Builder
	provider EpochHeadersProvider
}

func (af *AutoFixBuilder) ProcessHeader(header *types.Header) error {
	err := af.builder.ProcessHeader(header)
	if err != nil {
		return nil
	}
	if errors.Is(err, ErrHeaderRewinded) {
		// Chain rewinded ? rebuild
		// log(uptime calc rewinded)
	} else if errors.Is(err, ErrMissingPreviousHeaders) {
		// Skip in the headers ? rebuild
		// log(uptime calc missing headers)
	} else {
		// Unknown error, log
		return err
	}

	return af.RebuildUpTo(header)
}

func (af *AutoFixBuilder) ComputeUptime(epochLastHeader *types.Header) ([]*big.Int, error) {
	uptime, err := af.builder.ComputeUptime(epochLastHeader)
	if err != nil {
		return uptime, nil
	}
	if errors.Is(err, ErrHeaderRewinded) {
		// Chain rewinded ? rebuild
		// log(uptime calc rewinded)
	} else if errors.Is(err, ErrMissingPreviousHeaders) {
		// Skip in the headers ? rebuild
		// log(uptime calc missing headers)
	} else {
		// Unknown error, log
		return nil, err
	}

	err2 := af.RebuildUpTo(epochLastHeader)
	if err2 != nil {
		return nil, err
	}
	return af.builder.ComputeUptime(epochLastHeader)
}

func (af *AutoFixBuilder) RebuildUpTo(header *types.Header) error {
	af.builder.Clear()
	headers, err := af.provider.GetEpochHeadersUpTo(header)
	if err != nil {
		return err
	}
	for _, h := range headers {
		err2 := af.builder.ProcessHeader(h)
		if err2 != nil {
			return err2
		}
	}
	return nil
}

func (af *AutoFixBuilder) Clear() {
	af.builder.Clear()
}
