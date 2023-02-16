package istanbul

import (
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/stretchr/testify/assert"
)

func header(number int64, parent *types.Header) *types.Header {
	h := &types.Header{
		Number: big.NewInt(number),
	}
	if parent != nil {
		h.ParentHash = parent.Hash()
	}
	return h
}

func TestGetHeadersZeroAmount(t *testing.T) {
	r := getHeaders(nil, 3, 0)
	assert.Empty(t, r)
}

func TestFullZeroAmount(t *testing.T) {
	hp := NewHeadersProvider(nil)
	h := header(2400, nil)
	r, err := hp.GetEpochHeadersUpToLimit(2000, h, 0)
	assert.NoError(t, err)
	assert.Empty(t, r)
}

func TestGetHeadersOneAmount(t *testing.T) {
	expected := header(1337, nil)
	chr := &mockHP{
		t:               t,
		headersByNumber: map[uint64]*types.Header{1337: expected},
	}
	r := getHeaders(chr, 1337, 1)
	assert.Len(t, r, 1)
	assert.Same(t, expected, r[0])
}

func TestFullOneAmount(t *testing.T) {
	expected := header(1337, nil)
	chr := &mockHP{
		t: t,
	}
	hp := NewHeadersProvider(chr)
	r, err := hp.GetEpochHeadersUpToLimit(2000, expected, 1)
	assert.NoError(t, err)
	assert.Len(t, r, 1)
	assert.Same(t, expected, r[0])
}

func TestFullOnePlusOneAmount(t *testing.T) {
	expected := header(1337, nil)
	expected2 := header(1338, expected)
	chr := &mockHP{
		t:               t,
		headersByNumber: map[uint64]*types.Header{1337: expected},
	}
	hp := NewHeadersProvider(chr)
	r, err := hp.GetEpochHeadersUpToLimit(2000, expected2, 2)
	assert.NoError(t, err)
	assert.Len(t, r, 2)
	assert.Same(t, expected, r[0])
	assert.Same(t, expected2, r[1])
}

func TestGetHeadersMany(t *testing.T) {
	h1 := header(1337, nil)
	h2 := header(1338, h1)
	h3 := header(1339, h2)
	h4 := header(1340, h3)
	h5 := header(1341, h4)
	h6 := header(1342, h5)
	chr := &mockHP{
		t:               t,
		headersByNumber: map[uint64]*types.Header{1342: h6},
		headersByHash: map[common.Hash]*types.Header{
			h2.Hash(): h2,
			h3.Hash(): h3,
			h4.Hash(): h4,
			h5.Hash(): h5,
			h6.Hash(): h6,
		},
	}
	r := getHeaders(chr, 1342, 5)
	assert.Len(t, r, 5)
	assert.Same(t, h2, r[0])
	assert.Same(t, h3, r[1])
	assert.Same(t, h4, r[2])
	assert.Same(t, h5, r[3])
	assert.Same(t, h6, r[4])
}

func TestFullMany(t *testing.T) {
	h1 := header(1337, nil)
	h2 := header(1338, h1)
	h3 := header(1339, h2)
	h4 := header(1340, h3)
	h5 := header(1341, h4)
	h6 := header(1342, h5)
	chr := &mockHP{
		t:               t,
		headersByNumber: map[uint64]*types.Header{1341: h5},
		headersByHash: map[common.Hash]*types.Header{
			h2.Hash(): h2,
			h3.Hash(): h3,
			h4.Hash(): h4,
			h5.Hash(): h5,
			h6.Hash(): h6,
		},
	}
	hp := NewHeadersProvider(chr)
	r, err := hp.GetEpochHeadersUpToLimit(2000, h6, 5)
	assert.NoError(t, err)
	assert.Len(t, r, 5)
	assert.Same(t, h2, r[0])
	assert.Same(t, h3, r[1])
	assert.Same(t, h4, r[2])
	assert.Same(t, h5, r[3])
	assert.Same(t, h6, r[4])
}

func TestFullManyMidEpoch(t *testing.T) {
	h1 := header(1337, nil)
	h2 := header(1338, h1)
	h3 := header(1339, h2)
	h4 := header(1340, h3)
	h5 := header(1341, h4)
	h6 := header(1342, h5)
	chr := &mockHP{
		t:               t,
		headersByNumber: map[uint64]*types.Header{1341: h5},
		headersByHash: map[common.Hash]*types.Header{
			h2.Hash(): h2,
			h3.Hash(): h3,
			h4.Hash(): h4,
			h5.Hash(): h5,
			h6.Hash(): h6,
		},
	}
	hp := NewHeadersProvider(chr)
	r, err := hp.GetEpochHeadersUpToLimit(20, h6, 5)
	assert.NoError(t, err)
	assert.Len(t, r, 2)
	assert.Same(t, h5, r[0])
	assert.Same(t, h6, r[1])
}

type mockHP struct {
	headersByNumber map[uint64]*types.Header
	headersByHash   map[common.Hash]*types.Header
	t               *testing.T
}

func (m *mockHP) GetHeader(hash common.Hash, number uint64) *types.Header {
	h := m.headersByHash[hash]
	if h == nil {
		m.t.Fatal("Requested non existent header by hash:", hash, number)
	}
	if h.Number.Uint64() != number {
		m.t.Fatal("Number given does not match number in mock", number, h.Number.Uint64())
	}
	return h
}

func (m *mockHP) GetHeaderByNumber(number uint64) *types.Header {
	h, ok := m.headersByNumber[number]
	if !ok {
		m.t.Fatal("Requested non existent header by number: ", number)
	}
	return h
}
