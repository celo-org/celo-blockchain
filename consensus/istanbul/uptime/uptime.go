package uptime

import (
	"errors"
	"fmt"
	"math"

	"math/big"

	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

// Store provides a persistent storage for uptime entries
type Store interface {
	ReadAccumulatedEpochUptime(epoch uint64) *Uptime
	WriteAccumulatedEpochUptime(epoch uint64, uptime *Uptime)
}

// Uptime contains the latest block for which uptime metrics were accounted for. It also contains
// an array of Entries where the `i`th entry represents the uptime statistics of the `i`th validator
// in the validator set for that epoch
type Uptime struct {
	LatestBlock uint64
	Entries     []UptimeEntry
}

// UptimeEntry contains the uptime score of a validator during an epoch as well as the
// last block they signed on
type UptimeEntry struct {
	ScoreTally      uint64
	LastSignedBlock uint64
}

func (u *UptimeEntry) String() string {
	return fmt.Sprintf("UptimeEntry { scoreTally: %v, lastBlock: %v}", u.ScoreTally, u.LastSignedBlock)
}

// GetUptimeMonitoringWindow retrieves the range [first block, last block] where uptime is to be monitored
// for a give epoch. The range is inclusive.
// First blocks of an epoch need to be skipped since we can't assess the last `lookbackWindow` block for validators
// as those are froma different epoch.
// Similarly, last block of epoch is skipped since we can't obtaine the signer for it; as they are in the next block
func GetUptimeMonitoringWindow(epochNumber uint64, epochSize uint64, lookbackWindowSize uint64) (uint64, uint64) {
	if epochNumber == 0 {
		panic("no monitoring window for epoch 0")
	}

	epochFirstBlock, _ := istanbul.GetEpochFirstBlockNumber(epochNumber, epochSize)
	epochLastBlock := istanbul.GetEpochLastBlockNumber(epochNumber, epochSize)

	// first block to monitor:
	// We need to wait for the completion of the first window with the start window's block being the
	// 2nd block of the epoch, before we start tallying the validator score for epoch "epochNumber".
	// We can't include the epoch's first block since it's aggregated parent seals
	// is for the previous epoch's valset.
	firstBlockToMonitor := epochFirstBlock + 1 + (lookbackWindowSize - 1)

	// last block to monitor:
	// We stop tallying for epoch "epochNumber" at the second to last block of that epoch.
	// We can't include that epoch's last block as part of the tally because the epoch val score is calculated
	// using a tally that is updated AFTER a block is finalized.
	// Note that it's possible to count up to the last block of the epoch, but it's much harder to implement
	// than couting up to the second to last one.
	lastBlockToMonitor := epochLastBlock - 1

	return firstBlockToMonitor, lastBlockToMonitor
}

// Monitor is responsible for monitoring uptime
// by processing blocks
type Monitor struct {
	epochSize      uint64
	lookbackWindow uint64

	logger log.Logger
	store  Store
}

// NewMonitor creates a new uptime monitor
func NewMonitor(store Store, epochSize, lookbackWindow uint64) *Monitor {
	return &Monitor{
		epochSize:      epochSize,
		lookbackWindow: lookbackWindow,
		store:          store,
		logger:         log.New("module", "uptime-monitor"),
	}
}

// MonitoringWindow returns the monitoring window for the given epoch in the format
// [firstBlock, lastBlock] both inclusive
func (um *Monitor) MonitoringWindow(epoch uint64) (uint64, uint64) {
	return GetUptimeMonitoringWindow(epoch, um.epochSize, um.lookbackWindow)
}

// MonitoringWindowSize returns the size (in blocks) of the monitoring window
func (um *Monitor) MonitoringWindowSize(epoch uint64) uint64 {
	start, end := um.MonitoringWindow(epoch)
	return end - start + 1
}

// ComputeValidatorsUptime retrieves the uptime score for each validator for a given epoch
func (um *Monitor) ComputeValidatorsUptime(epoch uint64, valSetSize int) ([]*big.Int, error) {
	logger := um.logger.New("func", "Backend.updateValidatorScores", "epoch", epoch)
	logger.Trace("Updating validator scores")

	// The denominator is the (last block - first block + 1) of the val score tally window
	denominator := um.MonitoringWindowSize(epoch)

	uptimes := make([]*big.Int, 0, valSetSize)
	accumulated := um.store.ReadAccumulatedEpochUptime(epoch)

	if accumulated == nil {
		err := errors.New("Accumulated uptimes not found, cannot update validator scores")
		logger.Error(err.Error())
		return nil, err
	}

	for i, entry := range accumulated.Entries {
		if i >= valSetSize {
			break
		}

		if entry.ScoreTally > denominator {
			logger.Error("ScoreTally exceeds max possible", "scoreTally", entry.ScoreTally, "denominator", denominator, "valIdx", i)
			uptimes = append(uptimes, params.Fixidity1)
			continue
		}

		numerator := big.NewInt(0).Mul(big.NewInt(int64(entry.ScoreTally)), params.Fixidity1)
		uptimes = append(uptimes, big.NewInt(0).Div(numerator, big.NewInt(int64(denominator))))
	}

	if len(uptimes) < valSetSize {
		err := fmt.Errorf("%d accumulated uptimes found, cannot update validator scores", len(uptimes))
		logger.Error(err.Error())
		return nil, err
	}

	return uptimes, nil
}

// ProcessBlock will monitor uptimeness information on the give block and add it to current epoch tally
func (um *Monitor) ProcessBlock(block *types.Block) error {
	// The epoch's first block's aggregated parent signatures is for the previous epoch's valset.
	// We can ignore updating the tally for that block.
	if istanbul.IsFirstBlockOfEpoch(block.NumberU64(), um.epochSize) {
		return nil
	}

	// Get the bitmap from the previous block
	extra, err := types.ExtractIstanbulExtra(block.Header())
	if err != nil {
		um.logger.Error("Unable to extract istanbul extra", "func", "ProcessBlock", "blocknum", block.NumberU64())
		return errors.New("could not extract block header extra")
	}
	signedValidatorsBitmap := extra.ParentAggregatedSeal.Bitmap

	// Get the uptime scores
	epochNum := istanbul.GetEpochNumber(block.NumberU64(), um.epochSize)
	uptime := um.store.ReadAccumulatedEpochUptime(epochNum)

	// We only update the uptime for blocks which are greater than the last block we saw.
	// This ensures that we do not count the same block twice for any reason.
	if uptime == nil || uptime.LatestBlock < block.NumberU64() {
		uptime = updateUptime(uptime, block.NumberU64(), signedValidatorsBitmap, um.lookbackWindow, epochNum, um.epochSize)
		um.store.WriteAccumulatedEpochUptime(epochNum, uptime)
	} else {
		log.Trace("WritingBlockWithState with block number less than a block we previously wrote", "latestUptimeBlock", uptime.LatestBlock, "blockNumber", block.NumberU64())
	}

	return nil
}

// updateUptime receives the currently accumulated uptime for all validators in an epoch and proceeds to update it
// based on which validators signed on the provided block by inspecting the block's parent bitmap
//
// It expects a blockNumber with the signatures bitmap from the previous block (parent seal)
func updateUptime(uptime *Uptime, blockNumber uint64, bitmap *big.Int, window uint64, epochNum uint64, epochSize uint64) *Uptime {
	if uptime == nil {
		uptime = new(Uptime)
		// The number of validators is upper bounded by 3/2 of the number of 1s in the bitmap
		// We multiply by 2 just to be extra cautious of off-by-one errors.
		validatorsSizeUpperBound := uint64(math.Ceil(float64(bitCount(bitmap)) * 2))
		uptime.Entries = make([]UptimeEntry, validatorsSizeUpperBound)
	}

	firstBlockToMonitor, lastBlockToMonitor := GetUptimeMonitoringWindow(epochNum, epochSize, window)

	// lookbackBlockNum := (blockNumber -1) - window

	// signedBlockWindowLastBlockNum is just the previous block
	signedBlockWindowLastBlockNum := blockNumber - 1
	signedBlockWindowFirstBlockNum := signedBlockWindowLastBlockNum - (window - 1)

	uptime.LatestBlock = blockNumber
	for i := 0; i < len(uptime.Entries); i++ {
		if bitmap.Bit(i) == 1 {
			// update their latest signed block
			uptime.Entries[i].LastSignedBlock = blockNumber - 1
		}

		// If we are within the validator uptime tally window, then update the validator's score
		// if its last signed block is within the lookback window
		if blockNumber >= firstBlockToMonitor && blockNumber <= lastBlockToMonitor {
			lastSignedBlock := uptime.Entries[i].LastSignedBlock

			// Note that the second condition in the if condition is not necessary.  But it does
			// make the logic easier to understand.  (e.g. it's checking is lastSignedBlock is within
			// the range [signedBlockWindowFirstBlockNum, signedBlockWindowLastBlockNum])
			if lastSignedBlock >= signedBlockWindowFirstBlockNum && lastSignedBlock <= signedBlockWindowLastBlockNum {
				uptime.Entries[i].ScoreTally++
			}
		}
	}
	return uptime
}

// https://stackoverflow.com/questions/19105791/is-there-a-big-bitcount/32702348#32702348
func bitCount(n *big.Int) int {
	count := 0
	for _, v := range n.Bits() {
		count += popcount(uint64(v))
	}
	return count
}

// Straight and simple C to Go translation from https://en.wikipedia.org/wiki/Hamming_weight
func popcount(x uint64) int {
	const (
		m1  = 0x5555555555555555 //binary: 0101...
		m2  = 0x3333333333333333 //binary: 00110011..
		m4  = 0x0f0f0f0f0f0f0f0f //binary:  4 zeros,  4 ones ...
		h01 = 0x0101010101010101 //the sum of 256 to the power of 0,1,2,3...
	)
	x -= (x >> 1) & m1             //put count of each 2 bits into those 2 bits
	x = (x & m2) + ((x >> 2) & m2) //put count of each 4 bits into those 4 bits
	x = (x + (x >> 4)) & m4        //put count of each 8 bits into those 8 bits
	return int((x * h01) >> 56)    //returns left 8 bits of x + (x<<8) + (x<<16) + (x<<24) + ...
}
