package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/celo-org/celo-blockchain/cmd/utils"
	"github.com/celo-org/celo-blockchain/internal/debug"
	"github.com/celo-org/celo-blockchain/internal/flags"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/params"
	"gopkg.in/urfave/cli.v1"
)

var (
	// Git information set by linker when building with ci.go.
	gitCommit string
	gitDate   string
	app       = &cli.App{
		Name:                 filepath.Base(os.Args[0]),
		Usage:                "uptime",
		Version:              params.VersionWithCommit(gitCommit, gitDate),
		Writer:               os.Stdout,
		HideVersion:          true,
		EnableBashCompletion: true,
	}
)

func init() {
	// Set up the CLI app.
	app.Flags = append(app.Flags, debug.Flags...)
	app.Flags = append(app.Flags, utils.DataDirFlag)
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
		reportUptimeCommand,
	}
	cli.CommandHelpTemplate = flags.OriginCommandHelpTemplate
}

func exit(err interface{}) {
	if err == nil {
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func main() {
	exit(app.Run(os.Args))
}

const (
	clientIdentifier = "celo" // Client identifier to advertise over the network
)

func defaultNodeConfig() node.Config {
	cfg := node.DefaultConfig
	cfg.Name = clientIdentifier
	cfg.Version = params.VersionWithCommit(gitCommit, gitDate)
	cfg.HTTPModules = append(cfg.HTTPModules, "eth")
	cfg.WSModules = append(cfg.WSModules, "eth")
	cfg.IPCPath = "geth.ipc"
	return cfg
}
