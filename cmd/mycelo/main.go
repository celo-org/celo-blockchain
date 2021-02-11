package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/internal/fileutils"
	"golang.org/x/sync/errgroup"

	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/mycelo/cluster"
	"github.com/ethereum/go-ethereum/mycelo/env"
	"github.com/ethereum/go-ethereum/mycelo/genesis"
	"github.com/ethereum/go-ethereum/mycelo/loadbot"
	"github.com/ethereum/go-ethereum/params"
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
	}
}

func main() {
	exit(app.Run(os.Args))
}

var templateFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "template",
		Usage: "Optional template to use (default: local)",
	},
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

var buildpathFlag = cli.StringFlag{
	Name:  "buildpath",
	Usage: "Directory where smartcontract truffle build file live",
}

var newEnvFlag = cli.StringFlag{
	Name:  "newenv",
	Usage: "Creates a new env in desired folder",
}

var gethPathFlag = cli.StringFlag{
	Name:  "geth",
	Usage: "Path to geth binary",
}

var createGenesisCommand = cli.Command{
	Name:      "genesis",
	Usage:     "Creates genesis.json from a template and overrides",
	Action:    createGenesis,
	ArgsUsage: "",
	Flags: append(
		[]cli.Flag{buildpathFlag, newEnvFlag},
		templateFlags...),
}

var createGenesisConfigCommand = cli.Command{
	Name:      "genesis-config",
	Usage:     "Creates genesis-config.json from a template and overrides",
	Action:    createGenesisConfig,
	ArgsUsage: "[envdir]",
	Flags:     append([]cli.Flag{}, templateFlags...),
}

var createGenesisFromConfigCommand = cli.Command{
	Name:      "genesis-from-config",
	Usage:     "Creates genesis.json from a genesis-config.json and env.json",
	ArgsUsage: "[envdir]",
	Action:    createGenesisFromConfig,
	Flags:     []cli.Flag{buildpathFlag},
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
	Flags:     []cli.Flag{gethPathFlag},
}

var initNodesCommand = cli.Command{
	Name:      "node-init",
	Usage:     "Setup all tx nodes",
	ArgsUsage: "[envdir]",
	Action:    nodeInit,
	Flags:     []cli.Flag{gethPathFlag},
}

var runNodesCommand = cli.Command{
	Name:      "run",
	Usage:     "Runs the tx nodes",
	ArgsUsage: "[envdir]",
	Action:    nodeRun,
	Flags:     []cli.Flag{gethPathFlag},
}

var loadBotCommand = cli.Command{
	Name:      "load-bot",
	Usage:     "Runs the load bot on the environment",
	ArgsUsage: "[envdir]",
	Action:    loadBot,
	Flags:     []cli.Flag{},
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

func readEnv(ctx *cli.Context) (*env.Environment, error) {
	workdir, err := readWorkdir(ctx)
	if err != nil {
		return nil, err
	}
	return env.Load(workdir)
}

func envFromTemplate(ctx *cli.Context, workdir string) (*env.Environment, *genesis.Config, error) {
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
	err = env.Refresh()
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

func createGenesis(ctx *cli.Context) error {
	var workdir string
	var err error
	if ctx.IsSet("newenv") {
		workdir = ctx.String("newenv")
		if !fileutils.FileExists(workdir) {
			os.MkdirAll(workdir, os.ModePerm)
		}
	} else {
		workdir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	env, genesisConfig, err := envFromTemplate(ctx, workdir)
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

	if ctx.IsSet("newenv") {
		if err = env.Save(); err != nil {
			return err
		}
	}

	return env.SaveGenesis(genesis)
}

func createGenesisConfig(ctx *cli.Context) error {
	workdir, err := readWorkdir(ctx)
	if err != nil {
		return err
	}

	env, genesisConfig, err := envFromTemplate(ctx, workdir)
	if err != nil {
		return err
	}

	err = env.Save()
	if err != nil {
		return err
	}

	return genesis.SaveConfig(genesisConfig, path.Join(workdir, "genesis-config.json"))
}

func createGenesisFromConfig(ctx *cli.Context) error {
	workdir, err := readWorkdir(ctx)
	if err != nil {
		return err
	}
	env, err := env.Load(workdir)
	if err != nil {
		return err
	}

	genesisConfig, err := genesis.LoadConfig(path.Join(workdir, "genesis-config.json"))
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

	return env.SaveGenesis(genesis)
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

	cluster := cluster.New(env, gethPath)
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

	cluster := cluster.New(env, gethPath)

	group, runCtx := errgroup.WithContext(withExitSignals(context.Background()))

	group.Go(func() error { return cluster.Run(runCtx) })
	return group.Wait()
}

// TODO: Make this run a full node, not a validator node
func nodeInit(ctx *cli.Context) error {
	env, err := readEnv(ctx)
	if err != nil {
		return err
	}

	gethPath, err := readGethPath(ctx)
	if err != nil {
		return err
	}

	cluster := cluster.New(env, gethPath)
	return cluster.Init()
}

// TODO: Make this run a full node, not a validator node
func nodeRun(ctx *cli.Context) error {
	env, err := readEnv(ctx)
	if err != nil {
		return err
	}

	gethPath, err := readGethPath(ctx)
	if err != nil {
		return err
	}

	cluster := cluster.New(env, gethPath)

	group, runCtx := errgroup.WithContext(withExitSignals(context.Background()))

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
