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
	epochSize      uint64
	epoch          uint64
	lookbackWindow uint64
	valSetSize     int
	BitmapsBuffer  []*big.Int

	LatestBlock uint64
	UpBlocks    []uint64

	logger log.Logger
}

// NewMonitor creates a new uptime monitor
func NewMonitoraux(epochSize, epoch, lookbackWindow uint64, valSetSize int) *Monitor2 {
	window := MustMonitoringWindow(epoch, epochSize, lookbackWindow)

	return &Monitor2{
		epochSize:      epochSize,
		epoch:          epoch,
		lookbackWindow: lookbackWindow,
		valSetSize:     valSetSize,
		BitmapsBuffer:  make([]*big.Int, 0),

		// The epoch's first block's aggregated parent signatures is for the previous epoch's valset.
		LatestBlock: window.Start,
		UpBlocks:    make([]uint64, valSetSize),
		logger:      log.New("module", "uptime-monitor"),
	}
}

// MonitoringWindow returns the monitoring window for the given epoch in the format
// [firstBlock, lastBlock] both inclusive
func (um *Monitor2) MonitoringWindow2() Window {
	return MustMonitoringWindow(um.epoch, um.epochSize, um.lookbackWindow)
}

// ComputeValidatorsUptime retrieves the uptime score for each validator for a given epoch
func (um *Monitor2) ComputeValidatorsUptime2() ([]*big.Int, error) {
	logger := um.logger.New("func", "Backend.updateValidatorScores", "epoch", um.epoch)
	logger.Trace("Updating validator scores")

	// The totalMonitoredBlocks are the total number of block on which we monitor uptime for the epoch
	totalMonitoredBlocks := um.epochSize

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
		um.updateUptime2(signedValidatorsBitmap)
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
	bitmapAccum := um.BitmapsBuffer[0]
	for i := 1; i < int(um.lookbackWindow); i++ {
		bitmapAccum.Or(bitmapAccum, um.BitmapsBuffer[i])
	}

	one := big.NewInt(1)
	for i := 0; i < len(um.UpBlocks); i++ {
		if big.NewInt(0).And(bitmapAccum, one) == one {
			// validator signature present => update their latest signed block
			um.UpBlocks[i]++
		}
		bitmapAccum.Rsh(bitmapAccum, 1)
	}
}

// // https://stackoverflow.com/questions/19105791/is-there-a-big-bitcount/32702348#32702348
// func bitCount(n *big.Int) int {
// 	count := 0
// 	for _, v := range n.Bits() {
// 		count += popcount(uint64(v))
// 	}
// 	return count
// }

// // Straight and simple C to Go translation from https://en.wikipedia.org/wiki/Hamming_weight
// func popcount(x uint64) int {
// 	const (
// 		m1  = 0x5555555555555555 //binary: 0101...
// 		m2  = 0x3333333333333333 //binary: 00110011..
// 		m4  = 0x0f0f0f0f0f0f0f0f //binary:  4 zeros,  4 ones ...
// 		h01 = 0x0101010101010101 //the sum of 256 to the power of 0,1,2,3...
// 	)
// 	x -= (x >> 1) & m1             //put count of each 2 bits into those 2 bits
// 	x = (x & m2) + ((x >> 2) & m2) //put count of each 4 bits into those 4 bits
// 	x = (x + (x >> 4)) & m4        //put count of each 8 bits into those 8 bits
// 	return int((x * h01) >> 56)    //returns left 8 bits of x + (x<<8) + (x<<16) + (x<<24) + ...
// }
