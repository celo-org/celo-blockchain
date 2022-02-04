package uptime

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
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
	af := NewAutoFixBuilder(b, &headers{epochSize: 1234})
	assert.Equal(t, uint64(7), af.GetEpoch())
	assert.Equal(t, uint64(1234), af.GetEpochSize())
	assert.Equal(t, header(1), af.GetLastProcessedHeader())
	af.Clear()
	assert.Empty(t, b.headersAdded)
	assert.Nil(t, af.GetLastProcessedHeader())
}

func TestFailOnWrongEpoch(t *testing.T) {
	b := &builder{
		epoch:     2,
		epochSize: 100,
	}
	af := NewAutoFixBuilder(b, &headers{t: t, epochSize: 100, reqs: []headersReq{}})
	// Test the borders
	err := af.ProcessHeader(header(100))
	assert.ErrorIs(t, err, ErrWrongEpoch)
	err2 := af.ProcessHeader(header(201))
	assert.ErrorIs(t, err2, ErrWrongEpoch)
	// Now a random header in a far away epoch
	err3 := af.ProcessHeader(header(5555))
	assert.ErrorIs(t, err3, ErrWrongEpoch)
}

func TestAddOnFirstOfEpoch(t *testing.T) {
	b := &builder{
		epoch:     2,
		epochSize: 100,
	}
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	af := NewAutoFixBuilder(b, &headers{t: t, epochSize: 100, reqs: []headersReq{}})
	// First block from epoch
	err := af.ProcessHeader(header(101))
	assert.NoError(t, err)
	assert.Len(t, b.headersAdded, 1)
	assert.Equal(t, b.headersAdded[0], header(101))
}

func req(upTo *types.Header, limit uint64, retV []*types.Header, retE error) headersReq {
	return headersReq{
		upToHeader: upTo,
		limit:      limit,
		retV:       retV,
		retE:       retE,
	}
}

func TestAddManyFirstOfEpoch(t *testing.T) {
	b := &builder{
		epoch:     2,
		epochSize: 100,
	}
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	last := header(105)
	providerResult := []*types.Header{header(101), header(102), header(103), header(104), header(105)}
	provider := &headers{t: t, epochSize: 100, reqs: []headersReq{
		req(last, 5, providerResult, nil),
	}}
	af := NewAutoFixBuilder(b, provider)
	err := af.ProcessHeader(last)
	assert.NoError(t, err)
	assert.Len(t, b.headersAdded, 5)
	assert.Equal(t, b.headersAdded, providerResult)
}

func TestContinueSequentialAdd(t *testing.T) {
	b := &builder{
		epoch:        2,
		epochSize:    100,
		headersAdded: []*types.Header{header(101), header(102)},
	}
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	af := NewAutoFixBuilder(b, &headers{t: t, epochSize: 100, reqs: []headersReq{}})

	// Ensure proper parent hash
	h := header(103)
	h.ParentHash = header(102).Hash()

	err := af.ProcessHeader(h)
	assert.NoError(t, err)
	assert.Len(t, b.headersAdded, 3)
	assert.Equal(t, b.headersAdded[2], h)
}

// builder is a mock builder for testing
type builder struct {
	epoch        uint64
	epochSize    uint64
	headersAdded []*types.Header
	computeV     []*big.Int
	computeE     error
}

func (b *builder) ProcessHeader(header *types.Header) error {
	b.headersAdded = append(b.headersAdded, header)
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

// headers is a mock headers provider for testing
type headers struct {
	t         *testing.T
	epochSize uint64
	reqs      []headersReq
}

type headersReq struct {
	upToHeader *types.Header
	limit      uint64
	retV       []*types.Header
	retE       error
}

func (h *headers) GetEpochHeadersUpToLimit(epochSize uint64, upToHeader *types.Header, limit uint64) ([]*types.Header, error) {
	assert.Equal(h.t, h.epochSize, epochSize)
	for _, r := range h.reqs {
		if r.upToHeader == upToHeader && r.limit == limit {
			return r.retV, r.retE
		}
	}
	assert.FailNow(h.t, fmt.Sprintf("autoFixBuilder made a non-expected call to the headersProvider: %v %v", upToHeader, limit))
	return nil, nil
}
