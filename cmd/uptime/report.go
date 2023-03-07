package main

import (
	"fmt"
	"time"

	"github.com/celo-org/celo-blockchain/cmd/utils"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/uptime"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/node"
	"gopkg.in/urfave/cli.v1"
)

var epochFlag = cli.Int64Flag{
	Name:  "epoch",
	Usage: "Epoch number to report on",
}

var lookbackFlag = cli.Int64Flag{
	Name:  "lookback",
	Usage: "Lookback window to use for the uptime calculation",
}

var valSetSizeFlag = cli.Int64Flag{
	Name:  "valset",
	Usage: "Validator set size to use in the calculation",
}

var reportUptimeCommand = cli.Command{
	Name:      "report",
	Usage:     "Reports uptime for all validators",
	Action:    utils.MigrateFlags(reportUptime),
	ArgsUsage: "",
	Flags:     []cli.Flag{epochFlag, lookbackFlag, valSetSizeFlag, utils.DataDirFlag},
}

// getHeaderByNumber retrieves a block header from the database by number,
// caching it (associated with its hash) if found.
func getHeaderByNumber(db ethdb.Database, number uint64) *types.Header {
	hash := rawdb.ReadCanonicalHash(db, number)
	if hash == (common.Hash{}) {
		return nil
	}
	return rawdb.ReadHeader(db, hash, number)
}

func reportUptime(ctx *cli.Context) error {
	if !ctx.IsSet(epochFlag.Name) {
		utils.Fatalf("This command requires an epoch argument")
	}
	epoch := ctx.Uint64(epochFlag.Name)

	// lookback and valset size could actually be calculated
	// but we need a whole instantiated blockchain to be able
	// to execute the contract calls.
	if !ctx.IsSet(lookbackFlag.Name) {
		utils.Fatalf("This command requires a lookback argument")
	}
	lookback := ctx.Uint64(lookbackFlag.Name)

	if !ctx.IsSet(valSetSizeFlag.Name) {
		utils.Fatalf("This command requires a valset argument")
	}
	valSetSize := ctx.Uint64(valSetSizeFlag.Name)

	cfg := defaultNodeConfig()
	cfg.DataDir = utils.MakeDataDir(ctx)
	nod, _ := node.New(&cfg)
	defer nod.Close()

	db := utils.MakeChainDatabase(ctx, nod, true)
	defer db.Close()

	genesisHash := rawdb.ReadCanonicalHash(db, 0)
	genConfig := rawdb.ReadChainConfig(db, genesisHash)
	epochSize := genConfig.Istanbul.Epoch
	lastBlock := istanbul.GetEpochLastBlockNumber(epoch, epochSize)
	headers := getHeaders(db, lastBlock, int(epochSize))
	return runReport(headers, epochSize, lookback, int(valSetSize))
}

func getHeaders(db ethdb.Database, lastBlock uint64, amount int) []*types.Header {
	start := time.Now()
	headers := make([]*types.Header, amount)

	headers[amount-1] = getHeaderByNumber(db, lastBlock)
	for i := amount - 2; i >= 0; i-- {
		headers[i] = rawdb.ReadHeader(db, headers[i+1].ParentHash, headers[i+1].Number.Uint64()-1)
	}
	fmt.Printf("Headers[%d, %d] retrieved in %v\n", headers[0].Number.Int64(), headers[len(headers)-1].Number.Int64(), time.Since(start))
	return headers
}

func runReport(headers []*types.Header, epochSize uint64, lookback uint64, valSetSize int) error {
	epoch := istanbul.GetEpochNumber(headers[0].Number.Uint64(), epochSize)
	monitor := uptime.NewMonitor(epochSize, epoch, lookback, valSetSize)
	start := time.Now()
	var header *types.Header
	for _, header = range headers {
		monitor.ProcessHeader(header)
	}
	fmt.Printf("Headers added in %v\n", time.Since(start))
	r, err := monitor.ComputeUptime(header)
	if err != nil {
		return err
	}
	fmt.Printf("Report done in %v\n", time.Since(start))
	for i, v := range r {
		fmt.Println("Validator", i, "uptime", v)
	}
	return nil
}

type singleEpochStore struct {
	epoch  uint64
	uptime *uptime.Uptime
}

func (m *singleEpochStore) ReadAccumulatedEpochUptime(epoch uint64) *uptime.Uptime {
	if m.epoch == epoch {
		return m.uptime
	}
	return nil
}

func (m *singleEpochStore) WriteAccumulatedEpochUptime(epoch uint64, uptime *uptime.Uptime) {
	m.epoch = epoch
	m.uptime = uptime
}
