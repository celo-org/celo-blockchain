package istanbul

import (
	"fmt"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
)

type ChainHeaderReader interface {
	// GetHeader retrieves a block header from the database by hash and number.
	GetHeader(hash common.Hash, number uint64) *types.Header

	// GetHeaderByNumber retrieves a block header from the database by number.
	GetHeaderByNumber(number uint64) *types.Header
}

type EpochHeadersProvider interface {
	// GetEpochHeadersUpTo returns all headers from the same epoch as the header provided (included) ordered, up to
	// the given one, with a limit on the amount of headers to return. E.g limit 3 would return [prev-2, prev-1, upToHeader], assuming
	// all of them are on the same epoch.
	GetEpochHeadersUpToLimit(epochSize uint64, upToHeader *types.Header, limit uint64) ([]*types.Header, error)
}

func NewHeadersProvider(chr ChainHeaderReader) EpochHeadersProvider {
	return &dbHeadersProvider{chr: chr}
}

type dbHeadersProvider struct {
	chr ChainHeaderReader
}

func (d *dbHeadersProvider) GetEpochHeadersUpToLimit(epochSize uint64, upToHeader *types.Header, limit uint64) ([]*types.Header, error) {
	number := upToHeader.Number.Uint64()
	numberWithinEpoch := GetNumberWithinEpoch(number, epochSize)
	amountToLoad := numberWithinEpoch - 1
	if amountToLoad == 0 {
		// Nothing to do
		return []*types.Header{upToHeader}, nil
	}
	headers := getHeaders(d.chr, number-1, amountToLoad)
	if headers == nil {
		// Error retrieving headers
		return nil, fmt.Errorf("error attempting to retrieve headers: epochSize %d, headerHash %v", epochSize, upToHeader.Hash())
	}
	return append(headers, upToHeader), nil
}

func getHeaders(chr ChainHeaderReader, lastBlock uint64, amount uint64) []*types.Header {
	headers := make([]*types.Header, amount)
	lastHeader := chr.GetHeaderByNumber(lastBlock)
	if lastHeader == nil {
		// Error retrieving header
		return nil
	}
	headers[amount-1] = lastHeader
	for i := amount - 2; i >= 0; i-- {
		headers[i] = chr.GetHeader(headers[i+1].ParentHash, headers[i+1].Number.Uint64()-1)
		if headers[i] == nil {
			// Error retrieving header
			return nil
		}
	}
	return headers
}
