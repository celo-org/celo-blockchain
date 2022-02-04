package uptime

import (
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/stretchr/testify/assert"
)

func header(i int64) *types.Header {
	return &types.Header{
		Number: big.NewInt(i),
		Time:   uint64(i),
	}
}

func TestFwdFields(t *testing.T) {
	b := &builder{
		epoch:        7,
		epochSize:    1234,
		headersAdded: []*types.Header{header(0), header(1)},
	}
	af := NewAutoFixBuilder(b, nil)
	assert.Equal(t, uint64(7), af.GetEpoch())
	assert.Equal(t, uint64(1234), af.GetEpochSize())
	assert.Equal(t, header(1), af.GetLastProcessedHeader())
	af.Clear()
	assert.Empty(t, b.headersAdded)
	assert.Nil(t, af.GetLastProcessedHeader())
}

type builder struct {
	epoch        uint64
	epochSize    uint64
	headersAdded []*types.Header
	computeV     []*big.Int
	computeE     error
}

func (b *builder) ProcessHeader(header *types.Header) error {
	return nil
}

// Clear resets this builder
func (b *builder) Clear() {
	b.headersAdded = make([]*types.Header, 0)
}

// GetLastProcessedHeader returns the last processed header by this Builder.
func (b *builder) GetLastProcessedHeader() *types.Header {
	if len(b.headersAdded) > 0 {
		return b.headersAdded[len(b.headersAdded)-1]
	}
	return nil
}

// GetEpochSize returns the epoch size for the current epoch in this Builder.
func (b *builder) GetEpochSize() uint64 {
	return b.epochSize
}

// GetEpoch returns the epoch for this uptime Builder.
func (b *builder) GetEpoch() uint64 {
	return b.epoch
}

func (b *builder) ComputeUptime(epochLastHeader *types.Header) ([]*big.Int, error) {
	return b.computeV, b.computeE
}
