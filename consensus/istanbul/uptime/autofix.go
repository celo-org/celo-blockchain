package uptime

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
)

// autoFixBuilder is an uptime Builder that will fix rewinds and missing headers from a
// decorated builder.
type autoFixBuilder struct {
	builder  Builder
	provider istanbul.EpochHeadersProvider
}

func NewAutoFixBuilder(builder Builder, provider istanbul.EpochHeadersProvider) Builder {
	return &autoFixBuilder{
		builder:  builder,
		provider: provider,
	}
}

// ProcessHeader ensures the underlying builder is properly build up to the header given.
// There are a few different cases to manage:
// 1) Underlying builder is empty. Add all the epoch headers up to the one given
// 2) New header is behind the builder. Rebuild.
// 3) New header is the following header, just add.
// 4) New header is a few headers ahead. Bring missing ones and either add or rebuild if there's a fork
// 5) New header is the same height as the last one from the builder. Do nothing if it's the same
// or rebuild if it's a different hash.
//
// Note: A big assumption of the autofix is that when given a header, it will be the header returned
// by the EpochHeadersProvider when calling for epoch headers up to that one. That is, it assumes
// that it was a valid header stored in the chain.
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
		af.builder.Clear()
		return af.cleanBuild(header)
	}

	// Normal sequential case
	if number == lastHeaderNumber+1 {
		if header.ParentHash == lastHeader.Hash() {
			// Valid chain, add the header
			return af.builder.ProcessHeader(header)
		}
		// Fork! Rebuild
		af.builder.Clear()
		return af.cleanBuild(header)
	}

	if number > lastHeaderNumber+1 {
		// Normal advance (1 or more headers missing). Try to advance the builder
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
	af.builder.Clear()
	return af.cleanBuild(header)
}

// cleanBuild does a clean build up to the header given, assuming the decorated builder is empty.
func (af *autoFixBuilder) cleanBuild(upTo *types.Header) error {
	epochSize := af.builder.GetEpochSize()
	numberWithinEpoch := istanbul.GetNumberWithinEpoch(upTo.Number.Uint64(), epochSize)
	// If this is the first header in the epoch, no need to call the provider
	if numberWithinEpoch == 1 {
		return af.builder.ProcessHeader(upTo)
	}
	// Else, load all the previous headers from the db.
	headers, err := af.provider.GetEpochHeadersUpToLimit(epochSize, upTo, numberWithinEpoch)
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
	upNumber := upTo.Number.Uint64()
	fromNumber := from.Number.Uint64()
	limit := upNumber - fromNumber + 1
	headers, err := af.provider.GetEpochHeadersUpToLimit(af.builder.GetEpochSize(), upTo, limit)
	if err != nil {
		return err
	}
	// Verify that this is not a fork
	if headers[0].Hash() != from.Hash() {
		// Fork!
		// Rebuild
		return af.cleanBuild(upTo)
	}
	return af.addAll(headers[1:])
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
