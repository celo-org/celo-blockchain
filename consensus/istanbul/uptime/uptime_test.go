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
		name  string
		args  args
		want  uint64
		want1 uint64
	}{
		{"tally on first epoch", args{1, 10, 2}, 1 + 2, 9},
		{"tally on second epoch", args{2, 10, 2}, 11 + 2, 19},
		{"lookback window too big", args{1, 10, 10}, 11, 9},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := GetUptimeMonitoringWindow(tt.args.epochNumber, tt.args.epochSize, tt.args.lookbackWindowSize)
			if got != tt.want {
				t.Errorf("GetUptimeMonitoringWindow() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("GetUptimeMonitoringWindow() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestUptime(t *testing.T) {
	var uptimes *Uptime
	// (there can't be less than 2/3rds of validators sigs in a valid bitmap)
	bitmaps := []*big.Int{
		big.NewInt(3), // 011     // Parent aggregated seal for block #1
		big.NewInt(7), // 111
		big.NewInt(5), // 101
		big.NewInt(3), // 011
		big.NewInt(5), // 101
		big.NewInt(7), // 111
		big.NewInt(5), // 101     // Parent aggregated seal for block #7
	}
	// assume the first block is the first epoch's block (ie not the genesis)
	block := uint64(1)
	for _, bitmap := range bitmaps {
		// use a window of 2 blocks - ideally we want to expand
		// these tests to increase our confidence
		uptimes = updateUptime(uptimes, block, bitmap, 2, 1, 10)
		block++
	}

	expected := &Uptime{
		LatestBlock: 7,
		Entries: []UptimeEntry{
			{
				ScoreTally:      5,
				LastSignedBlock: 6,
			},
			{
				ScoreTally:      5,
				LastSignedBlock: 5,
			},
			{
				ScoreTally:      5,
				LastSignedBlock: 6,
			},
			{
				ScoreTally:      0,
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
	uptimes = updateUptime(uptimes, 212, big.NewInt(7), 3, 2, 211)
	// the first 2 uptime updates do not get scored since they're within the
	// first window after the epoch block
	expected := &Uptime{
		LatestBlock: 212,
		Entries: []UptimeEntry{
			{
				ScoreTally:      0,
				LastSignedBlock: 211,
			},
			{
				ScoreTally:      0,
				LastSignedBlock: 211,
			},
			{
				ScoreTally:      0,
				LastSignedBlock: 211,
			},
			// plus 2 dummies due to the *2
			{
				ScoreTally:      0,
				LastSignedBlock: 0,
			},
			{
				ScoreTally:      0,
				LastSignedBlock: 0,
			},
			{
				ScoreTally:      0,
				LastSignedBlock: 0,
			},
		},
	}
	if !reflect.DeepEqual(uptimes, expected) {
		t.Fatalf("uptimes were not updated correctly, got %v, expected %v", uptimes, expected)
	}
}
