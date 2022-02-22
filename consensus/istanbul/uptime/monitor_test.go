package uptime

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/stretchr/testify/assert"
)

func TestUptime(t *testing.T) {
	var uptimes *Uptime = new(Uptime)
	uptimes.Entries = make([]UptimeEntry, 3)
	// (there can't be less than 2/3rds of validators sigs in a valid bitmap)
	bitmaps := []*big.Int{
		big.NewInt(7), // 111     // signature bitmap for block #1
		big.NewInt(5), // 101
		big.NewInt(3), // 011
		big.NewInt(5), // 101
		big.NewInt(7), // 111
		big.NewInt(5), // 101     // signature bitmap for block #6
	}
	// start on first block of window
	block := uint64(1)
	// use a window of 2 blocks - ideally we want to expand
	monitoringWindow := MustMonitoringWindow(1, 10, 2) // [2,8]

	for _, bitmap := range bitmaps {
		// these tests to increase our confidence
		updateUptime(uptimes, block, bitmap, 2, monitoringWindow)
		block++
	}

	expected := &Uptime{
		LatestHeader: nil,
		Entries: []UptimeEntry{
			{
				UpBlocks:        5,
				LastSignedBlock: 6,
			},
			{
				UpBlocks:        5,
				LastSignedBlock: 5,
			},
			{
				UpBlocks:        5,
				LastSignedBlock: 6,
			},
		},
	}
	if !reflect.DeepEqual(uptimes, expected) {
		t.Fatalf("uptimes were not updated correctly, got %v, expected %v", uptimes, expected)
	}
}

func TestUptimeSingle(t *testing.T) {
	var uptimes *Uptime = new(Uptime)
	uptimes.Entries = make([]UptimeEntry, 3)
	monitoringWindow := MustMonitoringWindow(2, 211, 3)
	updateUptime(uptimes, 211, big.NewInt(7), 3, monitoringWindow)
	// the first 2 uptime updates do not get scored since they're within the
	// first window after the epoch block
	expected := &Uptime{
		LatestHeader: nil,
		Entries: []UptimeEntry{
			{
				UpBlocks:        0,
				LastSignedBlock: 211,
			},
			{
				UpBlocks:        0,
				LastSignedBlock: 211,
			},
			{
				UpBlocks:        0,
				LastSignedBlock: 211,
			},
		},
	}
	if !reflect.DeepEqual(uptimes, expected) {
		t.Fatalf("uptimes were not updated correctly, got %v, expected %v", uptimes, expected)
	}
}

func mockHeader(i int64, bitmap *big.Int, parentHash common.Hash) *types.Header {
	extra, _ := rlp.EncodeToBytes(&types.IstanbulExtra{
		ParentAggregatedSeal: types.IstanbulAggregatedSeal{
			Bitmap: bitmap,
		},
	})

	return &types.Header{
		Number:     big.NewInt(i),
		ParentHash: parentHash,
		Time:       uint64(i),
		Extra:      append(make([]byte, types.IstanbulExtraVanity), extra...),
	}
}

func TestMonitorFailOnWrongEpoch(t *testing.T) {
	monitor := NewMonitor(100, 2, 10, 3)
	// Test the borders
	err := monitor.ProcessHeader(mockHeader(100, big.NewInt(7), common.Hash{}))
	assert.ErrorIs(t, err, ErrWrongEpoch)
	err2 := monitor.ProcessHeader(mockHeader(201, big.NewInt(7), common.Hash{}))
	assert.ErrorIs(t, err2, ErrWrongEpoch)
	// Now a random header in a far away epoch
	err3 := monitor.ProcessHeader(mockHeader(5555, big.NewInt(7), common.Hash{}))
	assert.ErrorIs(t, err3, ErrWrongEpoch)
}

func TestMonitorAddFirstHeaderOfEpoch(t *testing.T) {
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	monitor := NewMonitor(100, 2, 10, 3)
	// First block from epoch
	header101 := mockHeader(101, big.NewInt(7), common.Hash{})
	err := monitor.ProcessHeader(header101)
	assert.NoError(t, err)
	assert.Equal(t, header101, monitor.GetLastProcessedHeader())
}

func TestMonitorNotComputeUntilLockbackWindowPlusOne(t *testing.T) {
	var uptime []*big.Int
	monitor := NewMonitor(100, 2, 2, 3)
	// First block from epoch
	header101 := mockHeader(101, big.NewInt(0), common.Hash{})
	err := monitor.ProcessHeader(header101)
	assert.NoError(t, err)
	_, err = monitor.ComputeUptime(header101)
	assert.ErrorIs(t, err, ErrUnpreparedCompute)
	header102 := mockHeader(102, big.NewInt(0), header101.Hash())
	err = monitor.ProcessHeader(header102)
	assert.NoError(t, err)
	_, err = monitor.ComputeUptime(header102)
	assert.ErrorIs(t, err, ErrUnpreparedCompute)
	header103 := mockHeader(103, big.NewInt(7), header102.Hash())
	err = monitor.ProcessHeader(header103)
	assert.NoError(t, err)
	uptime, err = monitor.ComputeUptime(header103)
	assert.NoError(t, err)
	assert.Equal(t, []*big.Int{params.Fixidity1, params.Fixidity1, params.Fixidity1}, uptime)
	assert.Equal(t, monitor.GetLastProcessedHeader(), header103)
}

func TestMonitorAddLastHeaderOfEpoch(t *testing.T) {
	monitor := NewMonitor(5, 2, 2, 3)
	// First block from epoch
	header6 := mockHeader(6, big.NewInt(0), common.Hash{})
	monitor.ProcessHeader(header6)
	header7 := mockHeader(7, big.NewInt(7), header6.Hash())
	monitor.ProcessHeader(header7)
	header8 := mockHeader(8, big.NewInt(0), header7.Hash())
	monitor.ProcessHeader(header8)
	header9 := mockHeader(9, big.NewInt(0), header8.Hash())
	monitor.ProcessHeader(header9)
	header10 := mockHeader(10, big.NewInt(0), header9.Hash())
	monitor.ProcessHeader(header10)
	uptime, err := monitor.ComputeUptime(header10)
	assert.NoError(t, err)
	halfScore := big.NewInt(0).Div(params.Fixidity1, big.NewInt(2))
	assert.Equal(t, []*big.Int{halfScore, halfScore, halfScore}, uptime)
	assert.Equal(t, header10, monitor.GetLastProcessedHeader())
}

func TestMonitorAddSameHeader(t *testing.T) {
	monitor := NewMonitor(100, 2, 10, 2)
	// First block from epoch
	header101 := mockHeader(101, big.NewInt(7), common.Hash{})
	err := monitor.ProcessHeader(header101)
	assert.NoError(t, err)
	err = monitor.ProcessHeader(header101)
	assert.ErrorIs(t, err, ErrHeaderNumberAlreadyUsed)
}

func TestMonitorAddGappedHeader(t *testing.T) {
	monitor := NewMonitor(100, 2, 10, 2)
	// First block from epoch
	header101 := mockHeader(101, big.NewInt(7), common.Hash{})
	err := monitor.ProcessHeader(header101)
	assert.NoError(t, err)
	header103 := mockHeader(103, big.NewInt(7), common.Hash{})
	err = monitor.ProcessHeader(header103)
	assert.ErrorIs(t, err, ErrMissingPreviousHeaders)
}

func TestMonitorFirstHeadearNonEpochStart(t *testing.T) {
	monitor := NewMonitor(100, 2, 10, 2)
	// First block from epoch
	header103 := mockHeader(103, big.NewInt(7), common.Hash{})
	err := monitor.ProcessHeader(header103)
	assert.ErrorIs(t, err, ErrMissingPreviousHeaders)
}

func TestMonitorClear(t *testing.T) {
	assert.True(t, istanbul.IsFirstBlockOfEpoch(101, 100))
	monitor := NewMonitor(100, 2, 10, 3)
	// First block from epoch
	header101 := mockHeader(101, big.NewInt(7), common.Hash{})
	err := monitor.ProcessHeader(header101)
	assert.NoError(t, err)
	monitor.Clear()
	err = monitor.ProcessHeader(header101)
	assert.NoError(t, err)
}

func TestMonitorReturnValSetResultsUsingBiggerBitmaps(t *testing.T) {
	// 3 validators
	monitor := NewMonitor(100, 2, 5, 3)
	headerLast := mockHeader(int64(100), big.NewInt(0), common.Hash{})
	for i := 1; i <= 20; i++ {
		// 111100 => 60
		actualHeader := mockHeader(int64(100+i), big.NewInt(60), headerLast.Hash())
		err := monitor.ProcessHeader(actualHeader)
		assert.NoError(t, err)
		headerLast = actualHeader
	}
	score, err := monitor.ComputeUptime(headerLast)
	assert.NoError(t, err)
	assert.Len(t, score, 3)
	assert.Equal(t, score[0], big.NewInt(0))
	assert.Equal(t, score[1], big.NewInt(0))
	assert.Equal(t, score[2], params.Fixidity1)
}

func TestMonitorReturnValSetResultsUsingSmallerBitmaps(t *testing.T) {
	// 3 validators
	monitor := NewMonitor(100, 2, 5, 3)
	headerLast := mockHeader(int64(100), big.NewInt(0), common.Hash{})
	for i := 1; i <= 20; i++ {
		actualHeader := mockHeader(int64(100+i), big.NewInt(0), headerLast.Hash())
		err := monitor.ProcessHeader(actualHeader)
		assert.NoError(t, err)
		headerLast = actualHeader
	}
	score, err := monitor.ComputeUptime(headerLast)
	assert.NoError(t, err)
	assert.Len(t, score, 3)
	assert.Equal(t, score[0], big.NewInt(0))
	assert.Equal(t, score[1], big.NewInt(0))
	assert.Equal(t, score[2], big.NewInt(0))
}

func TestMonitorPerfectScoreEqualToOne(t *testing.T) {
	// 3 validators
	monitor := NewMonitor(100, 2, 5, 3)
	headerLast := mockHeader(int64(100), big.NewInt(0), common.Hash{})
	for i := 1; i <= 100; i++ {
		actualHeader := mockHeader(int64(100+i), big.NewInt(7), headerLast.Hash())
		err := monitor.ProcessHeader(actualHeader)
		assert.NoError(t, err)
		headerLast = actualHeader
	}
	outOfEpocHeader := mockHeader(headerLast.Number.Int64()+1, big.NewInt(7), headerLast.Hash())
	// One more header should be part of the next epoch
	err := monitor.ProcessHeader(outOfEpocHeader)
	assert.ErrorIs(t, err, ErrWrongEpoch)

	score, err := monitor.ComputeUptime(headerLast)
	assert.NoError(t, err)
	assert.Len(t, score, 3)
	assert.Equal(t, score[0], params.Fixidity1)
	assert.Equal(t, score[1], params.Fixidity1)
	assert.Equal(t, score[2], params.Fixidity1)
}
