package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/internal/fileutils"
	"golang.org/x/sync/errgroup"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/mycelo/cluster"
	"github.com/ethereum/go-ethereum/mycelo/config"
	"github.com/ethereum/go-ethereum/mycelo/genesis"
	"github.com/ethereum/go-ethereum/mycelo/loadbot"
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
		Usage:       "mycelo",
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
		createGenesisCommand,
		createGenesisConfigCommand,
		createGenesisFromConfigCommand,
		initValidatorsCommand,
		runValidatorsCommand,
		// initNodesCommand,
		// runNodesCommand,
		loadBotCommand,
	}
}

func main() {
	exit(app.Run(os.Args))
}

var cfgOverrideFlags = []cli.Flag{
	cli.IntFlag{
		Name:  "validators",
		Usage: "Number of Validators",
	},
	cli.IntFlag{
		Name:  "dev.accounts",
		Usage: "Number of developer accounts",
	},
	cli.Uint64Flag{
		Name:  "blockperiod",
		Usage: "Seconds between each block",
	},
	cli.Uint64Flag{
		Name:  "epoch",
		Usage: "Epoch size",
	},
	cli.StringFlag{
		Name:  "mnemonic",
		Usage: "Mnemonic to generate accounts",
	},
}

var createGenesisCommand = cli.Command{
	Name:   "genesis",
	Usage:  "Creates genesis.json from a template and overrides",
	Action: createGenesis,
	Flags: append([]cli.Flag{
		cli.StringFlag{
			Name:  "buildpath",
			Usage: "Directory where smartcontract truffle build file live",
		},
		cli.StringFlag{
			Name:  "template",
			Usage: "Optional template to use (default: local)",
		},
	},
		cfgOverrideFlags...),
}

var createGenesisFromConfigCommand = cli.Command{
	Name:   "genesis-from-config",
	Usage:  "Creates genesis.json from a genesis-config.json and env.json",
	Action: createGenesisFromConfig,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "buildpath",
			Usage: "Directory where smartcontract truffle build file live",
		},
	},
}

var createGenesisConfigCommand = cli.Command{
	Name:   "genesis-config",
	Usage:  "Creates genesis-config.json from a template and overrides",
	Action: createGenesisConfig,
	Flags: append([]cli.Flag{
		cli.StringFlag{
			Name:  "template",
			Usage: "Optional template to use (default: local)",
		},
	},
		cfgOverrideFlags...),
}

var initValidatorsCommand = cli.Command{
	Name:   "validator-init",
	Usage:  "Setup all validators nodes",
	Action: validatorInit,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "geth",
			Usage: "Path to geth binary",
		},
	},
}

var runValidatorsCommand = cli.Command{
	Name:   "validator-run",
	Usage:  "Runs the testnet",
	Action: validatorRun,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "geth",
			Usage: "Path to geth binary",
		},
	},
}

var initNodesCommand = cli.Command{
	Name:   "node-init",
	Usage:  "Setup all tx nodes",
	Action: nodeInit,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "geth",
			Usage: "Path to geth binary",
		},
	},
}

var runNodesCommand = cli.Command{
	Name:   "run",
	Usage:  "Runs the tx nodes",
	Action: nodeRun,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "geth",
			Usage: "Path to geth binary",
		},
	},
}

var loadBotCommand = cli.Command{
	Name:   "load-bot",
	Usage:  "Runs the load bot on the environment",
	Action: loadBot,
	Flags:  []cli.Flag{},
}

func readWorkdir(ctx *cli.Context) (string, error) {
	if ctx.NArg() != 1 {
		return "", fmt.Errorf("Missing directory argument")
	}
	return ctx.Args()[0], nil
}

func readEnv(ctx *cli.Context) (*config.Environment, error) {
	workdir, err := readWorkdir(ctx)
	if err != nil {
		return nil, err
	}
	return config.ReadEnv(workdir)
}

func envFromTemplate(ctx *cli.Context) (*config.Environment, *genesis.Config, error) {
	workdir, err := readWorkdir(ctx)
	if err != nil {
		workdir = "."
	} else {
		if !fileutils.FileExists(workdir) {
			os.MkdirAll(workdir, os.ModePerm)
		}
	}
	templateString := ctx.String("template")
	template := templateFromString(templateString)
	env, err := template.createEnv(workdir)
	if err != nil {
		return nil, nil, err
	}
	// Env overrides
	if ctx.IsSet("validators") {
		env.Config.InitialValidators = ctx.Int("validators")
	}
	if ctx.IsSet("dev.accounts") {
		env.Config.DeveloperAccounts = ctx.Int("dev.accounts")
	}
	if ctx.IsSet("mnemonic") {
		env.Config.Mnemonic = ctx.String("mnemonic")
	}

	// Create the accounts after the env overrides are set
	err = env.CreateGenesisAccounts()
	if err != nil {
		return nil, nil, err
	}

	// Genesis config
	genesisConfig, err := template.createGenesisConfig(env)
	// Overrides
	if ctx.IsSet("epoch") {
		genesisConfig.Istanbul.Epoch = ctx.Uint64("epoch")
	}
	if ctx.IsSet("blockperiod") {
		genesisConfig.Istanbul.BlockPeriod = ctx.Uint64("blockperiod")
	}

	return env, genesisConfig, nil
}

func readBuildPath(ctx *cli.Context) (string, error) {
	buildpath := ctx.String("buildpath")
	if buildpath == "" {
		buildpath = path.Join(os.Getenv("CELO_MONOREPO"), "packages/protocol/build/contracts")
		if fileutils.FileExists(buildpath) {
			log.Info("Missing --buildpath flag, using CELO_MONOREPO derived path", "buildpath", buildpath)
		} else {
			return "", fmt.Errorf("Missing --buildpath flag")
		}
	}
	return buildpath, nil
}

func readGethPath(ctx *cli.Context) (string, error) {
	buildpath := ctx.String("geth")
	if buildpath == "" {
		buildpath = path.Join(os.Getenv("CELO_BLOCKCHAIN"), "build/bin/geth")
		if fileutils.FileExists(buildpath) {
			log.Info("Missing --geth flag, using CELO_BLOCKCHAIN derived path", "geth", buildpath)
		} else {
			return "", fmt.Errorf("Missing --geth flag")
		}
	}
	return buildpath, nil
}

func createGenesis(ctx *cli.Context) error {
	env, genesisConfig, err := envFromTemplate(ctx)
	if err != nil {
		return err
	}

	buildpath, err := readBuildPath(ctx)
	if err != nil {
		return err
	}

	genesis, err := genesis.GenerateGenesis(env.AdminAccount(), env.ValidatorAccounts(), genesisConfig, buildpath)
	if err != nil {
		return err
	}

	if envdir := ctx.String("newenv"); envdir != "" || ctx.NArg() == 1 {
		err = env.WriteEnv()
		if err != nil {
			return err
		}
	}

	return writeJSON(genesis, env.Paths.GenesisJSON())
}

func createGenesisConfig(ctx *cli.Context) error {
	env, genesisConfig, err := envFromTemplate(ctx)
	if err != nil {
		return err
	}

	err = env.WriteEnv()
	if err != nil {
		return err
	}

	return genesis.WriteConfig(env, *genesisConfig)
}

func createGenesisFromConfig(ctx *cli.Context) error {
	envdir, err := readWorkdir(ctx)
	if err != nil {
		envdir = "."
	}
	env, err := config.ReadEnv(envdir)
	if err != nil {
		return err
	}
	genesisConfig, err := genesis.ReadConfig(envdir)
	if err != nil {
		return err
	}

	buildpath, err := readBuildPath(ctx)
	if err != nil {
		return err
	}

	genesis, err := genesis.GenerateGenesis(env.AdminAccount(), env.ValidatorAccounts(), genesisConfig, buildpath)
	if err != nil {
		return err
	}

	return writeJSON(genesis, env.Paths.GenesisJSON())
}

func validatorInit(ctx *cli.Context) error {
	env, err := readEnv(ctx)
	if err != nil {
		return err
	}

	env.Paths.Geth, err = readGethPath(ctx)
	if err != nil {
		return err
	}

	cluster := cluster.New(env)
	return cluster.Init()
}

func validatorRun(ctx *cli.Context) error {
	env, err := readEnv(ctx)
	if err != nil {
		return err
	}

	env.Paths.Geth, err = readGethPath(ctx)
	if err != nil {
		return err
	}

	cluster := cluster.New(env)

	runCtx := context.Background()
	group, runCtx := errgroup.WithContext(runCtx)

	group.Go(func() error { return cluster.Run(runCtx) })
	return group.Wait()
}

// TODO: Make this run a full node, not a validator node
func nodeInit(ctx *cli.Context) error {
	env, err := readEnv(ctx)
	if err != nil {
		return err
	}

	env.Paths.Geth, err = readGethPath(ctx)
	if err != nil {
		return err
	}

	cluster := cluster.New(env)
	return cluster.Init()
}

// TODO: Make this run a full node, not a validator node
func nodeRun(ctx *cli.Context) error {
	env, err := readEnv(ctx)
	if err != nil {
		return err
	}

	env.Paths.Geth, err = readGethPath(ctx)
	if err != nil {
		return err
	}

	cluster := cluster.New(env)

	runCtx := context.Background()
	group, runCtx := errgroup.WithContext(runCtx)

	group.Go(func() error { return cluster.Run(runCtx) })

	return group.Wait()
}

func loadBot(ctx *cli.Context) error {
	env, err := readEnv(ctx)
	if err != nil {
		return err
	}

	runCtx := context.Background()

	// TODO: Pull all of these values from env.json
	return loadbot.Start(runCtx, &loadbot.LoadBotConfig{
		Accounts:         env.DeveloperAccounts(),
		Amount:           big.NewInt(10000000),
		TransactionDelay: 1 * time.Second,
		ClientFactory: func() (*ethclient.Client, error) {
			return ethclient.Dial("http://localhost:8545")
		},
	})
}

func exit(err interface{}) {
	if err == nil {
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
