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
	"github.com/celo-org/celo-blockchain/params"
	"gopkg.in/urfave/cli.v1"
)

var epochFlag = cli.Int64Flag{
	Name:  "epoch",
	Usage: "Epoch number to report on",
}

var reportUptimeCommand = cli.Command{
	Name:      "report",
	Usage:     "Reports uptime for all validators",
	Action:    reportUptime,
	ArgsUsage: "",
	Flags:     []cli.Flag{epochFlag},
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
	// if len(ctx.Args()) < 1 {
	// 	utils.Fatalf("This command requires an argument.")
	// }
	epochSize := params.MainnetChainConfig.Istanbul.Epoch
	if !ctx.IsSet(epochFlag.Name) {
		utils.Fatalf("This command requires an epoch argument")
	}
	epoch := ctx.Uint64(epochFlag.Name)
	cfg := defaultNodeConfig()
	cfg.DataDir = utils.MakeDataDir(ctx)
	nod, _ := node.New(&cfg)
	defer nod.Close()

	db := utils.MakeChainDatabase(ctx, nod, true)
	defer db.Close()

	lastBlock := istanbul.GetEpochLastBlockNumber(epoch, epochSize)
	headers := getHeaders(db, lastBlock, int(epochSize))
	runReport(headers, 12)
	return nil
}

func getHeaders(db ethdb.Database, lastBlock uint64, amount int) []*types.Header {
	start := time.Now()
	headers := make([]*types.Header, amount)

	headers[amount-1] = getHeaderByNumber(db, lastBlock)
	for i := amount - 2; i >= 0; i-- {
		headers[i] = rawdb.ReadHeader(db, headers[i+1].ParentHash, headers[i+1].Number.Uint64()-1)
	}
	fmt.Printf("Headers retrieved in %v\n", time.Since(start))
	return headers
}

func runReport(headers []*types.Header, lookback uint64) {
	epochSize := uint64(len(headers))
	store := &singleEpochStore{}
	monitor := uptime.NewMonitor(store, epochSize, uint64(12))
	start := time.Now()
	for _, header := range headers {
		monitor.ProcessHeader(header)
	}
	epoch := istanbul.GetEpochNumber(headers[0].Number.Uint64(), epochSize)
	monitor.ComputeValidatorsUptime(epoch, 100)
	fmt.Printf("Report done in %v\n", time.Since(start))
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
