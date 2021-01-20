package uptime

import (
	"errors"
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
		name  string
		args  args
		want  Window
		fails bool
	}{
		{"monitoringWindow on first epoch", args{1, 10, 2}, Window{2, 8}, false},
		{"monitoringWindow on second epoch", args{2, 10, 2}, Window{12, 18}, false},
		{"largest possible lookback window", args{1, 10, 8}, Window{8, 8}, false},
		{"fails when epochSize is too small", args{1, istanbul.MinEpochSize - 1, 1}, Window{0, 0}, true},
		{"fails for epoch 0 (genesis)", args{0, 10, 2}, Window{2, 8}, true},
		{"fails when lookbackWindow too big for current epoch size", args{1, 10, 9}, Window{2, 8}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MonitoringWindow(tt.args.epochNumber, tt.args.epochSize, tt.args.lookbackWindowSize)

			if err == nil && tt.fails {
				t.Errorf("MonitoringWindow() got %v, expected to fail", got)
			} else if err != nil && !tt.fails {
				t.Errorf("MonitoringWindow() panicked, wanted: %v", tt.want)
			} else if err == nil {
				if got.Start != tt.want.Start {
					t.Errorf("MonitoringWindow().Start got = %v, want = %v", got.Start, tt.want.Start)
				}
				if got.End != tt.want.End {
					t.Errorf("MonitoringWindow().End got = %v, want = %v", got.End, tt.want.End)
				}
			}
		})
	}
}

func TestComputeLookbackWindow(t *testing.T) {
	constant := func(value uint64) func() (uint64, error) {
		return func() (uint64, error) { return value, nil }
	}

	type args struct {
		epochSize             uint64
		defaultLookbackWindow uint64
		cip21                 bool
		getLookbackWindow     func() (uint64, error)
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{"returns default if Donut is not active", args{100, 20, false, constant(24)}, 20},
		{"returns default if call fails", args{100, 20, true, func() (uint64, error) { return 10, errors.New("some error") }}, 20},
		{"returns safe minimum if configured is below", args{100, 20, true, constant(2)}, MinSafeLookbackWindow},
		{"returns safe maximum if configured is above", args{1000, 20, true, constant(800)}, MaxSafeLookbackWindow},
		{"returns epochSize-2 if configured is above", args{100, 20, true, constant(99)}, 98},
		{"mainet config before donut activation", args{17280, 12, false, constant(64)}, 12},
		{"mainet config after donut activation", args{17280, 12, true, constant(64)}, 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeLookbackWindow(tt.args.epochSize, tt.args.defaultLookbackWindow, tt.args.cip21, tt.args.getLookbackWindow)
			if got != tt.want {
				t.Errorf("ComputeLookbackWindow() = %v, want %v", got, tt.want)
			}
		})
	}
}
