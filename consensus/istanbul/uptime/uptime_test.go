package uptime

import (
	"math/big"
	"reflect"
	"testing"
)

func TestGetUptimeMonitoringWindow(t *testing.T) {
	type args struct {
		epochNumber        uint64
		epochSize          uint64
		lookbackWindowSize uint64
	}
	tests := []struct {
		name      string
		args      args
		wantStart uint64
		wantEnd   uint64
	}{
		{"monitoringWindow on first epoch", args{1, 10, 2}, 2, 8},
		{"monitoringWindow on second epoch", args{2, 10, 2}, 12, 18},
		{"lookback window too big", args{1, 10, 10}, 10, 8},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := MonitoringWindow(tt.args.epochNumber, tt.args.epochSize, tt.args.lookbackWindowSize)
			if w.Start != tt.wantStart {
				t.Errorf("MonitoringWindow() got = %v, wantStart %v", w.Start, tt.wantStart)
			}
			if w.End != tt.wantEnd {
				t.Errorf("MonitoringWindow() got1 = %v, wantEnd %v", w.End, tt.wantEnd)
			}
		})
	}
}

func TestUptime(t *testing.T) {
	var uptimes *Uptime
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
	monitoringWindow := MonitoringWindow(1, 10, 2) // [2,8]

	for _, bitmap := range bitmaps {
		// these tests to increase our confidence
		uptimes = updateUptime(uptimes, block, bitmap, 2, monitoringWindow)
		block++
	}

	expected := &Uptime{
		LatestBlock: 0,
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
			{
				UpBlocks:        0,
				LastSignedBlock: 0,
			},
			{
				UpBlocks:        0,
				LastSignedBlock: 0,
			},
			{
				UpBlocks:        0,
				LastSignedBlock: 0,
			},
		},
	}
	if !reflect.DeepEqual(uptimes, expected) {
		t.Fatalf("uptimes were not updated correctly, got %v, expected %v", uptimes, expected)
	}
}

func TestUptimeSingle(t *testing.T) {
	var uptimes *Uptime
	monitoringWindow := MonitoringWindow(2, 211, 3)
	uptimes = updateUptime(uptimes, 211, big.NewInt(7), 3, monitoringWindow)
	// the first 2 uptime updates do not get scored since they're within the
	// first window after the epoch block
	expected := &Uptime{
		LatestBlock: 0,
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
			// plus 2 dummies due to the *2
			{
				UpBlocks:        0,
				LastSignedBlock: 0,
			},
			{
				UpBlocks:        0,
				LastSignedBlock: 0,
			},
			{
				UpBlocks:        0,
				LastSignedBlock: 0,
			},
		},
	}
	if !reflect.DeepEqual(uptimes, expected) {
		t.Fatalf("uptimes were not updated correctly, got %v, expected %v", uptimes, expected)
	}
}
