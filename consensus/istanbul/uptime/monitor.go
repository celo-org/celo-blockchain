package uptime

import (
	"errors"
	"fmt"
	"math"

	"math/big"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
)

// Store provides a persistent storage for uptime entries
type Store interface {
	ReadAccumulatedEpochUptime(epoch uint64) *Uptime
	WriteAccumulatedEpochUptime(epoch uint64, uptime *Uptime)
}

// Uptime contains the latest block for which uptime metrics were accounted. It also contains
// an array of Entries where the `i`th entry represents the uptime statistics of the `i`th validator
// in the validator set for that epoch
type Uptime struct {
	LatestBlock uint64
	Entries     []UptimeEntry
}

// UptimeEntry contains the uptime score of a validator during an epoch as well as the
// last block they signed on
type UptimeEntry struct {
	// Numbers of blocks validator is considered UP within monitored window
	UpBlocks        uint64
	LastSignedBlock uint64
}

func (u *UptimeEntry) String() string {
	return fmt.Sprintf("UptimeEntry { upBlocks: %v, lastBlock: %v}", u.UpBlocks, u.LastSignedBlock)
}

// Monitor is responsible for monitoring uptime by processing blocks
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
func (um *Monitor) MonitoringWindow(epoch uint64) Window {
	return MustMonitoringWindow(epoch, um.epochSize, um.lookbackWindow)
}

// ComputeValidatorsUptime retrieves the uptime score for each validator for a given epoch
func (um *Monitor) ComputeValidatorsUptime(epoch uint64, valSetSize int) ([]*big.Int, error) {
	logger := um.logger.New("func", "Backend.updateValidatorScores", "epoch", epoch)
	logger.Trace("Updating validator scores")

	// The totalMonitoredBlocks are the total number of block on which we monitor uptime for the epoch
	totalMonitoredBlocks := um.MonitoringWindow(epoch).Size()

	uptimes := make([]*big.Int, 0, valSetSize)
	accumulated := um.store.ReadAccumulatedEpochUptime(epoch)

	if accumulated == nil {
		err := errors.New("accumulated uptimes not found, cannot update validator scores")
		logger.Error(err.Error())
		return nil, err
	}

	for i, entry := range accumulated.Entries {
		if i >= valSetSize {
			break
		}

		if entry.UpBlocks > totalMonitoredBlocks {
			logger.Error("UpBlocks exceeds max possible", "upBlocks", entry.UpBlocks, "totalMonitoredBlocks", totalMonitoredBlocks, "valIdx", i)
			uptimes = append(uptimes, params.Fixidity1)
			continue
		}

		numerator := big.NewInt(0).Mul(big.NewInt(int64(entry.UpBlocks)), params.Fixidity1)
		uptimes = append(uptimes, big.NewInt(0).Div(numerator, big.NewInt(int64(totalMonitoredBlocks))))
	}

	if len(uptimes) < valSetSize {
		err := fmt.Errorf("%d accumulated uptimes found, cannot update validator scores", len(uptimes))
		logger.Error(err.Error())
		return nil, err
	}

	return uptimes, nil
}

// ReprocessEpochUpTo
func (um *Monitor) ReprocessEpochUpTo(headers []*types.Header) error {
	if len(headers) == 0 {
		return errors.New("empty headers array")
	}

	epochNum := istanbul.GetEpochNumber(headers[0].Number.Uint64(), um.epochSize)
	monitoringWindow := um.MonitoringWindow(epochNum)

	// Check that all headers must be of the same epoch
	if lastEpochNum := istanbul.GetEpochNumber(headers[len(headers)-1].Number.Uint64(), um.epochSize); epochNum != lastEpochNum {
		return errors.New("invalid arguments: headers in array are not from the same epoch")
	}

	// Check that we will compute all headers since monitoringWindow.Start (inclusive)
	if monitoringWindow.Start < headers[0].Number.Uint64() {
		return errors.New("invalid arguments: first header must be same or ancestor of monitoring window start")
	}

	var uptime *Uptime
	prevBlockNumber := headers[0].Number.Uint64() - 1
	for _, header := range headers {
		blockNumber := header.Number.Uint64()

		// Check that headers are consecutive
		if prevBlockNumber+1 != header.Number.Uint64() {
			return errors.New("invalid arguments: headers are not consecutive")
		}

		// skip blocks that are not part of the monitoring window
		if !monitoringWindow.Contains(blockNumber) {
			continue
		}

		// Get the bitmap from the previous block
		extra, err := types.ExtractIstanbulExtra(header)
		if err != nil {
			um.logger.Error("Unable to extract istanbul extra", "func", "ProcessBlock", "blocknum", blockNumber)
			return errors.New("could not extract block header extra")
		}
		signedValidatorsBitmap := extra.ParentAggregatedSeal.Bitmap

		uptime = updateUptime(uptime, blockNumber, signedValidatorsBitmap, um.lookbackWindow, monitoringWindow)
		uptime.LatestBlock = blockNumber
	}

	um.store.WriteAccumulatedEpochUptime(epochNum, uptime)

	return nil
}

// ProcessHeader uses the header's signature bitmap (which encodes who signed the parent block) to update the epoch's Uptime data
func (um *Monitor) ProcessHeader(header *types.Header) error {
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

	// Get the uptime scores
	epochNum := istanbul.GetEpochNumber(blockNumber, um.epochSize)
	uptime := um.store.ReadAccumulatedEpochUptime(epochNum)

	// We only update the uptime for blocks which are greater than the last block we saw.
	// This ensures that we do not count the same block twice for any reason.
	if uptime == nil || uptime.LatestBlock < blockNumber {
		uptime = updateUptime(uptime, blockNumber-1, signedValidatorsBitmap, um.lookbackWindow, um.MonitoringWindow(epochNum))
		uptime.LatestBlock = blockNumber
		um.store.WriteAccumulatedEpochUptime(epochNum, uptime)
	} else {
		log.Trace("WritingBlockWithState with block number less than a block we previously wrote", "latestUptimeBlock", uptime.LatestBlock, "blockNumber", blockNumber)
	}

	return nil
}

// updateUptime updates the accumulated uptime given a block and its validator's signatures bitmap
func updateUptime(uptime *Uptime, blockNumber uint64, bitmap *big.Int, lookbackWindowSize uint64, monitoringWindow Window) *Uptime {
	if uptime == nil {
		uptime = new(Uptime)
		// The number of validators is upper bounded by 3/2 of the number of 1s in the bitmap
		// We multiply by 2 just to be extra cautious of off-by-one errors.
		validatorsSizeUpperBound := uint64(math.Ceil(float64(bitCount(bitmap)) * 2))
		uptime.Entries = make([]UptimeEntry, validatorsSizeUpperBound)
	}

	// Obtain current lookback window
	currentLookbackWindow := newWindowEndingAt(blockNumber, lookbackWindowSize)

	for i := 0; i < len(uptime.Entries); i++ {
		if bitmap.Bit(i) == 1 {
			// validator signature present => update their latest signed block
			uptime.Entries[i].LastSignedBlock = blockNumber
		}

		// If block number is to be monitored, then check if lastSignedBlock is within current lookback window
		if monitoringWindow.Contains(blockNumber) && currentLookbackWindow.Contains(uptime.Entries[i].LastSignedBlock) {
			// since within currentLookbackWindow there's at least one signed block (lastSignedBlock) validator is considered UP
			uptime.Entries[i].UpBlocks++
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
