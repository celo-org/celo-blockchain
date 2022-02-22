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

// Uptime contains the latest block for which uptime metrics were accounted. It also contains
// an array of Entries where the `i`th entry represents the uptime statistics of the `i`th validator
// in the validator set for that epoch
type Uptime struct {
	LatestHeader *types.Header
	Entries      []UptimeEntry
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
	epoch           uint64
	epochSize       uint64
	firstEpochBlock uint64
	lastEpochBlock  uint64
	lookbackWindow  uint64
	valSetSize      int
	window          Window

	accumulatedUptime *Uptime
	logger            log.Logger
}

// NewMonitor creates a new uptime monitor
func NewMonitor(epochSize, epoch, lookbackWindow uint64, valSetSize int) *Monitor {
	window := MustMonitoringWindow(epoch, epochSize, lookbackWindow)
	uptime := new(Uptime)
	uptime.Entries = make([]UptimeEntry, valSetSize)
	firstEpochBlock, err := istanbul.GetEpochFirstBlockNumber(epoch, epochSize)
	if err != nil {
		// this is because the getEpochFirst...returns error if the first epoch was requested
		// this shouldn't happen
		firstEpochBlock = 0
	}
	lastEpochBlock := istanbul.GetEpochLastBlockNumber(epoch, epochSize)

	return &Monitor{
		epoch:           epoch,
		epochSize:       epochSize,
		firstEpochBlock: firstEpochBlock,
		lastEpochBlock:  lastEpochBlock,

		lookbackWindow:    lookbackWindow,
		valSetSize:        valSetSize,
		window:            window,
		accumulatedUptime: uptime,
		logger:            log.New("module", "uptime-monitor"),
	}
}

// ComputeValidatorsUptime retrieves the uptime score for each validator for a given epoch
func (um *Monitor) ComputeUptime(header *types.Header) ([]*big.Int, error) {
	logger := um.logger.New("func", "Backend.updateValidatorScores", "epoch", um.epoch, "until header number", um.accumulatedUptime.LatestHeader.Number.Uint64())
	logger.Trace("Updating validator scores")

	headerNumber := header.Number.Uint64()

	if headerNumber < um.firstEpochBlock || headerNumber > um.lastEpochBlock {
		return nil, ErrWrongEpoch
	}

	// first block of the epoch has the parentSeal of the last epoch, and it requires
	// at least lookbackWindow headers, to calculate the first score
	if headerNumber < um.firstEpochBlock+um.lookbackWindow {
		return nil, ErrUnpreparedCompute
	}

	// The totalMonitoredBlocks are the total number of block on which we monitor uptime until the header.Number
	window, err := MonitoringWindowUntil(um.epoch, um.epochSize, um.lookbackWindow, headerNumber)
	if err != nil {
		return nil, err
	}
	totalMonitoredBlocks := window.Size()

	uptimes := make([]*big.Int, um.valSetSize)

	for i, entry := range um.accumulatedUptime.Entries {
		numerator := big.NewInt(0).Mul(big.NewInt(int64(entry.UpBlocks)), params.Fixidity1)
		uptimes[i] = big.NewInt(0).Div(numerator, big.NewInt(int64(totalMonitoredBlocks)))
	}

	return uptimes, nil
}

// ProcessHeader uses the header's signature bitmap (which encodes who signed the parent block) to update the epoch's Uptime data
func (um *Monitor) ProcessHeader(header *types.Header) error {
	headerNumber := header.Number.Uint64()

	if headerNumber < um.firstEpochBlock || headerNumber > um.lastEpochBlock {
		return ErrWrongEpoch
	}
	// The epoch's first block's aggregated parent signatures is for the previous epoch's valset.
	// We can ignore updating the tally for that block.
	if headerNumber == um.firstEpochBlock {
		if um.accumulatedUptime.LatestHeader == nil {
			um.accumulatedUptime.LatestHeader = header
			return nil
		}
	}
	// Monitor was never initialized with the first block of the epoch
	if um.accumulatedUptime.LatestHeader == nil {
		return ErrMissingPreviousHeaders
	}

	if um.accumulatedUptime.LatestHeader.Number.Uint64() >= headerNumber {
		return ErrHeaderNumberAlreadyUsed
	}

	if um.accumulatedUptime.LatestHeader.Hash() != header.ParentHash {
		return ErrMissingPreviousHeaders
	}

	// Get the bitmap from the previous block
	extra, err := types.ExtractIstanbulExtra(header)
	if err != nil {
		um.logger.Error("Unable to extract istanbul extra", "func", "ProcessBlock", "blocknum", headerNumber)
		return errors.New("could not extract block header extra")
	}
	signedValidatorsBitmap := extra.ParentAggregatedSeal.Bitmap

	updateUptime(um.accumulatedUptime, headerNumber-1, signedValidatorsBitmap, um.lookbackWindow, um.window)
	um.accumulatedUptime.LatestHeader = header

	return nil
}

func (um *Monitor) Clear() {
	um.accumulatedUptime = new(Uptime)
	um.accumulatedUptime.Entries = make([]UptimeEntry, um.valSetSize)
}

func (um *Monitor) GetLastProcessedHeader() *types.Header {
	return um.accumulatedUptime.LatestHeader
}

func (um *Monitor) GetEpochSize() uint64 {
	return um.epochSize
}

func (um *Monitor) GetEpoch() uint64 {
	return um.epoch
}

// updateUptime updates the accumulated uptime given a block and its validator's signatures bitmap
func updateUptime(uptime *Uptime, blockNumber uint64, bitmap *big.Int, lookbackWindowSize uint64, monitoringWindow Window) {
	// Obtain current lookback window
	currentLookbackWindow := newWindowEndingAt(blockNumber, lookbackWindowSize)
	bitmapClone := new(big.Int).Set(bitmap)
	byteWords := bitmapClone.Bits()

	for i := 0; i < len(uptime.Entries); i++ {
		byteNumber := i / 64
		if len(byteWords) > byteNumber {
			// validator signature present => update their latest signed block
			if byteWords[byteNumber]&1 == 1 {
				uptime.Entries[i].LastSignedBlock = blockNumber
			}
			byteWords[byteNumber] = byteWords[byteNumber] >> 1
		}

		// If block number is to be monitored, then check if lastSignedBlock is within current lookback window
		if monitoringWindow.Contains(blockNumber) && currentLookbackWindow.Contains(uptime.Entries[i].LastSignedBlock) {
			// since within currentLookbackWindow there's at least one signed block (lastSignedBlock) validator is considered UP
			uptime.Entries[i].UpBlocks++
		}
	}
}
