package uptime

import (
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/contract_comm/blockchain_parameters"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// Check CIP-21 Spec (https://github.com/celo-org/celo-proposals/blob/master/CIPs/cip-0021.md)
const (
	// MinSafeLookbackWindow is the minimum number allowed for lookbackWindow size
	MinSafeLookbackWindow = 12
	// MaxSafeLookbackWindow is the maximum number allowed for lookbackWindow size
	MaxSafeLookbackWindow = 720
	// BlocksToSkipAtEpochEnd represents the number of blocks to skip on the monitoring window from the end of the epoch
	// Currently we skip blocks:
	// lastBlock     => its parentSeal is on firstBlock of next epoch
	// lastBlock - 1 => parentSeal is on lastBlockOfEpoch, but validatorScore is computed with lastBlockOfEpoch and before updating scores
	// (lastBlock-1 could be counted, but much harder to implement)
	BlocksToSkipAtEpochEnd = 2
)

// LookbackWindow returns the size of the lookback window for calculating uptime (in blocks)
func LookbackWindow(config *params.ChainConfig, istConfig *istanbul.Config, header *types.Header, state *state.StateDB) (uint64, error) {
	if !config.IsDonut(header.Number) {
		return istConfig.DefaultLookbackWindow, nil
	}

	if istConfig.Epoch <= istanbul.MinEpochSize {
		panic("Invalid epoch value")
	}

	value, err := blockchain_parameters.GetLookbackWindow(header, state)
	if err != nil {
		value = MinSafeLookbackWindow
	}

	// Adjust to safe range
	if value < MinSafeLookbackWindow {
		value = MinSafeLookbackWindow
	} else if value > MaxSafeLookbackWindow {
		value = MaxSafeLookbackWindow
	}

	// Ensure it's sensible to given chain params
	if value > (istConfig.Epoch - BlocksToSkipAtEpochEnd) {
		value = istConfig.Epoch - BlocksToSkipAtEpochEnd
	}

	return value, nil
}

// MonitoringWindow retrieves the block window where uptime is to be monitored
// for a given epoch.
func MonitoringWindow(epochNumber uint64, epochSize uint64, lookbackWindowSize uint64) Window {
	if epochNumber == 0 {
		panic("no monitoring window for epoch 0")
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
	}
}
