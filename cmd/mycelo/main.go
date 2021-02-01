package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/ethereum/go-ethereum/internal/fileutils"
	"golang.org/x/sync/errgroup"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/mycelo/cluster"
	"github.com/ethereum/go-ethereum/mycelo/config"
	"github.com/ethereum/go-ethereum/mycelo/genesis"
	"github.com/ethereum/go-ethereum/params"
	"gopkg.in/urfave/cli.v1"
)

func writeJSON(genesis *core.Genesis, filepath string) error {
	genesisBytes, err := json.Marshal(genesis)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath, genesisBytes, 0644)
}

var (
	// Git information set by linker when building with ci.go.
	gitCommit string
	gitDate   string
	app       = &cli.App{
		Name:        filepath.Base(os.Args[0]),
		Usage:       "go-ethereum devp2p tool",
		Version:     params.VersionWithCommit(gitCommit, gitDate),
		Writer:      os.Stdout,
		HideVersion: true,
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
		createConfigCommand,
		createContractsConfigCommand,
		createGenesisCommand,
		initNodesCommand,
		runCommand,
	}
}

func main() {
	exit(app.Run(os.Args))
}

var createConfigCommand = cli.Command{
	Name:   "create-config",
	Usage:  "Creates config.json file (first cmd to run)",
	Action: createConfig,
}

var createContractsConfigCommand = cli.Command{
	Name:   "create-contracts-config",
	Usage:  "Creates contracts-config.json file (run once you have config.json correctly configured)",
	Action: createContractsConfig,
}

var createGenesisCommand = cli.Command{
	Name:   "create-genesis",
	Usage:  "Creates genesis.json (requires config.json & contract-config.json correctly configured)",
	Action: createGenesis,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "buildpath",
			Usage: "Directory where smartcontract truffle build file live",
		},
	},
}

var initNodesCommand = cli.Command{
	Name:   "init-nodes",
	Usage:  "Setup all validators nodes",
	Action: initNodes,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "geth",
			Usage: "Path to geth binary",
		},
	},
}

var runCommand = cli.Command{
	Name:   "run",
	Usage:  "Runs the testnet",
	Action: run,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "geth",
			Usage: "Path to geth binary",
		},
	},
}

func readPaths(ctx *cli.Context) (config.Paths, error) {
	if ctx.NArg() != 1 {
		return config.Paths{}, fmt.Errorf("Missing directory argument")
	}

	workdir := ctx.Args()[0]

	if !fileutils.FileExists(workdir) {
		os.MkdirAll(workdir, os.ModePerm)
	}
	return config.Paths{
		Workdir: workdir,
	}, nil
}

func createConfig(ctx *cli.Context) error {
	paths, err := readPaths(ctx)
	if err != nil {
		return err
	}

	var cfg = config.DefaultConfig()
	return config.WriteConfig(cfg, paths.Config())
}

func createContractsConfig(ctx *cli.Context) error {
	paths, err := readPaths(ctx)
	if err != nil {
		return err
	}

	cfg, err := config.ReadConfig(paths.Config())
	if err != nil {
		return err
	}

	err = config.WriteContractsConfig(config.DefaultContractsConfig(cfg), paths.ContractsConfig())
	if err != nil {
		return err
	}

	return nil
}
func createGenesis(ctx *cli.Context) error {
	paths, err := readPaths(ctx)
	if err != nil {
		return err
	}

	buildpath := ctx.String("buildpath")
	if buildpath == "" {
		buildpath = path.Join(os.Getenv("CELO_MONOREPO"), "packages/protocol/build/contracts")
		if fileutils.FileExists(buildpath) {
			log.Info("Missing --buildpath flag, using CELO_MONOREPO derived path", "buildpath", buildpath)
		} else {
			return fmt.Errorf("Missing --buildpath flag")
		}
	}

	cfg, err := config.ReadConfig(paths.Config())
	if err != nil {
		return err
	}
	contractsCfg, err := config.ReadContractsConfig(paths.ContractsConfig())
	if err != nil {
		return err
	}

	genesis, err := genesis.GenerateGenesis(cfg, contractsCfg, buildpath)
	if err != nil {
		return err
	}

	return writeJSON(genesis, paths.GenesisJSON())
}

func initNodes(ctx *cli.Context) error {
	paths, err := readPaths(ctx)
	if err != nil {
		return err
	}

	paths.Geth = ctx.String("geth")
	if paths.Geth == "" {
		return fmt.Errorf("Missing --geth flag")
	}

	cfg, err := config.ReadConfig(paths.Config())
	if err != nil {
		return err
	}

	cluster := cluster.New(paths, cfg)
	return cluster.Init()
}

func run(cliCtx *cli.Context) error {
	paths, err := readPaths(cliCtx)
	if err != nil {
		return err
	}

	paths.Geth = cliCtx.String("geth")
	if paths.Geth == "" {
		return fmt.Errorf("Missing --geth flag")
	}

	cfg, err := config.ReadConfig(paths.Config())
	if err != nil {
		return err
	}

	cluster := cluster.New(paths, cfg)

	ctx := context.Background()
	group, ctx := errgroup.WithContext(ctx)

	group.Go(func() error { return cluster.Run(ctx) })
	// group.Go(func() error { return cluster.RunMigrations() })

	return group.Wait()
}

// commandHasFlag returns true if the current command supports the given flag.
func commandHasFlag(ctx *cli.Context, flag cli.Flag) bool {
	flags := ctx.FlagNames()
	sort.Strings(flags)
	i := sort.SearchStrings(flags, flag.GetName())
	return i != len(flags) && flags[i] == flag.GetName()
}

func exit(err interface{}) {
	if err == nil {
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
