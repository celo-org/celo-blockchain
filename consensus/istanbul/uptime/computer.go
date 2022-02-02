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

type EpochHeadersProvider interface {
	// GetEpochHeadersUpTo returns all headers from the same epoch as the header provided (included) ordered, up to
	// the given one.
	GetEpochHeadersUpTo(upToHeader *types.Header) ([]*types.Header, error)

	// GetEpochHeadersUpTo returns all headers from the same epoch as the header provided (included) ordered, up to
	// the given one, but starting at the header at height fromHeight.
	GetEpochHeadersUpToFrom(upToHeader *types.Header, fromHeight uint64) ([]*types.Header, error)
}

// AutoFixBuilder is an uptime Builder that will fix rewinds and missing headers from a
// decorated builder.
type autoFixBuilder struct {
	builder  Builder
	provider EpochHeadersProvider
}

func NewAutoFixBuilder(builder Builder, provider EpochHeadersProvider) Builder {
	return &autoFixBuilder{
		builder:  builder,
		provider: provider,
	}
}

func (af *autoFixBuilder) ProcessHeader(header *types.Header) error {
	number := header.Number.Uint64()
	epoch := istanbul.GetEpochNumber(number, af.builder.GetEpochSize())
	if af.builder.GetEpoch() != epoch {
		// Provided header from the wrong epoch
		return ErrWrongEpoch
	}

	lastHeader := af.builder.GetLastProcessedHeader()

	if lastHeader == nil {
		// It's from the same epoch and the builder is empty.
		// Build up to the new header.
		return af.cleanBuild(header)
	}

	lastHeaderNumber := lastHeader.Number.Uint64()

	// If it's a rewind, rebuild
	if number < lastHeaderNumber {
		return af.cleanBuild(header)
	}

	if number > lastHeaderNumber {
		// Normal advance. Try to advance the builder
		return af.advance(lastHeader, header)
	}

	// number == lastHeaderNumber
	// check if it's an idempotent call or if
	// there' a fork
	if lastHeader.Hash() == header.Hash() {
		// Nothing to do.
		return nil
	}

	// fork, rebuild
	return af.cleanBuild(header)
}

// cleanBuild does a clean build up to the header given.
func (af *autoFixBuilder) cleanBuild(upTo *types.Header) error {
	headers, err := af.provider.GetEpochHeadersUpTo(upTo)
	if err != nil {
		return err
	}
	return af.addAll(headers)
}

// advance advances the computation from the header given to the upTo header given.
// This method requests to the provider the missing headers in between and adds them.
// The only caveat is if the 'from' header is not in the same chain as the 'upTo' header.
// In that case, a rebuild is made.
func (af *autoFixBuilder) advance(from, upTo *types.Header) error {
	headers, err := af.provider.GetEpochHeadersUpToFrom(upTo, from.Number.Uint64())
	if err != nil {
		return err
	}
	// Verify that this is not a fork
	if headers[0].Hash() != from.Hash() {
		// Fork!
		// Get the previous headers from the epoch, and add all
		preforkHeaders, err := af.provider.GetEpochHeadersUpTo(headers[0])
		if err != nil {
			return err
		}
		headers := headers[1:] // remove repeated header
		all := append(preforkHeaders, headers...)
		af.builder.Clear()
		return af.addAll(all)
	}
	return af.addAll(headers)
}

// addAll adds all the headers to the decorated builder.
func (af *autoFixBuilder) addAll(headers []*types.Header) error {
	for _, h := range headers {
		err := af.builder.ProcessHeader(h)
		if err != nil {
			return err
		}
	}
	return nil
}

func (af *autoFixBuilder) ComputeUptime(epochLastHeader *types.Header) ([]*big.Int, error) {
	err := af.ProcessHeader(epochLastHeader)
	if err != nil {
		return nil, err
	}
	return af.builder.ComputeUptime(epochLastHeader)
}

func (af *autoFixBuilder) GetEpochSize() uint64 {
	return af.builder.GetEpochSize()
}

func (af *autoFixBuilder) GetEpoch() uint64 {
	return af.builder.GetEpoch()
}

func (af *autoFixBuilder) Clear() {
	af.builder.Clear()
}

func (af *autoFixBuilder) GetLastProcessedHeader() *types.Header {
	return af.builder.GetLastProcessedHeader()
}
