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

	// last block to monitor:
	// Last 2 blocks from the epoch are removed from the window
	// lastBlock     => its parentSeal is on firstBlock of next epoch
	// lastBlock - 1 => parentSeal is on lastBlockOfEpoch, but validatorScore is computed with lastBlockOfEpoch and before updating scores
	// (lastBlock-1 could be counted, but much harder to implement)
	lastBlockToMonitor := epochLastBlock - 2

	return Window{Start: firstBlockToMonitor, End: lastBlockToMonitor}
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
	return MonitoringWindow(epoch, um.epochSize, um.lookbackWindow)
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
		err := errors.New("Accumulated uptimes not found, cannot update validator scores")
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

// ProcessBlock uses the block's signature bitmap (which encodes who signed the parent block) to update the epoch's Uptime data
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
		uptime = updateUptime(uptime, block.NumberU64()-1, signedValidatorsBitmap, um.lookbackWindow, um.MonitoringWindow(epochNum))
		uptime.LatestBlock = block.NumberU64()
		um.store.WriteAccumulatedEpochUptime(epochNum, uptime)
	} else {
		log.Trace("WritingBlockWithState with block number less than a block we previously wrote", "latestUptimeBlock", uptime.LatestBlock, "blockNumber", block.NumberU64())
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
