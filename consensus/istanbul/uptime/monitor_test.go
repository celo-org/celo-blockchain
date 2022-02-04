package uptime

import (
	"math/big"
	"reflect"
	"testing"
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
