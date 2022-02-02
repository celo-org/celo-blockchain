package uptime

import (
	"math/big"

	"errors"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
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

	GetLastProcessedHeader() *types.Header

	GetEpoch() uint64
	GetEpochSize() uint64

	Computer // Not 100% sure Builder should include Computer or if they can be completely separated.
}

type EpochHeadersProvider interface {
	// GetEpochHeadersUpTo returns all headers from the same epoch as the header provided (included) ordered.
	GetEpochHeadersUpTo(upToHeader *types.Header, from *types.Header) ([]*types.Header, error)
}

// AutoFixBuilder is an uptime Builder that will fix rewinds and missing headers.
type AutoFixBuilder struct {
	builder  Builder
	provider EpochHeadersProvider
}

func (af *AutoFixBuilder) ProcessHeader(header *types.Header) error {
	lastHeader := af.builder.GetLastProcessedHeader()

	if lastHeader == nil {
		firstBlock, err := istanbul.GetEpochFirstBlockNumber(af.builder.GetEpoch(), af.builder.GetEpochSize())
		if err != nil {
			return err
		}
		if header.Number.Uint64() == firstBlock {
			return af.builder.ProcessHeader(header)
		}
	} else {
		if lastHeader.Number.Cmp(header.Number) < 0 {
			if (lastHeader.Number.Uint64() + 1) == header.Number.Uint64() {
				return af.builder.ProcessHeader(header)
			}
		} else {
			// return error header lower
		}
	}

	return buildUpTo(af.provider, af.builder, header, lastHeader)
}

func (af *AutoFixBuilder) ComputeUptime(epochLastHeader *types.Header) ([]*big.Int, error) {
	lastBlock := istanbul.GetEpochLastBlockNumber(af.builder.GetEpoch(), af.builder.GetEpochSize())
	if lastBlock != epochLastHeader.Number.Uint64() {
		// return error
	}
	lastHeader := af.builder.GetLastProcessedHeader()
	if lastHeader == nil || lastHeader.Number.Cmp(epochLastHeader.Number) < 0 {
		if err := af.ProcessHeader(epochLastHeader); err != nil {
			return nil, err
		}
	}

	return af.builder.ComputeUptime(epochLastHeader)
}

func (af *AutoFixBuilder) RebuildUpTo(header *types.Header) error {
	af.builder.Clear()
	return buildUpTo(af.provider, af.builder, header, nil)
}

func buildUpTo(provider EpochHeadersProvider, builder Builder, header, from *types.Header) error {
	headers, err := provider.GetEpochHeadersUpTo(header, from)
	if err != nil {
		return err
	}
	for _, h := range headers {
		err2 := builder.ProcessHeader(h)
		if err2 != nil {
			return err2
		}
	}
	return nil
}

func (af *AutoFixBuilder) Clear() {
	af.builder.Clear()
}
