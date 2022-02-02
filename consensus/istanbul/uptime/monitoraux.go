package uptime

import (
	"errors"
	"fmt"

	"math/big"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
)

// // Store provides a persistent storage for uptime entries
// type Store interface {
// 	ReadAccumulatedEpochUptime(epoch uint64) *Uptime
// 	WriteAccumulatedEpochUptime(epoch uint64, uptime *Uptime)
// }

// Uptime contains the latest block for which uptime metrics were accounted. It also contains
// an array of Entries where the `i`th entry represents the uptime statistics of the `i`th validator
// in the validator set for that epoch
// type Uptime2 struct {
// 	LatestBlock   uint64
// 	BitmapsBuffer *big.Int
// 	UpBlocks      []uint64
// }

// func (u *UptimeEntry) String() string {
// 	return fmt.Sprintf("UptimeEntry { upBlocks: %v, lastBlock: %v}", u.UpBlocks, u.LastSignedBlock)
// }

// Monitor is responsible for monitoring uptime by processing blocks
type Monitor2 struct {
	epoch          uint64
	epochSize      uint64
	lookbackWindow uint64
	valSetSize     int
	BitmapsBuffer  []*big.Int
	window         Window

	LatestBlock uint64
	UpBlocks    []uint64

	logger log.Logger
}

// NewMonitor creates a new uptime monitor
func NewMonitoraux(epochSize, epoch, lookbackWindow uint64, valSetSize int) *Monitor2 {
	window := MustMonitoringWindow(epoch, epochSize, lookbackWindow)
	firstBlockOfEpoch, _ := istanbul.GetEpochFirstBlockNumber(epoch, epochSize)

	return &Monitor2{
		epoch:          epoch,
		epochSize:      epochSize,
		lookbackWindow: lookbackWindow,
		valSetSize:     valSetSize,
		BitmapsBuffer:  make([]*big.Int, 0),
		window:         window,

		// The epoch's first block's aggregated parent signatures is for the previous epoch's valset.
		LatestBlock: firstBlockOfEpoch,
		UpBlocks:    make([]uint64, valSetSize),
		logger:      log.New("module", "uptime-monitor"),
	}
}

// MonitoringWindow returns the monitoring window for the given epoch in the format
// [firstBlock, lastBlock] both inclusive
func (um *Monitor2) MonitoringWindow2() Window {
	return um.window
}

// ComputeValidatorsUptime retrieves the uptime score for each validator for a given epoch
func (um *Monitor2) ComputeValidatorsUptime2() ([]*big.Int, error) {
	logger := um.logger.New("func", "Backend.updateValidatorScores", "epoch", um.epoch)
	logger.Trace("Updating validator scores")

	// The totalMonitoredBlocks are the total number of block on which we monitor uptime for the epoch
	totalMonitoredBlocks := um.window.Size()

	uptimes := make([]*big.Int, 0, um.valSetSize)
	// accumulated := um.store.ReadAccumulatedEpochUptime(epoch)

	// if accumulated == nil {
	// 	err := errors.New("Accumulated uptimes not found, cannot update validator scores")
	// 	logger.Error(err.Error())
	// 	return nil, err
	// }

	for i, entry := range um.UpBlocks {
		if i >= um.valSetSize {
			break
		}

		if entry > totalMonitoredBlocks {
			logger.Error("UpBlocks exceeds max possible", "upBlocks", entry, "totalMonitoredBlocks", totalMonitoredBlocks, "valIdx", i)
			uptimes = append(uptimes, params.Fixidity1)
			continue
		}

		numerator := big.NewInt(0).Mul(big.NewInt(int64(entry)), params.Fixidity1)
		uptimes = append(uptimes, big.NewInt(0).Div(numerator, big.NewInt(int64(totalMonitoredBlocks))))
	}

	if len(uptimes) < um.valSetSize {
		err := fmt.Errorf("%d accumulated uptimes found, cannot update validator scores", len(uptimes))
		logger.Error(err.Error())
		return nil, err
	}

	return uptimes, nil
}

// ProcessBlock uses the block's signature bitmap (which encodes who signed the parent block) to update the epoch's Uptime data
func (um *Monitor2) ProcessHeader2(header *types.Header) error {
	blockNumber := header.Number.Uint64()

	// The epoch's first block's aggregated parent signatures is for the previous epoch's valset.
	// We can ignore updating the tally for that block.
	if istanbul.IsFirstBlockOfEpoch(blockNumber, um.epochSize) {
		return nil
	}

	// Get the bitmap from the previous block
	extra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		um.logger.Error("Unable to extract istanbul extra", "func", "ProcessBlock", "blocknum", blockNumber)
		return errors.New("could not extract block header extra")
	}
	signedValidatorsBitmap := extra.ParentAggregatedSeal.Bitmap

	// // Get the uptime scores
	// epochNum := istanbul.GetEpochNumber(blockNumber, um.epochSize)

	// We only update the uptime for blocks which are greater than the last block we saw.
	// This ensures that we do not count the same block twice for any reason.
	if um.LatestBlock < blockNumber {
		if blockNumber <= um.window.End+1 {
			um.updateUptime2(signedValidatorsBitmap)
		}
		um.LatestBlock = blockNumber
		// um.store.WriteAccumulatedEpochUptime(epochNum, uptime)
	} else {
		log.Trace("WritingBlockWithState with block number less than a block we previously wrote", "latestUptimeBlock", um.LatestBlock, "blockNumber", blockNumber)
	}

	return nil
}

// updateUptime updates the accumulated uptime given a block and its validator's signatures bitmap
func (um *Monitor2) updateUptime2(bitmap *big.Int) {
	um.BitmapsBuffer = append(um.BitmapsBuffer, bitmap)

	if len(um.BitmapsBuffer) > int(um.lookbackWindow) {
		// Maintain the last lookbackWindow blocks
		um.BitmapsBuffer = um.BitmapsBuffer[1:]
	}
	if len(um.BitmapsBuffer) < int(um.lookbackWindow) {
		// We need at least lookbackWindows blocks from the beginning to start the score
		return
	}
	bitmapAccum := new(big.Int).Set(um.BitmapsBuffer[0])
	for i := 1; i < int(um.lookbackWindow); i++ {
		bitmapAccum = bitmapAccum.Or(bitmapAccum, um.BitmapsBuffer[i])
	}

	words := bitmapAccum.Bits()
	for i := 0; i < len(um.UpBlocks); i++ {
		wordPos := i / 64
		if wordPos < len(words) && words[wordPos]&1 == 1 {
			// validator signature present => update their latest signed block
			um.UpBlocks[i]++
		}
		words[wordPos] = words[wordPos] >> 1
	}
}
