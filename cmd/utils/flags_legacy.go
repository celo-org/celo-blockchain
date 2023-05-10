// Copyright 2020 The go-ethereum Authors
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

package utils

import (
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/eth/ethconfig"
	"github.com/celo-org/celo-blockchain/node"
	"gopkg.in/urfave/cli.v1"
)

var ShowDeprecated = cli.Command{
	Action:      showDeprecated,
	Name:        "show-deprecated-flags",
	Usage:       "Show flags that have been deprecated",
	ArgsUsage:   " ",
	Category:    "MISCELLANEOUS COMMANDS",
	Description: "Show flags that have been deprecated and will soon be removed",
}

var DeprecatedFlags = []cli.Flag{
	LegacyTestnetFlag,
	LegacyLightServFlag,
	LegacyLightPeersFlag,
	LegacyMinerExtraDataFlag,
	LegacyMinerGasPriceFlag,
	EtherbaseFlag,
	LegacyProxyEnodeURLPairsFlag,
	LegacyIstanbulRequestTimeoutFlag,
	LegacyIstanbulBlockPeriodFlag,
	LegacyIstanbulProposerPolicyFlag,
	LegacyEthStatsURLFlag,
	NoUSBFlag,
}

var (
	// (Deprecated April 2018)
	LegacyMinerExtraDataFlag = cli.StringFlag{
		Name:  "extradata",
		Usage: "Block extra data set by the miner (default = client version, deprecated, use --miner.extradata)",
	}

	// (Deprecated June 2019)
	LegacyLightServFlag = cli.IntFlag{
		Name:  "lightserv",
		Usage: "Maximum percentage of time allowed for serving LES requests (deprecated, use --light.serve)",
		Value: ethconfig.Defaults.LightServ,
	}
	LegacyLightPeersFlag = cli.IntFlag{
		Name:  "lightpeers",
		Usage: "Maximum number of light clients to serve, or light servers to attach to  (deprecated, use --light.maxpeers)",
		Value: ethconfig.Defaults.LightPeers,
	}

	// (Deprecated April 2020)
	LegacyTestnetFlag = cli.BoolFlag{ // TODO(q9f): Remove after Ropsten is discontinued.
		Name:  "testnet",
		Usage: "Pre-configured test network (Deprecated: Please choose one of --goerli, --rinkeby, or --ropsten.)",
	}

	// (Deprecated May 2020, shown in aliased flags section)
	NoUSBFlag = cli.BoolFlag{
		Name:  "nousb",
		Usage: "Disables monitoring for and managing USB hardware wallets (deprecated)",
	}
	LegacyWSListenAddrFlag = cli.StringFlag{
		Name:  "wsaddr",
		Usage: "WS-RPC server listening interface (deprecated, use --ws.addr)",
		Value: node.DefaultWSHost,
	}
	LegacyWSPortFlag = cli.IntFlag{
		Name:  "wsport",
		Usage: "WS-RPC server listening port (deprecated, use --ws.port)",
		Value: node.DefaultWSPort,
	}
	LegacyWSApiFlag = cli.StringFlag{
		Name:  "wsapi",
		Usage: "API's offered over the WS-RPC interface (deprecated, use --ws.api)",
		Value: "",
	}
	LegacyWSAllowedOriginsFlag = cli.StringFlag{
		Name:  "wsorigins",
		Usage: "Origins from which to accept websockets requests (deprecated, use --ws.origins)",
		Value: "",
	}

	// Deprecated in celo-blockchain v1.2.0
	LegacyEthStatsURLFlag = cli.StringFlag{
		Name:  "ethstats",
		Usage: "Reporting URL of a celostats service (nodename:secret@host:port) (deprecated, Use --celostats)",
	}
	LegacyProxyEnodeURLPairsFlag = cli.StringFlag{
		Name:  "proxy.proxyenodeurlpair",
		Usage: "Each enode URL in a pair is separated by a semicolon. Enode URL pairs are separated by a space. The format should be \"<proxy 0 internal facing enode URL>;<proxy 0 external facing enode URL>,<proxy 1 internal facing enode URL>;<proxy 1 external facing enode URL>,...\" (deprecated, use --proxy.proxyenodeurlpairs)",
	}

	// Deprecated in celo-blockchain v1.3.0
	LegacyMinerEtherbaseFlag = cli.StringFlag{
		Name:  "miner.etherbase",
		Usage: "Public address for block mining rewards (deprecated, use --etherbase or both --tx-fee-recipient and --miner.validator)",
		Value: "0",
	}

	LegacyIstanbulRequestTimeoutFlag = cli.Uint64Flag{
		Name:  "istanbul.requesttimeout",
		Usage: "Timeout for each Istanbul round in milliseconds (deprecated, value obtained from genesis config)",
		Value: 0,
	}
	LegacyIstanbulBlockPeriodFlag = cli.Uint64Flag{
		Name:  "istanbul.blockperiod",
		Usage: "Default minimum difference between two consecutive block's timestamps in seconds  (deprecated, value obtained from genesis config)",
		Value: 0,
	}
	LegacyIstanbulProposerPolicyFlag = cli.Uint64Flag{
		Name:  "istanbul.proposerpolicy",
		Usage: "Default minimum difference between two consecutive block's timestamps in seconds (deprecated, value obtained from genesis config)",
		Value: 0,
	}

	// Deprecated in celo-blockchain 1.4.0
	LegacyMinerGasPriceFlag = BigFlag{
		Name:  "miner.gasprice",
		Usage: "Minimum gas price for mining a transaction",
		Value: big.NewInt(1),
	}

	// (Deprecated July 2020, shown in aliased flags section)
	LegacyGraphQLListenAddrFlag = cli.StringFlag{
		Name:  "graphql.addr",
		Usage: "GraphQL server listening interface (deprecated, graphql can only be enabled on the HTTP-RPC server endpoint, use --graphql)",
	}
	LegacyGraphQLPortFlag = cli.IntFlag{
		Name:  "graphql.port",
		Usage: "GraphQL server listening port (deprecated, graphql can only be enabled on the HTTP-RPC server endpoint, use --graphql)",
		Value: node.DefaultHTTPPort,
	}
)

// showDeprecated displays deprecated flags that will be soon removed from the codebase.
func showDeprecated(*cli.Context) {
	fmt.Println("--------------------------------------------------------------------")
	fmt.Println("The following flags are deprecated and will be removed in the future!")
	fmt.Println("--------------------------------------------------------------------")
	fmt.Println()
	for _, flag := range DeprecatedFlags {
		fmt.Println(flag.String())
	}
	fmt.Println()
}
