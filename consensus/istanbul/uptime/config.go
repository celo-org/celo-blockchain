package uptime

import (
	"errors"
	"fmt"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

// Check CIP-21 Spec (https://github.com/celo-org/celo-proposals/blob/master/CIPs/cip-0021.md)
const (
	// MinSafeLookbackWindow is the minimum number allowed for lookbackWindow size
	MinSafeLookbackWindow = 3
	// MaxSafeLookbackWindow is the maximum number allowed for lookbackWindow size
	MaxSafeLookbackWindow = 720
	// BlocksToSkipAtEpochEnd represents the number of blocks to skip on the monitoring window from the end of the epoch
	// Currently we skip blocks:
	// lastBlock     => its parentSeal is on firstBlock of next epoch
	// lastBlock - 1 => parentSeal is on lastBlockOfEpoch, but validatorScore is computed with lastBlockOfEpoch and before updating scores
	// (lastBlock-1 could be counted, but much harder to implement)
	BlocksToSkipAtEpochEnd = 2
)

// ComputeLookbackWindow computes the lookbackWindow based on different required parameters.
// getLookbackWindow represents the way to obtain lookbackWindow from the smart contract
func ComputeLookbackWindow(epochSize uint64, defaultLookbackWindow uint64, cip21 bool, getLookbackWindow func() (uint64, error)) uint64 {
	if !cip21 {
		return defaultLookbackWindow
	}

	if epochSize <= istanbul.MinEpochSize {
		panic("Invalid epoch value")
	}

	value, err := getLookbackWindow()
	if err != nil {
		// It can fail because smart contract it's not present or it's not initialized
		// in both cases, we use the old value => defaultLookbackWindow
		value = defaultLookbackWindow
	}

	// Adjust to safe range
	if value < MinSafeLookbackWindow {
		value = MinSafeLookbackWindow
	} else if value > MaxSafeLookbackWindow {
		value = MaxSafeLookbackWindow
	}

	// Ensure it's sensible to given chain params
	if value > (epochSize - BlocksToSkipAtEpochEnd) {
		value = epochSize - BlocksToSkipAtEpochEnd
	}

	return value
}

// MustMonitoringWindow is a MonitoringWindow variant that panics on error
func MustMonitoringWindow(epochNumber uint64, epochSize uint64, lookbackWindowSize uint64) Window {
	w, err := MonitoringWindow(epochNumber, epochSize, lookbackWindowSize)
	if err != nil {
		panic(err)
	}
	return w
}

// MonitoringWindow retrieves the block window where uptime is to be monitored
// for a given epoch.
func MonitoringWindow(epochNumber uint64, epochSize uint64, lookbackWindowSize uint64) (Window, error) {
	if epochNumber == 0 {
		return Window{}, errors.New("no monitoring window for epoch 0")
	}
	if epochSize < istanbul.MinEpochSize {
		return Window{}, errors.New("invalid epoch value")
	}
	if epochSize < lookbackWindowSize+BlocksToSkipAtEpochEnd {
		return Window{}, fmt.Errorf("lookbackWindow (%d) too big for epochSize (%d)", lookbackWindowSize, epochSize)
	}

	epochFirstBlock, _ := istanbul.GetEpochFirstBlockNumber(epochNumber, epochSize)
	epochLastBlock := istanbul.GetEpochLastBlockNumber(epochNumber, epochSize)

	// first block to monitor:
	// we can't monitor uptime when current lookbackWindow crosses the epoch boundary
	// thus, first block to monitor is the final block of the lookbackwindow that starts at firstBlockOfEpoch
	firstBlockToMonitor := newWindowStartingAt(epochFirstBlock, lookbackWindowSize).End

	return Window{
		Start: firstBlockToMonitor,
		End:   epochLastBlock - BlocksToSkipAtEpochEnd,
	}, nil
}

// MonitoringWindow retrieves the block window where uptime is to be monitored
// for a given epoch.
func MonitoringWindowUntil(epochNumber uint64, epochSize uint64, lookbackWindowSize uint64, untilBlockNumber uint64) (Window, error) {
	w, err := MonitoringWindow(epochNumber, epochSize, lookbackWindowSize)
	if err != nil {
		return Window{}, err
	}
	if w.Start > untilBlockNumber {
		return Window{}, errors.New("header older than epoch start")
	}
	if w.End > untilBlockNumber {
		w.End = untilBlockNumber
	}
	return w, nil
}
