package uptime

import (
	"testing"

	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

func TestEpochSizeIsConsistentWithSkippedBlock(t *testing.T) {
	if istanbul.MinEpochSize <= BlocksToSkipAtEpochEnd {
		t.Fatalf("Constant MinEpochSize MUST BE greater than BlocksToSkipAtEpochEnd (%d, %d) ", istanbul.MinEpochSize, BlocksToSkipAtEpochEnd)
	}
}

func TestMonitoringWindow(t *testing.T) {
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
