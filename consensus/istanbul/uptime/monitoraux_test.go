package uptime

import (
	"math/big"
	"reflect"
	"testing"
)

func TestUptime2(t *testing.T) {
	// (there can't be less than 2/3rds of validators sigs in a valid bitmap)
	bitmaps := []*big.Int{
		big.NewInt(7), // 111     // signature bitmap for block #1
		big.NewInt(5), // 101
		big.NewInt(3), // 011
		big.NewInt(5), // 101
		big.NewInt(7), // 111
		big.NewInt(5), // 101     // signature bitmap for block #6
	}

	// use a window of 2 blocks - ideally we want to expand
	monitor := NewMonitoraux(10, 1, 2, 3)

	for _, bitmap := range bitmaps {
		// these tests to increase our confidence
		monitor.updateUptime2(bitmap)
	}

	expected := make([]uint64, 3)
	expected[0] = 5
	expected[1] = 5
	expected[2] = 5

	if !reflect.DeepEqual(monitor.UpBlocks, expected) {
		t.Fatalf("uptimes were not updated correctly, got %v, expected %v", monitor.UpBlocks, expected)
	}
}

func TestUptime2Big(t *testing.T) {
	// (there can't be less than 2/3rds of validators sigs in a valid bitmap)
	bitmaps := []*big.Int{
		big.NewInt(7), // 111     // signature bitmap for block #1
		big.NewInt(1), // 001
		big.NewInt(2), // 010
		big.NewInt(0), // 000
		big.NewInt(7), // 111
		big.NewInt(5), // 101     // signature bitmap for block #6
	}

	// use a window of 2 blocks - ideally we want to expand
	monitor := NewMonitoraux(10, 1, 2, 3)

	for _, bitmap := range bitmaps {
		// these tests to increase our confidence
		monitor.updateUptime2(bitmap)
	}

	expected := make([]uint64, 3)
	expected[0] = 4
	expected[1] = 5
	expected[2] = 3

	if !reflect.DeepEqual(monitor.UpBlocks, expected) {
		t.Fatalf("uptimes were not updated correctly, got %v, expected %v", monitor.UpBlocks, expected)
	}
}

// func TestUptimeSingle2(t *testing.T) {
// 	var uptimes *Uptime
// 	monitoringWindow := MustMonitoringWindow(2, 211, 3)
// 	uptimes = updateUptime(uptimes, 211, big.NewInt(7), 3, monitoringWindow)
// 	// the first 2 uptime updates do not get scored since they're within the
// 	// first window after the epoch block
// 	expected := &Uptime{
// 		LatestBlock: 0,
// 		Entries: []UptimeEntry{
// 			{
// 				UpBlocks:        0,
// 				LastSignedBlock: 211,
// 			},
// 			{
// 				UpBlocks:        0,
// 				LastSignedBlock: 211,
// 			},
// 			{
// 				UpBlocks:        0,
// 				LastSignedBlock: 211,
// 			},
// 			// plus 2 dummies due to the *2
// 			{
// 				UpBlocks:        0,
// 				LastSignedBlock: 0,
// 			},
// 			{
// 				UpBlocks:        0,
// 				LastSignedBlock: 0,
// 			},
// 			{
// 				UpBlocks:        0,
// 				LastSignedBlock: 0,
// 			},
// 		},
// 	}
// 	if !reflect.DeepEqual(uptimes, expected) {
// 		t.Fatalf("uptimes were not updated correctly, got %v, expected %v", uptimes, expected)
// 	}
// }
