package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path"
	"path/filepath"

	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/internal/fileutils"
	"github.com/celo-org/celo-blockchain/internal/flags"
	"golang.org/x/sync/errgroup"

	"github.com/celo-org/celo-blockchain/internal/debug"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/mycelo/cluster"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/mycelo/loadbot"
	"github.com/celo-org/celo-blockchain/params"
	"gopkg.in/urfave/cli.v1"
)

var (
	// Git information set by linker when building with ci.go.
	gitCommit string
	gitDate   string
	app       = &cli.App{
		Name:                 filepath.Base(os.Args[0]),
		Usage:                "mycelo",
		Version:              params.VersionWithCommit(gitCommit, gitDate),
		Writer:               os.Stdout,
		HideVersion:          true,
		EnableBashCompletion: true,
	}
)

func init() {
	// Set up the CLI app.
	app.Flags = append(app.Flags, debug.Flags...)
	app.Before = func(ctx *cli.Context) error {
		return debug.Setup(ctx)
	}
	app.After = func(ctx *cli.Context) error {
		debug.Exit()
		return nil
	}
	app.CommandNotFound = func(ctx *cli.Context, cmd string) {
		fmt.Fprintf(os.Stderr, "No such command: %s\n", cmd)
		os.Exit(1)
	}
	// Add subcommands.
	app.Commands = []cli.Command{
		createGenesisCommand,
		createGenesisConfigCommand,
		createGenesisFromConfigCommand,
		initValidatorsCommand,
		runValidatorsCommand,
		// initNodesCommand,
		// runNodesCommand,
		loadBotCommand,
		envCommand,
	}
	cli.CommandHelpTemplate = flags.OriginCommandHelpTemplate
}

func main() {
	exit(app.Run(os.Args))
}

var gethPathFlag = cli.StringFlag{
	Name:  "geth",
	Usage: "Path to geth binary",
}

var gethExtraFlagsFlag = cli.StringFlag{
	Name:  "extraflags",
	Usage: "extra flags to pass to the validators",
}

var loadTestTPSFlag = cli.IntFlag{
	Name:  "tps",
	Usage: "Transactions per second to target in the load test",
	Value: 20,
}

var loadTestMaxPendingFlag = cli.UintFlag{
	Name:  "maxpending",
	Usage: "Maximum number of in flight txs. Set to 0 to disable.",
	Value: 200,
}

var loadTestSkipGasEstimationFlag = cli.BoolFlag{
	Name:  "skipgasestimation",
	Usage: "Skips estimating gas if true and instead hardcodes a value for the cUSD transfer",
}

var loadTestMixFeeCurrencyFlag = cli.BoolFlag{
	Name:  "mixfeecurrency",
	Usage: "Switches between paying for gas in cUSD and CELO",
}

var initValidatorsCommand = cli.Command{
	Name:      "validator-init",
	Usage:     "Setup all validators nodes",
	ArgsUsage: "[envdir]",
	Action:    validatorInit,
	Flags:     []cli.Flag{gethPathFlag},
}

var runValidatorsCommand = cli.Command{
	Name:      "validator-run",
	Usage:     "Runs the testnet",
	ArgsUsage: "[envdir]",
	Action:    validatorRun,
	Flags: []cli.Flag{
		gethPathFlag,
		gethExtraFlagsFlag,
		cli.BoolFlag{Name: "init", Usage: "Init nodes before running them"},
	},
}

// var initNodesCommand = cli.Command{
// 	Name:      "node-init",
// 	Usage:     "Setup all tx nodes",
// 	ArgsUsage: "[envdir]",
// 	Action:    nodeInit,
// 	Flags:     []cli.Flag{gethPathFlag},
// }

// var runNodesCommand = cli.Command{
// 	Name:      "run",
// 	Usage:     "Runs the tx nodes",
// 	ArgsUsage: "[envdir]",
// 	Action:    nodeRun,
// 	Flags:     []cli.Flag{gethPathFlag},
// }

var loadBotCommand = cli.Command{
	Name:      "load-bot",
	Usage:     "Runs the load bot on the environment",
	ArgsUsage: "[envdir]",
	Action:    loadBot,
	Flags: []cli.Flag{
		loadTestTPSFlag,
		loadTestMaxPendingFlag,
		loadTestSkipGasEstimationFlag,
		loadTestMixFeeCurrencyFlag},
}

func readWorkdir(ctx *cli.Context) (string, error) {
	if ctx.NArg() != 1 {
		fmt.Println("Using current directory as workdir")
		dir, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return dir, err
	}
	return ctx.Args().Get(0), nil
}

func readGethPath(ctx *cli.Context) (string, error) {
	gethPath := ctx.String(gethPathFlag.Name)
	if gethPath == "" {
		gethPath = path.Join(os.Getenv("CELO_BLOCKCHAIN"), "build/bin/geth")
		if fileutils.FileExists(gethPath) {
			log.Info("Missing --geth flag, using CELO_BLOCKCHAIN derived path", "geth", gethPath)
		} else {
			return "", fmt.Errorf("Missing --geth flag")
		}
	}
	return gethPath, nil
}

func readEnv(ctx *cli.Context) (*env.Environment, error) {
	workdir, err := readWorkdir(ctx)
	if err != nil {
		return nil, err
	}
	return env.Load(workdir)
}

func validatorInit(ctx *cli.Context) error {
	env, err := readEnv(ctx)
	if err != nil {
		return err
	}

	gethPath, err := readGethPath(ctx)
	if err != nil {
		return err
	}
	cfg := cluster.Config{GethPath: gethPath}

	cluster := cluster.New(env, cfg)
	return cluster.Init()
}

func validatorRun(ctx *cli.Context) error {
	env, err := readEnv(ctx)
	if err != nil {
		return err
	}

	gethPath, err := readGethPath(ctx)
	if err != nil {
		return err
	}
	extra := ""
	if ctx.IsSet(gethExtraFlagsFlag.Name) {
		extra = ctx.String(gethExtraFlagsFlag.Name)
	}
	cfg := cluster.Config{
		GethPath:   gethPath,
		ExtraFlags: extra,
	}

	cluster := cluster.New(env, cfg)

	if ctx.IsSet("init") {
		if err := cluster.Init(); err != nil {
			return fmt.Errorf("error running init: %w", err)
		}
	}

	group, runCtx := errgroup.WithContext(withExitSignals(context.Background()))

	group.Go(func() error { return cluster.Run(runCtx) })
	return group.Wait()
}

// // TODO: Make this run a full node, not a validator node
// func nodeInit(ctx *cli.Context) error {
// 	env, err := readEnv(ctx)
// 	if err != nil {
// 		return err
// 	}

// 	gethPath, err := readGethPath(ctx)
// 	if err != nil {
// 		return err
// 	}

// 	cluster := cluster.New(env, gethPath)
// 	return cluster.Init()
// }

// // TODO: Make this run a full node, not a validator node
// func nodeRun(ctx *cli.Context) error {
// 	env, err := readEnv(ctx)
// 	if err != nil {
// 		return err
// 	}

// 	gethPath, err := readGethPath(ctx)
// 	if err != nil {
// 		return err
// 	}

// 	cluster := cluster.New(env, gethPath)

// 	group, runCtx := errgroup.WithContext(withExitSignals(context.Background()))

// 	group.Go(func() error { return cluster.Run(runCtx) })

// 	return group.Wait()
// }

func loadBot(ctx *cli.Context) error {
	env, err := readEnv(ctx)
	if err != nil {
		return err
	}

	verbosityLevel := ctx.GlobalInt("verbosity")
	verbose := verbosityLevel >= 4

	runCtx := context.Background()

	var clients []*ethclient.Client
	for i := 0; i < env.Accounts().NumValidators; i++ {
		// TODO: Pull all of these values from env.json
		client, err := ethclient.Dial(env.ValidatorIPC(i))
		if err != nil {
			return err
		}
		clients = append(clients, client)
	}

	return loadbot.Start(runCtx, &loadbot.Config{
		ChainID:               env.Config.ChainID,
		Accounts:              env.Accounts().DeveloperAccounts(),
		Amount:                big.NewInt(10000000),
		TransactionsPerSecond: ctx.Int(loadTestTPSFlag.Name),
		Clients:               clients,
		Verbose:               verbose,
		MaxPending:            ctx.Uint64(loadTestMaxPendingFlag.Name),
		SkipGasEstimation:     ctx.GlobalBool(loadTestSkipGasEstimationFlag.Name),
		MixFeeCurrency:        ctx.GlobalBool(loadTestMixFeeCurrencyFlag.Name),
	})
}
