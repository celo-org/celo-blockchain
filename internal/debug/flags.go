// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package debug

import (
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof" // #nosec TODO?
	"os"
	"runtime"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/metrics/exp"
	"github.com/fjl/memsize/memsizeui"
	colorable "github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"gopkg.in/urfave/cli.v1"
)

var Memsize memsizeui.Handler

var (
	verbosityFlag = cli.IntFlag{
		Name:  "verbosity",
		Usage: "Logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail",
		Value: 3,
	}
	vmoduleFlag = cli.StringFlag{
		Name:  "vmodule",
		Usage: "Per-module verbosity: comma-separated list of <pattern>=<level> (e.g. eth/*=5,p2p=4)",
		Value: "",
	}
	backtraceAtFlag = cli.StringFlag{
		Name:  "backtrace",
		Usage: "Request a stack trace at a specific logging statement (e.g. \"block.go:271\")",
		Value: "",
	}
	debugFlag = cli.BoolFlag{
		Name:  "debug",
		Usage: "Prepends log messages with call-site location (file and line number)",
	}
	pprofFlag = cli.BoolFlag{
		Name:  "pprof",
		Usage: "Enable the pprof HTTP server",
	}
	pprofPortFlag = cli.IntFlag{
		Name:  "pprof.port",
		Usage: "pprof HTTP server listening port",
		Value: 6060,
	}
	pprofAddrFlag = cli.StringFlag{
		Name:  "pprof.addr",
		Usage: "pprof HTTP server listening interface",
		Value: "127.0.0.1",
	}
	memprofilerateFlag = cli.IntFlag{
		Name:  "pprof.memprofilerate",
		Usage: "Turn on memory profiling with the given rate",
		Value: runtime.MemProfileRate,
	}
	blockprofilerateFlag = cli.IntFlag{
		Name:  "pprof.blockprofilerate",
		Usage: "Turn on block profiling with the given rate",
	}
	cpuprofileFlag = cli.StringFlag{
		Name:  "pprof.cpuprofile",
		Usage: "Write CPU profile to the given file",
	}
	traceFlag = cli.StringFlag{
		Name:  "trace",
		Usage: "Write execution trace to the given file",
	}
	consoleFormatFlag = cli.StringFlag{
		Name:  "consoleformat",
		Usage: "Write console logs as 'json' or 'term'",
	}
	consoleOutputFlag = cli.StringFlag{
		Name: "consoleoutput",
		Usage: "(stderr|stdout|split) By default, console output goes to stderr. " +
			"In stdout mode, write console logs to stdout (not stderr). " +
			"In split mode, write critical(warning, error, and critical) console logs to stderr " +
			"and non-critical (info, debug, and trace) to stdout",
	}
	// (Deprecated April 2020 in go-ethereum, Feb 2021 in celo-blockchain)
	legacyPprofPortFlag = cli.IntFlag{
		Name:  "pprofport",
		Usage: "pprof HTTP server listening port (deprecated, use --pprof.port)",
		Value: 6060,
	}
	legacyPprofAddrFlag = cli.StringFlag{
		Name:  "pprofaddr",
		Usage: "pprof HTTP server listening interface (deprecated, use --pprof.addr)",
		Value: "127.0.0.1",
	}
	legacyMemprofilerateFlag = cli.IntFlag{
		Name:  "memprofilerate",
		Usage: "Turn on memory profiling with the given rate (deprecated, use --pprof.memprofilerate)",
		Value: runtime.MemProfileRate,
	}
	legacyBlockprofilerateFlag = cli.IntFlag{
		Name:  "blockprofilerate",
		Usage: "Turn on block profiling with the given rate (deprecated, use --pprof.blockprofilerate)",
	}
	legacyCpuprofileFlag = cli.StringFlag{
		Name:  "cpuprofile",
		Usage: "Write CPU profile to the given file (deprecated, use --pprof.cpuprofile)",
	}
)

// Flags holds all command-line flags required for debugging.
var Flags = []cli.Flag{
	verbosityFlag, vmoduleFlag, backtraceAtFlag, debugFlag,
	pprofFlag, pprofAddrFlag, pprofPortFlag, memprofilerateFlag,
	blockprofilerateFlag, cpuprofileFlag, traceFlag,
	consoleFormatFlag, consoleOutputFlag,
}

var DeprecatedFlags = []cli.Flag{
	legacyPprofPortFlag, legacyPprofAddrFlag, legacyMemprofilerateFlag,
	legacyBlockprofilerateFlag, legacyCpuprofileFlag,
}

var (
	ostream log.Handler
	glogger *log.GlogHandler
)

type StdoutStderrHandler struct {
	stdoutHandler log.Handler
	stderrHandler log.Handler
}

func (this StdoutStderrHandler) Log(r *log.Record) error {
	switch r.Lvl {
	case log.LvlCrit:
		fallthrough
	case log.LvlError:
		fallthrough
	case log.LvlWarn:
		return this.stderrHandler.Log(r)

	case log.LvlInfo:
		fallthrough
	case log.LvlDebug:
		fallthrough
	case log.LvlTrace:
		return this.stdoutHandler.Log(r)

	default:
		return this.stdoutHandler.Log(r)
	}
}

func init() {
	ostream = log.StreamHandler(io.Writer(os.Stderr), log.TerminalFormat(false))
	glogger = log.NewGlogHandler(ostream)
}

// Setup initializes profiling and logging based on the CLI flags.
// It should be called as early as possible in the program.
func Setup(ctx *cli.Context) error {
	// logging

	consoleFormat := ctx.GlobalString(consoleFormatFlag.Name)
	consoleOutputMode := ctx.GlobalString(consoleOutputFlag.Name)

	ostream := CreateStreamHandler(consoleFormat, consoleOutputMode)
	glogger = log.NewGlogHandler(ostream)

	log.PrintOrigins(ctx.GlobalBool(debugFlag.Name))
	glogger.Verbosity(log.Lvl(ctx.GlobalInt(verbosityFlag.Name)))
	glogger.Vmodule(ctx.GlobalString(vmoduleFlag.Name))
	glogger.BacktraceAt(ctx.GlobalString(backtraceAtFlag.Name))
	log.Root().SetHandler(glogger)

	// profiling, tracing
	if ctx.GlobalIsSet(legacyMemprofilerateFlag.Name) {
		runtime.MemProfileRate = ctx.GlobalInt(legacyMemprofilerateFlag.Name)
		log.Warn("The flag --memprofilerate is deprecated and will be removed in the future, please use --pprof.memprofilerate")
	}
	runtime.MemProfileRate = ctx.GlobalInt(memprofilerateFlag.Name)

	if ctx.GlobalIsSet(legacyBlockprofilerateFlag.Name) {
		Handler.SetBlockProfileRate(ctx.GlobalInt(legacyBlockprofilerateFlag.Name))
		log.Warn("The flag --blockprofilerate is deprecated and will be removed in the future, please use --pprof.blockprofilerate")
	}
	Handler.SetBlockProfileRate(ctx.GlobalInt(blockprofilerateFlag.Name))

	if traceFile := ctx.GlobalString(traceFlag.Name); traceFile != "" {
		if err := Handler.StartGoTrace(traceFile); err != nil {
			return err
		}
	}

	if cpuFile := ctx.GlobalString(cpuprofileFlag.Name); cpuFile != "" {
		if err := Handler.StartCPUProfile(cpuFile); err != nil {
			return err
		}
	}
	if cpuFile := ctx.GlobalString(legacyCpuprofileFlag.Name); cpuFile != "" {
		log.Warn("The flag --cpuprofile is deprecated and will be removed in the future, please use --pprof.cpuprofile")
		if err := Handler.StartCPUProfile(cpuFile); err != nil {
			return err
		}
	}

	// pprof server
	if ctx.GlobalBool(pprofFlag.Name) {
		listenHost := ctx.GlobalString(pprofAddrFlag.Name)
		if ctx.GlobalIsSet(legacyPprofAddrFlag.Name) && !ctx.GlobalIsSet(pprofAddrFlag.Name) {
			listenHost = ctx.GlobalString(legacyPprofAddrFlag.Name)
			log.Warn("The flag --pprofaddr is deprecated and will be removed in the future, please use --pprof.addr")
		}

		port := ctx.GlobalInt(pprofPortFlag.Name)
		if ctx.GlobalIsSet(legacyPprofPortFlag.Name) && !ctx.GlobalIsSet(pprofPortFlag.Name) {
			port = ctx.GlobalInt(legacyPprofPortFlag.Name)
			log.Warn("The flag --pprofport is deprecated and will be removed in the future, please use --pprof.port")
		}

		address := fmt.Sprintf("%s:%d", listenHost, port)
		// This context value ("metrics.addr") represents the utils.MetricsHTTPFlag.Name.
		// It cannot be imported because it will cause a cyclical dependency.
		StartPProf(address, !ctx.GlobalIsSet("metrics.addr"))
	}
	return nil
}

func CreateStreamHandler(consoleFormat string, consoleOutputMode string) log.Handler {
	if consoleOutputMode == "stdout" {
		usecolor := useColor(os.Stdout)
		var output io.Writer
		if usecolor {
			output = colorable.NewColorableStdout()
		} else {
			output = io.Writer(os.Stdout)
		}
		return log.StreamHandler(output, getConsoleLogFormat(consoleFormat, usecolor))
	}

	// This is the default mode to maintain backward-compatibility with the geth command-line
	if consoleOutputMode == "stderr" || len(consoleOutputMode) == 0 {
		usecolor := useColor(os.Stderr)
		var output io.Writer
		if usecolor {
			output = colorable.NewColorableStderr()
		} else {
			output = io.Writer(os.Stderr)
		}
		return log.StreamHandler(output, getConsoleLogFormat(consoleFormat, usecolor))
	}

	if consoleOutputMode == "split" {
		usecolorStdout := useColor(os.Stdout)
		usecolorStderr := useColor(os.Stderr)

		var outputStdout io.Writer
		var outputStderr io.Writer

		if usecolorStdout {
			outputStdout = colorable.NewColorableStdout()
		} else {
			outputStdout = io.Writer(os.Stdout)
		}

		if usecolorStderr {
			outputStderr = colorable.NewColorableStderr()
		} else {
			outputStderr = io.Writer(os.Stderr)
		}

		return StdoutStderrHandler{
			stdoutHandler: log.StreamHandler(outputStdout, getConsoleLogFormat(consoleFormat, usecolorStdout)),
			stderrHandler: log.StreamHandler(outputStderr, getConsoleLogFormat(consoleFormat, usecolorStderr))}
	}

	panic(fmt.Sprintf("Unexpected value for \"%s\" flag: \"%s\"", consoleOutputFlag.Name, consoleOutputMode))
}

func useColor(file *os.File) bool {
	return (isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())) && os.Getenv("TERM") != "dumb"
}

func getConsoleLogFormat(consoleFormat string, usecolor bool) log.Format {
	if consoleFormat == "json" {
		return log.JSONFormat()
	}
	if consoleFormat == "term" || len(consoleFormat) == 0 /* No explicit format specified */ {
		return log.TerminalFormat(usecolor)
	}
	panic(fmt.Sprintf("Unexpected value for \"%s\" flag: \"%s\"", consoleFormatFlag.Name, consoleFormat))
}

func StartPProf(address string, withMetrics bool) {
	// Hook go-metrics into expvar on any /debug/metrics request, load all vars
	// from the registry into expvar, and execute regular expvar handler.
	if withMetrics {
		exp.Exp(metrics.DefaultRegistry)
	}
	http.Handle("/memsize/", http.StripPrefix("/memsize", &Memsize))
	log.Info("Starting pprof server", "addr", fmt.Sprintf("http://%s/debug/pprof", address))
	go func() {
		if err := http.ListenAndServe(address, nil); err != nil {
			log.Error("Failure in running pprof server", "err", err)
		}
	}()
}

// Exit stops all running profiles, flushing their output to the
// respective file.
func Exit() {
	Handler.StopCPUProfile()
	Handler.StopGoTrace()
}
