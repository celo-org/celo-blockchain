// Copyright 2017 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ethereum/go-ethereum/cmd/internal/browser"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/params"
	cli "gopkg.in/urfave/cli.v1"
)

var bugCommand = cli.Command{
	Action:    utils.MigrateFlags(reportBug),
	Name:      "bug",
	Usage:     "opens a window to report a bug on the geth repo",
	ArgsUsage: " ",
	Category:  "MISCELLANEOUS COMMANDS",
}

const issueURL = "https://github.com/celo-org/celo-blockchain/issues/new"

// reportBug reports a bug by opening a new URL to the go-ethereum GH issue
// tracker and setting default values as the issue body.
func reportBug(ctx *cli.Context) error {
	// execute template and write contents to buff
	var buff bytes.Buffer

	fmt.Fprintln(&buff, header)
	printSystemInformation(&buff)

	// open a new GH issue
	if !browser.Open(issueURL + "?body=" + url.QueryEscape(buff.String())) {
		fmt.Printf("Please file a new issue at %s using this template:\n\n%s", issueURL, buff.String())
	}
	return nil
}

func printSystemInformation(w io.Writer) {
	if gitCommit != "" {
		fmt.Fprintln(w, "Git Commit:", gitCommit)
	}
	fmt.Fprintln(w, "Geth Version:", params.VersionWithMeta)
	fmt.Fprintln(w)
	// TODO(trevor): uncomment this and set to future mainnet network id
	// fmt.Println("Network Id:", eth.DefaultConfig.NetworkId)
	fmt.Fprintln(w, "Go Version:", runtime.Version())
	fmt.Fprintf(w, "GOPATH=%s\n", os.Getenv("GOPATH"))
	fmt.Fprintf(w, "GOROOT=%s\n", runtime.GOROOT())
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Architecture:", runtime.GOARCH)
	fmt.Fprintln(w, "Operating System:", runtime.GOOS)
	printOSDetails(w)
}

// copied from the Go source. Copyright 2017 The Go Authors
func printOSDetails(w io.Writer) {
	switch runtime.GOOS {
	case "darwin":
		printCmdOut(w, "uname -v: ", "uname", "-v")
		printCmdOut(w, "", "sw_vers")
	case "linux":
		printCmdOut(w, "uname -sr: ", "uname", "-sr")
		printCmdOut(w, "", "lsb_release", "-a")
	case "openbsd", "netbsd", "freebsd", "dragonfly":
		printCmdOut(w, "uname -v: ", "uname", "-v")
	case "solaris":
		out, err := ioutil.ReadFile("/etc/release")
		if err == nil {
			fmt.Fprintf(w, "/etc/release: %s\n", out)
		} else {
			fmt.Printf("failed to read /etc/release: %v\n", err)
		}
	}
}

// printCmdOut prints the output of running the given command.
// It ignores failures; 'go bug' is best effort.
//
// copied from the Go source. Copyright 2017 The Go Authors
func printCmdOut(w io.Writer, prefix, path string, args ...string) {
	cmd := exec.Command(path, args...)
	out, err := cmd.Output()
	if err != nil {
		fmt.Printf("%s %s: %v\n", path, strings.Join(args, " "), err)
		return
	}
	fmt.Fprintf(w, "%s%s\n", prefix, bytes.TrimSpace(out))
}

const header = `
### Expected Behavior

Please describe the behavior you are expecting

### Current Behavior

What is the current behavior?

### Steps to Reproduce Behavior

How can we reproduce this?

### Logs

Are there any logs?

### System Information

`
