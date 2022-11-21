package uptime

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/stretchr/testify/assert"
)

func copy(b *builder) *builder {
	h := make([]*types.Header, len(b.headersAdded))
	for i, hd := range b.headersAdded {
		h[i] = hd
	}
	return &builder{
		epoch:        b.epoch,
		epochSize:    b.epochSize,
		headersAdded: h,
		computeV:     b.computeV,
		computeE:     b.computeE,
	}
}

func header(i int64) *types.Header {
	return &types.Header{
		Number: big.NewInt(i),
		Time:   uint64(i),
	}
}

var computeV []*big.Int = []*big.Int{}
var computeE error = errors.New("test error")

func TestFwdFields(t *testing.T) {
	b := &builder{
		epoch:        7,
		epochSize:    1234,
		headersAdded: []*types.Header{header(0), header(1)},
		computeV:     computeV,
		computeE:     computeE,
	}
	af := NewAutoFixBuilder(b, &headers{epochSize: 1234})
	assert.Equal(t, uint64(7), af.GetEpoch())
}

func TestFailOnWrongEpoch(t *testing.T) {
	b := &builder{
		epoch:     2,
		epochSize: 100,
		computeV:  computeV,
		computeE:  computeE,
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
		computeV:  computeV,
		computeE:  computeE,
	}
	b2 := copy(b) // copy of builder, for the compute test
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	provider := &headers{t: t, epochSize: 100, reqs: []headersReq{}}
	af := NewAutoFixBuilder(b, provider)
	// First block from epoch
	err := af.ProcessHeader(header(101))
	assert.NoError(t, err)
	assert.Len(t, b.headersAdded, 1)
	assert.Equal(t, b.headersAdded[0], header(101))

	// Ensure the forward of compute uptime
	af2 := NewAutoFixBuilder(b2, provider)
	cV, cE := af2.ComputeUptime(header(101))
	assert.Equal(t, computeV, cV)
	assert.Equal(t, computeE, cE)

	assert.Equal(t, []*types.Header{}, b2.headersAdded)
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
		computeV:  computeV,
		computeE:  computeE,
	}
	b2 := copy(b) // copy of builder, for the compute test
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	last := header(105)
	providerResult := []*types.Header{header(101), header(102), header(103), header(104), header(105)}
	provider := &headers{t: t, epochSize: 100, reqs: []headersReq{
		req(last, 5, providerResult, nil),
	}}
	af := NewAutoFixBuilder(b, provider)
	err := af.ProcessHeader(last)
	assert.NoError(t, err)
	assert.Equal(t, providerResult, b.headersAdded)

	// Ensure the forward of compute uptime
	af2 := NewAutoFixBuilder(b2, provider)
	cV, cE := af2.ComputeUptime(last)
	assert.Equal(t, computeV, cV)
	assert.Equal(t, computeE, cE)

	assert.Equal(t, []*types.Header{}, b2.headersAdded)
}

func TestContinueSequentialAdd(t *testing.T) {
	initial := []*types.Header{header(101), header(102)}
	b := &builder{
		epoch:        2,
		epochSize:    100,
		headersAdded: initial,
		computeV:     computeV,
		computeE:     computeE,
	}
	b2 := copy(b) // copy of builder, for the compute test
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	provider := &headers{t: t, epochSize: 100, reqs: []headersReq{}}
	af := NewAutoFixBuilder(b, provider)

	// Ensure proper parent hash
	h := header(103)
	h.ParentHash = header(102).Hash()

	err := af.ProcessHeader(h)
	assert.NoError(t, err)
	assert.Len(t, b.headersAdded, 3)
	assert.Equal(t, h, b.headersAdded[2])

	// Ensure the forward of compute uptime
	af2 := NewAutoFixBuilder(b2, provider)
	cV, cE := af2.ComputeUptime(h)
	assert.Equal(t, computeV, cV)
	assert.Equal(t, computeE, cE)

	assert.Equal(t, initial, b2.headersAdded)
}

func TestSequentialAddFork(t *testing.T) {
	initial := []*types.Header{header(101), header(102)}
	b := &builder{
		epoch:        2,
		epochSize:    100,
		headersAdded: initial,
		computeV:     computeV,
		computeE:     computeE,
	}
	b2 := copy(b) // copy of builder, for the compute test
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	last := header(103)
	providerResult := []*types.Header{header(101), header(102), last}
	providerResult[0].GasUsed = 1477 // ensure difference from initial headersAdded
	provider := &headers{t: t, epochSize: 100, reqs: []headersReq{
		req(last, 3, providerResult, nil),
	}}
	af := NewAutoFixBuilder(b, provider)

	// Wrong hash to provoke a fork
	last.ParentHash = header(9999).Hash()

	err := af.ProcessHeader(last)
	assert.NoError(t, err)
	assert.Equal(t, providerResult, b.headersAdded)

	// Ensure the forward of compute uptime
	af2 := NewAutoFixBuilder(b2, provider)
	cV, cE := af2.ComputeUptime(last)
	assert.Equal(t, computeV, cV)
	assert.Equal(t, computeE, cE)

	assert.Equal(t, initial, b2.headersAdded)
}

func TestRewind(t *testing.T) {
	initial := []*types.Header{header(101), header(102), header(103)}
	b := &builder{
		epoch:        2,
		epochSize:    100,
		headersAdded: initial,
		computeV:     computeV,
		computeE:     computeE,
	}
	b2 := copy(b) // copy of builder, for the compute test
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	last := header(102)
	providerResult := []*types.Header{header(101), header(102)}
	provider := &headers{t: t, epochSize: 100, reqs: []headersReq{
		req(last, 2, providerResult, nil),
	}}
	af := NewAutoFixBuilder(b, provider)
	err := af.ProcessHeader(last)
	assert.NoError(t, err)
	assert.Equal(t, providerResult, b.headersAdded)

	// Ensure the forward of compute uptime
	af2 := NewAutoFixBuilder(b2, provider)
	cV, cE := af2.ComputeUptime(last)
	assert.Equal(t, computeV, cV)
	assert.Equal(t, computeE, cE)

	assert.Equal(t, initial, b2.headersAdded)
}

func TestDoNothing(t *testing.T) {
	last := header(103)
	last.GasUsed = 1234
	h := header(103)
	h.GasUsed = 1234 // ensure same hash as last
	initial := []*types.Header{header(101), header(102), last}
	b := &builder{
		epoch:        2,
		epochSize:    100,
		headersAdded: initial,
		computeV:     computeV,
		computeE:     computeE,
	}
	b2 := copy(b) // copy of builder, for the compute test
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	provider := &headers{t: t, epochSize: 100, reqs: []headersReq{}}
	af := NewAutoFixBuilder(b, provider)
	err := af.ProcessHeader(h)
	assert.NoError(t, err)
	assert.Equal(t, initial, b.headersAdded)

	// Ensure the forward of compute uptime
	af2 := NewAutoFixBuilder(b2, provider)
	cV, cE := af2.ComputeUptime(h)
	assert.Equal(t, computeV, cV)
	assert.Equal(t, computeE, cE)

	assert.Equal(t, initial, b2.headersAdded)
}

func TestSameHeightRebuild(t *testing.T) {
	last := header(103)
	last.GasUsed = 1234
	h := header(103)
	h.GasUsed = 1237 // difference in hash
	initial := []*types.Header{header(101), header(102), last}
	b := &builder{
		epoch:        2,
		epochSize:    100,
		headersAdded: initial,
		computeV:     computeV,
		computeE:     computeE,
	}
	b2 := copy(b) // copy of builder, for the compute test
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	providerResult := []*types.Header{header(101), header(102), last}
	provider := &headers{t: t, epochSize: 100, reqs: []headersReq{
		req(h, 3, providerResult, nil),
	}}
	af := NewAutoFixBuilder(b, provider)
	err := af.ProcessHeader(h)
	assert.NoError(t, err)
	assert.Equal(t, providerResult, b.headersAdded)

	// Ensure the forward of compute uptime
	af2 := NewAutoFixBuilder(b2, provider)
	cV, cE := af2.ComputeUptime(h)
	assert.Equal(t, computeV, cV)
	assert.Equal(t, computeE, cE)

	assert.Equal(t, initial, b2.headersAdded)
}

func TestAdvance(t *testing.T) {
	pivot := header(103)
	pivot.GasUsed = 12345 // enforce non trivial hash
	initial := []*types.Header{header(101), header(102), pivot}
	b := &builder{
		epoch:        2,
		epochSize:    100,
		headersAdded: initial,
		computeV:     computeV,
		computeE:     computeE,
	}
	b2 := copy(b)
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	last := header(106)
	providerResult := []*types.Header{pivot, header(104), header(105), header(106)}
	provider := &headers{t: t, epochSize: 100, reqs: []headersReq{
		req(last, 4, providerResult, nil),
	}}
	af := NewAutoFixBuilder(b, provider)
	err := af.ProcessHeader(last)
	assert.NoError(t, err)
	endResult := []*types.Header{header(101), header(102), pivot, header(104), header(105), header(106)}
	assert.Equal(t, endResult, b.headersAdded)

	// Ensure the forward of compute uptime
	af2 := NewAutoFixBuilder(b2, provider)
	cV, cE := af2.ComputeUptime(last)
	assert.Equal(t, computeV, cV)
	assert.Equal(t, computeE, cE)

	assert.Equal(t, initial, b2.headersAdded)
}

func TestAdvanceFork(t *testing.T) {
	pivot := header(103)
	pivot.GasUsed = 12345 // enforce non trivial hash
	forkPivot := header(103)
	forkPivot.GasUsed = 99999
	initial := []*types.Header{header(101), header(102), pivot}
	b := &builder{
		epoch:        2,
		epochSize:    100,
		headersAdded: initial,
		computeV:     computeV,
		computeE:     computeE,
	}
	b2 := copy(b)
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	last := header(106)
	providerResult1 := []*types.Header{forkPivot, header(104), header(105), header(106)}
	providerResult2 := []*types.Header{header(101), header(102), forkPivot, header(104), header(105), header(106)}
	provider := &headers{t: t, epochSize: 100, reqs: []headersReq{
		req(last, 4, providerResult1, nil),
		req(last, 6, providerResult2, nil),
	}}
	af := NewAutoFixBuilder(b, provider)
	err := af.ProcessHeader(last)
	assert.NoError(t, err)
	assert.Equal(t, providerResult2, b.headersAdded)

	// Ensure the forward of compute uptime
	af2 := NewAutoFixBuilder(b2, provider)
	cV, cE := af2.ComputeUptime(last)
	assert.Equal(t, computeV, cV)
	assert.Equal(t, computeE, cE)

	assert.Equal(t, initial, b2.headersAdded)
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

func (b *builder) Copy() FixableBuilder {
	return &builder{
		epoch:        b.epoch,
		epochSize:    b.epochSize,
		headersAdded: b.headersAdded,
		computeV:     b.computeV,
		computeE:     b.computeE,
	}
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
