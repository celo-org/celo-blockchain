package main

import (
	"fmt"
	"os"
	"path"

	"github.com/celo-org/celo-blockchain/internal/fileutils"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/mycelo/genesis"
	"gopkg.in/urfave/cli.v1"
)

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
	cli.Int64Flag{
		Name:  "blockgaslimit",
		Usage: "Block gas limit",
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

var loadTestMaxPendingFlag = cli.UintFlag{
	Name:  "maxpending",
	Usage: "Maximum number of in flight txs. Set to 0 to disable.",
	Value: 200,
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

func readBuildPath(ctx *cli.Context) (string, error) {
	buildpath := ctx.String(buildpathFlag.Name)
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

func envFromTemplate(ctx *cli.Context, workdir string) (*env.Environment, *genesis.Config, error) {
	templateString := ctx.String("template")
	template := templateFromString(templateString)
	env, err := template.createEnv(workdir)
	if err != nil {
		return nil, nil, err
	}
	// Env overrides
	if ctx.IsSet("validators") {
		env.Accounts().NumValidators = ctx.Int("validators")
	}
	if ctx.IsSet("dev.accounts") {
		env.Accounts().NumDeveloperAccounts = ctx.Int("dev.accounts")
	}
	if ctx.IsSet("mnemonic") {
		env.Accounts().Mnemonic = ctx.String("mnemonic")
	}

	// Genesis config
	genesisConfig, err := template.createGenesisConfig(env)
	if err != nil {
		return nil, nil, err
	}

	// Overrides
	if ctx.IsSet("epoch") {
		genesisConfig.Istanbul.Epoch = ctx.Uint64("epoch")
	}
	if ctx.IsSet("blockperiod") {
		genesisConfig.Istanbul.BlockPeriod = ctx.Uint64("blockperiod")
	}
	if ctx.IsSet("blockgaslimit") {
		genesisConfig.Blockchain.BlockGasLimit = ctx.Uint64("blockgaslimit")
	}

	return env, genesisConfig, nil
}

func createGenesis(ctx *cli.Context) error {
	var workdir string
	var err error
	if ctx.IsSet(newEnvFlag.Name) {
		workdir = ctx.String(newEnvFlag.Name)
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

	generatedGenesis, err := genesis.GenerateGenesis(env.Accounts(), genesisConfig, buildpath)
	if err != nil {
		return err
	}

	if ctx.IsSet(newEnvFlag.Name) {
		if err = env.Save(); err != nil {
			return err
		}
	}

	return env.SaveGenesis(generatedGenesis)
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

	return genesisConfig.Save(path.Join(workdir, "genesis-config.json"))
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

	genesis, err := genesis.GenerateGenesis(env.Accounts(), genesisConfig, buildpath)
	if err != nil {
		return err
	}

	return env.SaveGenesis(genesis)
}
