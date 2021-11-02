// Copyright 2015 The go-ethereum Authors
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

// Package utils contains internal helper functions for go-ethereum commands.
package utils

import (
	"crypto/ecdsa"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/celo-org/celo-blockchain/accounts"
	"github.com/celo-org/celo-blockchain/accounts/keystore"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/fdlimit"
	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/eth"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/ethstats"
	"github.com/celo-org/celo-blockchain/graphql"
	"github.com/celo-org/celo-blockchain/internal/ethapi"
	"github.com/celo-org/celo-blockchain/internal/flags"
	"github.com/celo-org/celo-blockchain/les"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
	"github.com/celo-org/celo-blockchain/metrics/exp"
	"github.com/celo-org/celo-blockchain/metrics/influxdb"
	"github.com/celo-org/celo-blockchain/miner"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/discv5"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/p2p/nat"
	"github.com/celo-org/celo-blockchain/p2p/netutil"
	"github.com/celo-org/celo-blockchain/params"
	"gopkg.in/urfave/cli.v1"
)

func init() {
	cli.AppHelpTemplate = `{{.Name}} {{if .Flags}}[global options] {{end}}command{{if .Flags}} [command options]{{end}} [arguments...]

VERSION:
   {{.Version}}

COMMANDS:
   {{range .Commands}}{{.Name}}{{with .ShortName}}, {{.}}{{end}}{{ "\t" }}{{.Usage}}
   {{end}}{{if .Flags}}
GLOBAL OPTIONS:
   {{range .Flags}}{{.}}
   {{end}}{{end}}
`
	cli.CommandHelpTemplate = flags.CommandHelpTemplate
	cli.HelpPrinter = printHelp
}

func printHelp(out io.Writer, templ string, data interface{}) {
	funcMap := template.FuncMap{"join": strings.Join}
	t := template.Must(template.New("help").Funcs(funcMap).Parse(templ))
	w := tabwriter.NewWriter(out, 38, 8, 2, ' ', 0)
	err := t.Execute(w, data)
	if err != nil {
		panic(err)
	}
	w.Flush()
}

// These are all the command line flags we support.
// If you add to this list, please remember to include the
// flag in the appropriate command definition.
//
// The flags are defined here so their names and help texts
// are the same for all commands.

var (
	// General settings

	DataDirFlag = DirectoryFlag{
		Name:  "datadir",
		Usage: "Data directory for the databases and keystore",
		Value: DirectoryString(node.DefaultDataDir()),
	}
	AncientFlag = DirectoryFlag{
		Name:  "datadir.ancient",
		Usage: "Data directory for ancient chain segments (default = inside chaindata)",
	}
	KeyStoreDirFlag = DirectoryFlag{
		Name:  "keystore",
		Usage: "Directory for the keystore (default = inside the datadir)",
	}
	NoUSBFlag = cli.BoolFlag{
		Name:  "nousb",
		Usage: "Disables monitoring for and managing USB hardware wallets",
	}
	NetworkIdFlag = cli.Uint64Flag{
		Name:  "networkid",
		Usage: fmt.Sprintf("Network identifier (%s)", params.NetworkIdHelp),
		Value: params.MainnetNetworkId,
	}
	AlfajoresFlag = cli.BoolFlag{
		Name:  "alfajores",
		Usage: "Alfajores network: pre-configured Celo test network",
	}
	BaklavaFlag = cli.BoolFlag{
		Name:  "baklava",
		Usage: "Baklava network: pre-configured Celo test network",
	}
	DeveloperFlag = cli.BoolFlag{
		Name:  "dev",
		Usage: "Ephemeral proof-of-authority network with a pre-funded developer account, mining enabled",
	}
	DeveloperPeriodFlag = cli.IntFlag{
		Name:  "dev.period",
		Usage: "Block period to use in developer mode (0 = mine only if transaction pending)",
	}
	IdentityFlag = cli.StringFlag{
		Name:  "identity",
		Usage: "Custom node name",
	}
	DocRootFlag = DirectoryFlag{
		Name:  "docroot",
		Usage: "Document Root for HTTPClient file scheme",
		Value: DirectoryString(HomeDir()),
	}
	ExitWhenSyncedFlag = cli.BoolFlag{
		Name:  "exitwhensynced",
		Usage: "Exits after block synchronisation completes",
	}
	IterativeOutputFlag = cli.BoolFlag{
		Name:  "iterative",
		Usage: "Print streaming JSON iteratively, delimited by newlines",
	}
	ExcludeStorageFlag = cli.BoolFlag{
		Name:  "nostorage",
		Usage: "Exclude storage entries (save db lookups)",
	}
	IncludeIncompletesFlag = cli.BoolFlag{
		Name:  "incompletes",
		Usage: "Include accounts for which we don't have the address (missing preimage)",
	}
	ExcludeCodeFlag = cli.BoolFlag{
		Name:  "nocode",
		Usage: "Exclude contract code (save db lookups)",
	}
	defaultSyncMode = eth.DefaultConfig.SyncMode
	SyncModeFlag    = TextMarshalerFlag{
		Name:  "syncmode",
		Usage: `Blockchain sync mode ("fast", "full", "light", or "lightest")`,
		Value: &defaultSyncMode,
	}
	GCModeFlag = cli.StringFlag{
		Name:  "gcmode",
		Usage: `Blockchain garbage collection mode ("full", "archive")`,
		Value: "full",
	}
	SnapshotFlag = cli.BoolTFlag{
		Name:  "snapshot",
		Usage: `Enables snapshot-database mode (default = enable)`,
	}
	TxLookupLimitFlag = cli.Int64Flag{
		Name:  "txlookuplimit",
		Usage: "Number of recent blocks to maintain transactions index by-hash for (default = index all blocks)",
		Value: 0,
	}
	LightKDFFlag = cli.BoolFlag{
		Name:  "lightkdf",
		Usage: "Reduce key-derivation RAM & CPU usage at some expense of KDF strength",
	}
	WhitelistFlag = cli.StringFlag{
		Name:  "whitelist",
		Usage: "Comma separated block number-to-hash mappings to enforce (<number>=<hash>)",
	}
	EtherbaseFlag = cli.StringFlag{
		Name:  "etherbase",
		Usage: "Public address for transaction broadcasting and block mining rewards (default = first account)",
		Value: "0",
	}
	TxFeeRecipientFlag = cli.StringFlag{
		Name:  "tx-fee-recipient",
		Usage: "Public address for block transaction fees and gateway fees",
		Value: "0",
	}
	BLSbaseFlag = cli.StringFlag{
		Name:  "blsbase",
		Usage: "Public address for block mining BLS signatures (default = first account created)",
		Value: "0",
	}

	// Hard fork activation overrides
	OverrideEHardforkFlag = cli.Uint64Flag{
		Name:  "override.eHardfork",
		Usage: "Manually specify E fork-block, overriding the bundled setting",
	}

	// Light server and client settings

	LightServeFlag = cli.IntFlag{
		Name:  "light.serve",
		Usage: "Maximum percentage of time allowed for serving LES requests (multi-threaded processing allows values over 100)",
		Value: eth.DefaultConfig.LightServ,
	}
	LightIngressFlag = cli.IntFlag{
		Name:  "light.ingress",
		Usage: "Incoming bandwidth limit for serving light clients (kilobytes/sec, 0 = unlimited)",
		Value: eth.DefaultConfig.LightIngress,
	}
	LightEgressFlag = cli.IntFlag{
		Name:  "light.egress",
		Usage: "Outgoing bandwidth limit for serving light clients (kilobytes/sec, 0 = unlimited)",
		Value: eth.DefaultConfig.LightEgress,
	}
	LightMaxPeersFlag = cli.IntFlag{
		Name:  "light.maxpeers",
		Usage: "Maximum number of light clients to serve, or light servers to attach to",
		Value: eth.DefaultConfig.LightPeers,
	}
	LightGatewayFeeFlag = BigFlag{
		Name:  "light.gatewayfee",
		Usage: "Minimum value of gateway fee to serve a light client transaction",
		Value: eth.DefaultConfig.GatewayFee,
	}
	UltraLightServersFlag = cli.StringFlag{
		Name:  "ulc.servers",
		Usage: "List of trusted ultra-light servers",
		Value: strings.Join(eth.DefaultConfig.UltraLightServers, ","),
	}
	UltraLightFractionFlag = cli.IntFlag{
		Name:  "ulc.fraction",
		Usage: "Minimum % of trusted ultra-light servers required to announce a new head",
		Value: eth.DefaultConfig.UltraLightFraction,
	}
	UltraLightOnlyAnnounceFlag = cli.BoolFlag{
		Name:  "ulc.onlyannounce",
		Usage: "Ultra light server sends announcements only",
	}
	LightNoPruneFlag = cli.BoolFlag{
		Name:  "light.nopruning",
		Usage: "Disable ancient light chain data pruning",
	}
	// Transaction pool settings

	TxPoolLocalsFlag = cli.StringFlag{
		Name:  "txpool.locals",
		Usage: "Comma separated accounts to treat as locals (no flush, priority inclusion)",
	}
	TxPoolNoLocalsFlag = cli.BoolFlag{
		Name:  "txpool.nolocals",
		Usage: "Disables price exemptions for locally submitted transactions",
	}
	TxPoolJournalFlag = cli.StringFlag{
		Name:  "txpool.journal",
		Usage: "Disk journal for local transaction to survive node restarts",
		Value: core.DefaultTxPoolConfig.Journal,
	}
	TxPoolRejournalFlag = cli.DurationFlag{
		Name:  "txpool.rejournal",
		Usage: "Time interval to regenerate the local transaction journal",
		Value: core.DefaultTxPoolConfig.Rejournal,
	}
	TxPoolPriceLimitFlag = cli.Uint64Flag{
		Name:  "txpool.pricelimit",
		Usage: "Minimum gas price limit to enforce for acceptance into the pool",
		Value: eth.DefaultConfig.TxPool.PriceLimit,
	}
	TxPoolPriceBumpFlag = cli.Uint64Flag{
		Name:  "txpool.pricebump",
		Usage: "Price bump percentage to replace an already existing transaction",
		Value: eth.DefaultConfig.TxPool.PriceBump,
	}
	TxPoolAccountSlotsFlag = cli.Uint64Flag{
		Name:  "txpool.accountslots",
		Usage: "Minimum number of executable transaction slots guaranteed per account",
		Value: eth.DefaultConfig.TxPool.AccountSlots,
	}
	TxPoolGlobalSlotsFlag = cli.Uint64Flag{
		Name:  "txpool.globalslots",
		Usage: "Maximum number of executable transaction slots for all accounts",
		Value: eth.DefaultConfig.TxPool.GlobalSlots,
	}
	TxPoolAccountQueueFlag = cli.Uint64Flag{
		Name:  "txpool.accountqueue",
		Usage: "Maximum number of non-executable transaction slots permitted per account",
		Value: eth.DefaultConfig.TxPool.AccountQueue,
	}
	TxPoolGlobalQueueFlag = cli.Uint64Flag{
		Name:  "txpool.globalqueue",
		Usage: "Maximum number of non-executable transaction slots for all accounts",
		Value: eth.DefaultConfig.TxPool.GlobalQueue,
	}
	TxPoolLifetimeFlag = cli.DurationFlag{
		Name:  "txpool.lifetime",
		Usage: "Maximum amount of time non-executable transaction are queued",
		Value: eth.DefaultConfig.TxPool.Lifetime,
	}

	// Performance tuning settings

	CacheFlag = cli.IntFlag{
		Name:  "cache",
		Usage: "Megabytes of memory allocated to internal caching (default = 4096 mainnet full node, 128 light mode)",
		Value: 1024,
	}
	CacheDatabaseFlag = cli.IntFlag{
		Name:  "cache.database",
		Usage: "Percentage of cache memory allowance to use for database io",
		Value: 50,
	}
	CacheTrieFlag = cli.IntFlag{
		Name:  "cache.trie",
		Usage: "Percentage of cache memory allowance to use for trie caching (default = 15% full mode, 30% archive mode)",
		Value: 15,
	}
	CacheTrieJournalFlag = cli.StringFlag{
		Name:  "cache.trie.journal",
		Usage: "Disk journal directory for trie cache to survive node restarts",
		Value: eth.DefaultConfig.TrieCleanCacheJournal,
	}
	CacheTrieRejournalFlag = cli.DurationFlag{
		Name:  "cache.trie.rejournal",
		Usage: "Time interval to regenerate the trie cache journal",
		Value: eth.DefaultConfig.TrieCleanCacheRejournal,
	}
	CacheGCFlag = cli.IntFlag{
		Name:  "cache.gc",
		Usage: "Percentage of cache memory allowance to use for trie pruning (default = 25% full mode, 0% archive mode)",
		Value: 25,
	}
	CacheSnapshotFlag = cli.IntFlag{
		Name:  "cache.snapshot",
		Usage: "Percentage of cache memory allowance to use for snapshot caching (default = 10% full mode, 20% archive mode)",
		Value: 10,
	}
	CacheNoPrefetchFlag = cli.BoolFlag{
		Name:  "cache.noprefetch",
		Usage: "Disable heuristic state prefetch during block import (less CPU and disk IO, more time waiting for data)",
	}
	CachePreimagesFlag = cli.BoolTFlag{
		Name:  "cache.preimages",
		Usage: "Enable recording the SHA3/keccak preimages of trie keys (default: true)",
	}
	// Miner settings

	MiningEnabledFlag = cli.BoolFlag{
		Name:  "mine",
		Usage: "Enable mining",
	}
	MinerValidatorFlag = cli.StringFlag{
		Name:  "miner.validator",
		Usage: "Public address for participation in consensus",
		Value: "0",
	}
	MinerExtraDataFlag = cli.StringFlag{
		Name:  "miner.extradata",
		Usage: "Block extra data set by the miner (default = client version)",
	}

	// Account settings

	UnlockedAccountFlag = cli.StringFlag{
		Name:  "unlock",
		Usage: "Comma separated list of accounts to unlock",
		Value: "",
	}
	PasswordFileFlag = cli.StringFlag{
		Name:  "password",
		Usage: "Password file to use for non-interactive password input",
		Value: "",
	}
	ExternalSignerFlag = cli.StringFlag{
		Name:  "signer",
		Usage: "External signer (url or path to ipc file)",
		Value: "",
	}
	VMEnableDebugFlag = cli.BoolFlag{
		Name:  "vmdebug",
		Usage: "Record information useful for VM and contract debugging",
	}
	InsecureUnlockAllowedFlag = cli.BoolFlag{
		Name:  "allow-insecure-unlock",
		Usage: "Allow insecure account unlocking when account-related RPCs are exposed by http",
	}
	RPCGlobalGasCapFlag = cli.Uint64Flag{
		Name:  "rpc.gascap",
		Usage: "Sets a cap on gas that can be used in eth_call/estimateGas (0=infinite)",
		Value: eth.DefaultConfig.RPCGasCap,
	}
	RPCGlobalTxFeeCapFlag = cli.Float64Flag{
		Name:  "rpc.txfeecap",
		Usage: "Sets a cap on transaction fee (in celo) that can be sent via the RPC APIs (0 = no cap)",
		Value: eth.DefaultConfig.RPCTxFeeCap,
	}
	// Logging and debug settings

	CeloStatsURLFlag = cli.StringFlag{
		Name:  "celostats",
		Usage: "Reporting URL of a celostats service (nodename:secret@host:port)",
	}
	NoCompactionFlag = cli.BoolFlag{
		Name:  "nocompaction",
		Usage: "Disables db compaction after import",
	}
	// RPC settings

	IPCDisabledFlag = cli.BoolFlag{
		Name:  "ipcdisable",
		Usage: "Disable the IPC-RPC server",
	}
	IPCPathFlag = DirectoryFlag{
		Name:  "ipcpath",
		Usage: "Filename for IPC socket/pipe within the datadir (explicit paths escape it)",
	}
	HTTPEnabledFlag = cli.BoolFlag{
		Name:  "http",
		Usage: "Enable the HTTP-RPC server",
	}
	HTTPListenAddrFlag = cli.StringFlag{
		Name:  "http.addr",
		Usage: "HTTP-RPC server listening interface",
		Value: node.DefaultHTTPHost,
	}
	HTTPPortFlag = cli.IntFlag{
		Name:  "http.port",
		Usage: "HTTP-RPC server listening port",
		Value: node.DefaultHTTPPort,
	}
	HTTPCORSDomainFlag = cli.StringFlag{
		Name:  "http.corsdomain",
		Usage: "Comma separated list of domains from which to accept cross origin requests (browser enforced)",
		Value: "",
	}
	HTTPVirtualHostsFlag = cli.StringFlag{
		Name:  "http.vhosts",
		Usage: "Comma separated list of virtual hostnames from which to accept requests (server enforced). Accepts '*' wildcard.",
		Value: strings.Join(node.DefaultConfig.HTTPVirtualHosts, ","),
	}
	HTTPApiFlag = cli.StringFlag{
		Name:  "http.api",
		Usage: "API's offered over the HTTP-RPC interface",
		Value: "",
	}
	GraphQLEnabledFlag = cli.BoolFlag{
		Name:  "graphql",
		Usage: "Enable GraphQL on the HTTP-RPC server. Note that GraphQL can only be started if an HTTP server is started as well.",
	}
	GraphQLCORSDomainFlag = cli.StringFlag{
		Name:  "graphql.corsdomain",
		Usage: "Comma separated list of domains from which to accept cross origin requests (browser enforced)",
		Value: "",
	}
	GraphQLVirtualHostsFlag = cli.StringFlag{
		Name:  "graphql.vhosts",
		Usage: "Comma separated list of virtual hostnames from which to accept requests (server enforced). Accepts '*' wildcard.",
		Value: strings.Join(node.DefaultConfig.GraphQLVirtualHosts, ","),
	}
	WSEnabledFlag = cli.BoolFlag{
		Name:  "ws",
		Usage: "Enable the WS-RPC server",
	}
	WSListenAddrFlag = cli.StringFlag{
		Name:  "ws.addr",
		Usage: "WS-RPC server listening interface",
		Value: node.DefaultWSHost,
	}
	WSPortFlag = cli.IntFlag{
		Name:  "ws.port",
		Usage: "WS-RPC server listening port",
		Value: node.DefaultWSPort,
	}
	WSApiFlag = cli.StringFlag{
		Name:  "ws.api",
		Usage: "API's offered over the WS-RPC interface",
		Value: "",
	}
	WSAllowedOriginsFlag = cli.StringFlag{
		Name:  "ws.origins",
		Usage: "Origins from which to accept websockets requests",
		Value: "",
	}
	ExecFlag = cli.StringFlag{
		Name:  "exec",
		Usage: "Execute JavaScript statement",
	}
	PreloadJSFlag = cli.StringFlag{
		Name:  "preload",
		Usage: "Comma separated list of JavaScript files to preload into the console",
	}

	// Network Settings

	MaxPeersFlag = cli.IntFlag{
		Name:  "maxpeers",
		Usage: "Maximum number of network peers (network disabled if set to 0)",
		Value: node.DefaultConfig.P2P.MaxPeers,
	}
	MaxPendingPeersFlag = cli.IntFlag{
		Name:  "maxpendpeers",
		Usage: "Maximum number of pending connection attempts (defaults used if set to 0)",
		Value: node.DefaultConfig.P2P.MaxPendingPeers,
	}
	ListenPortFlag = cli.IntFlag{
		Name:  "port",
		Usage: "Network listening port",
		Value: 30303,
	}
	BootnodesFlag = cli.StringFlag{
		Name:  "bootnodes",
		Usage: "Comma separated enode URLs for P2P discovery bootstrap",
		Value: "",
	}
	NodeKeyFileFlag = cli.StringFlag{
		Name:  "nodekey",
		Usage: "P2P node key file",
	}
	NodeKeyHexFlag = cli.StringFlag{
		Name:  "nodekeyhex",
		Usage: "P2P node key as hex (for testing)",
	}
	NATFlag = cli.StringFlag{
		Name:  "nat",
		Usage: "NAT port mapping mechanism (any|none|upnp|pmp|extip:<IP>)",
		Value: "any",
	}
	NoDiscoverFlag = cli.BoolFlag{
		Name:  "nodiscover",
		Usage: "Disables the peer discovery mechanism (manual peer addition)",
	}
	DiscoveryV5Flag = cli.BoolFlag{
		Name:  "v5disc",
		Usage: "Enables the experimental RLPx V5 (Topic Discovery) mechanism",
	}
	NetrestrictFlag = cli.StringFlag{
		Name:  "netrestrict",
		Usage: "Restricts network communication to the given IP networks (CIDR masks)",
	}
	PingIPFromPacketFlag = cli.BoolFlag{
		Name:  "ping-ip-from-packet",
		Usage: "Has the discovery protocol use the IP address given by a ping packet",
	}
	UseInMemoryDiscoverTableFlag = cli.BoolFlag{
		Name:  "use-in-memory-discovery-table",
		Usage: "Specifies whether to use an in memory discovery table",
	}

	VersionCheckFlag = cli.BoolFlag{
		Name:  "disable-version-check",
		Usage: "Disable version check. Use if the parameter is set erroneously",
	}
	DNSDiscoveryFlag = cli.StringFlag{
		Name:  "discovery.dns",
		Usage: "Sets DNS discovery entry points (use \"\" to disable DNS)",
	}

	// ATM the url is left to the user and deployment to
	JSpathFlag = cli.StringFlag{
		Name:  "jspath",
		Usage: "JavaScript root path for `loadScript`",
		Value: ".",
	}

	WhisperEnabledFlag = cli.BoolFlag{
		Name:  "shh",
		Usage: "Enable Whisper",
	}
	WhisperMaxMessageSizeFlag = cli.IntFlag{
		Name:  "shh.maxmessagesize",
		Usage: "Max message size accepted",
		Value: 1024 * 1024,
	}
	WhisperMinPOWFlag = cli.Float64Flag{
		Name:  "shh.pow",
		Usage: "Minimum POW accepted",
		Value: 0.2,
	}
	WhisperRestrictConnectionBetweenLightClientsFlag = cli.BoolFlag{
		Name:  "shh.restrict-light",
		Usage: "Restrict connection between two whisper light clients",
	}

	// Metrics flags

	MetricsEnabledFlag = cli.BoolFlag{
		Name:  "metrics",
		Usage: "Enable metrics collection and reporting",
	}
	MetricsEnabledExpensiveFlag = cli.BoolFlag{
		Name:  "metrics.expensive",
		Usage: "Enable expensive metrics collection and reporting",
	}

	// MetricsHTTPFlag defines the endpoint for a stand-alone metrics HTTP endpoint.
	// Since the pprof service enables sensitive/vulnerable behavior, this allows a user
	// to enable a public-OK metrics endpoint without having to worry about ALSO exposing
	// other profiling behavior or information.
	MetricsHTTPFlag = cli.StringFlag{
		Name:  "metrics.addr",
		Usage: "Enable stand-alone metrics HTTP server listening interface",
		Value: "127.0.0.1",
	}
	MetricsPortFlag = cli.IntFlag{
		Name:  "metrics.port",
		Usage: "Metrics HTTP server listening port",
		Value: 6060,
	}
	MetricsEnableInfluxDBFlag = cli.BoolFlag{
		Name:  "metrics.influxdb",
		Usage: "Enable metrics export/push to an external InfluxDB database",
	}
	MetricsInfluxDBEndpointFlag = cli.StringFlag{
		Name:  "metrics.influxdb.endpoint",
		Usage: "InfluxDB API endpoint to report metrics to",
		Value: "http://localhost:8086",
	}
	MetricsInfluxDBDatabaseFlag = cli.StringFlag{
		Name:  "metrics.influxdb.database",
		Usage: "InfluxDB database name to push reported metrics to",
		Value: "geth",
	}
	MetricsInfluxDBUsernameFlag = cli.StringFlag{
		Name:  "metrics.influxdb.username",
		Usage: "Username to authorize access to the database",
		Value: "test",
	}
	MetricsInfluxDBPasswordFlag = cli.StringFlag{
		Name:  "metrics.influxdb.password",
		Usage: "Password to authorize access to the database",
		Value: "test",
	}
	// Tags are part of every measurement sent to InfluxDB. Queries on tags are faster in InfluxDB.
	// For example `host` tag could be used so that we can group all nodes and average a measurement
	// across all of them, but also so that we can select a specific node and inspect its measurements.
	// https://docs.influxdata.com/influxdb/v1.4/concepts/key_concepts/#tag-key
	MetricsInfluxDBTagsFlag = cli.StringFlag{
		Name:  "metrics.influxdb.tags",
		Usage: "Comma-separated InfluxDB tags (key/values) attached to all measurements",
		Value: "host=localhost",
	}
	MetricsLoadTestCSVFlag = cli.StringFlag{
		Name:  "metrics.loadtestcsvfile",
		Usage: "Write a csv with information about the block production cycle to the given file name. If passed an empty string or non-existent, do not output csv metrics.",
		Value: "",
	}

	EWASMInterpreterFlag = cli.StringFlag{
		Name:  "vm.ewasm",
		Usage: "External ewasm configuration (default = built-in interpreter)",
		Value: "",
	}
	EVMInterpreterFlag = cli.StringFlag{
		Name:  "vm.evm",
		Usage: "External EVM configuration (default = built-in interpreter)",
		Value: "",
	}

	// Istanbul settings

	IstanbulReplicaFlag = cli.BoolFlag{
		Name:  "istanbul.replica",
		Usage: "Run this node as a validator replica. Must be paired with --mine. Use the RPCs to enable participation in consensus.",
	}

	// Announce settings

	AnnounceQueryEnodeGossipPeriodFlag = cli.Uint64Flag{
		Name:  "announce.queryenodegossipperiod",
		Usage: "Time duration (in seconds) between gossiped query enode messages",
		Value: eth.DefaultConfig.Istanbul.AnnounceQueryEnodeGossipPeriod,
	}
	AnnounceAggressiveQueryEnodeGossipOnEnablementFlag = cli.BoolFlag{
		Name:  "announce.aggressivequeryenodegossiponenablement",
		Usage: "Specifies if this node should aggressively query enodes on announce enablement",
	}

	// Proxy node settings

	ProxyFlag = cli.BoolFlag{
		Name:  "proxy.proxy",
		Usage: "Specifies whether this node is a proxy",
	}
	ProxyInternalFacingEndpointFlag = cli.StringFlag{
		Name:  "proxy.internalendpoint",
		Usage: "Specifies the internal facing endpoint for this proxy to listen to.  The format should be <ip address>:<port>",
		Value: ":30503",
	}
	ProxiedValidatorAddressFlag = cli.StringFlag{
		Name:  "proxy.proxiedvalidatoraddress",
		Usage: "Address of the proxied validator",
	}

	// Proxied validator settings

	ProxiedFlag = cli.BoolFlag{
		Name:  "proxy.proxied",
		Usage: "Specifies whether this validator will be proxied by a proxy node",
	}
	ProxyEnodeURLPairsFlag = cli.StringFlag{
		Name:  "proxy.proxyenodeurlpairs",
		Usage: "Each enode URL in a pair is separated by a semicolon. Enode URL pairs are separated by a space. The format should be \"<proxy 0 internal facing enode URL>;<proxy 0 external facing enode URL>,<proxy 1 internal facing enode URL>;<proxy 1 external facing enode URL>,...\"",
	}
	ProxyAllowPrivateIPFlag = cli.BoolFlag{
		Name:  "proxy.allowprivateip",
		Usage: "Specifies whether private IP is allowed for external facing proxy enodeURL",
	}
)

// MakeDataDir retrieves the currently requested data directory, terminating
// if none (or the empty string) is specified. If the node is starting a testnet,
// then a subdirectory of the specified datadir will be used.
func MakeDataDir(ctx *cli.Context) string {
	if path := ctx.GlobalString(DataDirFlag.Name); path != "" {
		if ctx.GlobalBool(BaklavaFlag.Name) {
			return filepath.Join(path, "baklava")
		} else if ctx.GlobalBool(AlfajoresFlag.Name) {
			return filepath.Join(path, "alfajores")
		}
		return path
	}
	Fatalf("Cannot determine default data directory, please set manually (--datadir)")
	return ""
}

// setNodeKey creates a node key from set command line flags, either loading it
// from a file or as a specified hex value. If neither flags were provided, this
// method returns nil and an emphemeral key is to be generated.
func setNodeKey(ctx *cli.Context, cfg *p2p.Config) {
	var (
		hex  = ctx.GlobalString(NodeKeyHexFlag.Name)
		file = ctx.GlobalString(NodeKeyFileFlag.Name)
		key  *ecdsa.PrivateKey
		err  error
	)
	switch {
	case file != "" && hex != "":
		Fatalf("Options %q and %q are mutually exclusive", NodeKeyFileFlag.Name, NodeKeyHexFlag.Name)
	case file != "":
		if key, err = crypto.LoadECDSA(file); err != nil {
			Fatalf("Option %q: %v", NodeKeyFileFlag.Name, err)
		}
		cfg.PrivateKey = key
	case hex != "":
		if key, err = crypto.HexToECDSA(hex); err != nil {
			Fatalf("Option %q: %v", NodeKeyHexFlag.Name, err)
		}
		cfg.PrivateKey = key
	}
}

// setNodeUserIdent creates the user identifier from CLI flags.
func setNodeUserIdent(ctx *cli.Context, cfg *node.Config) {
	if identity := ctx.GlobalString(IdentityFlag.Name); len(identity) > 0 {
		cfg.UserIdent = identity
	}
}

func GetBootstrapNodes(ctx *cli.Context) []string {
	urls := params.MainnetBootnodes
	switch {
	case ctx.GlobalIsSet(BootnodesFlag.Name) || ctx.GlobalIsSet(LegacyBootnodesV4Flag.Name):
		if ctx.GlobalIsSet(LegacyBootnodesV4Flag.Name) {
			urls = SplitAndTrim(ctx.GlobalString(LegacyBootnodesV4Flag.Name))
		} else {
			urls = SplitAndTrim(ctx.GlobalString(BootnodesFlag.Name))
		}
	case ctx.GlobalBool(AlfajoresFlag.Name):
		urls = params.AlfajoresBootnodes
	case ctx.GlobalBool(BaklavaFlag.Name):
		urls = params.BaklavaBootnodes
	}
	return urls
}

// setBootstrapNodes creates a list of bootstrap nodes from the command line
// flags, reverting to pre-configured ones if none have been specified.
func setBootstrapNodes(ctx *cli.Context, cfg *p2p.Config) {
	if cfg.BootstrapNodes != nil {
		return
	}
	urls := GetBootstrapNodes(ctx)

	cfg.BootstrapNodes = make([]*enode.Node, 0, len(urls))
	for _, url := range urls {
		if url != "" {
			node, err := enode.Parse(enode.ValidSchemes, url)
			if err != nil {
				log.Crit("Bootstrap URL invalid", "enode", url, "err", err)
				continue
			}
			cfg.BootstrapNodes = append(cfg.BootstrapNodes, node)
		}
	}
}

// setBootstrapNodesV5 creates a list of bootstrap nodes from the command line
// flags, reverting to pre-configured ones if none have been specified.
func setBootstrapNodesV5(ctx *cli.Context, cfg *p2p.Config) {
	urls := params.MainnetBootnodes
	switch {
	case ctx.GlobalIsSet(BootnodesFlag.Name) || ctx.GlobalIsSet(LegacyBootnodesV5Flag.Name):
		if ctx.GlobalIsSet(LegacyBootnodesV5Flag.Name) {
			urls = SplitAndTrim(ctx.GlobalString(LegacyBootnodesV5Flag.Name))
		} else {
			urls = SplitAndTrim(ctx.GlobalString(BootnodesFlag.Name))
		}
	case ctx.GlobalBool(AlfajoresFlag.Name):
		urls = params.AlfajoresBootnodes
	case ctx.GlobalBool(BaklavaFlag.Name):
		urls = params.BaklavaBootnodes
	case cfg.BootstrapNodesV5 != nil:
		return // already set, don't apply defaults.
	}

	cfg.BootstrapNodesV5 = make([]*discv5.Node, 0, len(urls))
	for _, url := range urls {
		if url != "" {
			node, err := discv5.ParseNode(url)
			if err != nil {
				log.Error("Bootstrap URL invalid", "enode", url, "err", err)
				continue
			}
			cfg.BootstrapNodesV5 = append(cfg.BootstrapNodesV5, node)
		}
	}
}

// setListenAddress creates a TCP listening address string from set command
// line flags.
func setListenAddress(ctx *cli.Context, cfg *p2p.Config) {
	if ctx.GlobalIsSet(ListenPortFlag.Name) {
		cfg.ListenAddr = fmt.Sprintf(":%d", ctx.GlobalInt(ListenPortFlag.Name))
	}
}

// setNAT creates a port mapper from command line flags.
func setNAT(ctx *cli.Context, cfg *p2p.Config) {
	if ctx.GlobalIsSet(NATFlag.Name) {
		natif, err := nat.Parse(ctx.GlobalString(NATFlag.Name))
		if err != nil {
			Fatalf("Option %s: %v", NATFlag.Name, err)
		}
		cfg.NAT = natif
	}
}

// SplitAndTrim splits input separated by a comma
// and trims excessive white space from the substrings.
func SplitAndTrim(input string) (ret []string) {
	l := strings.Split(input, ",")
	for _, r := range l {
		if r = strings.TrimSpace(r); r != "" {
			ret = append(ret, r)
		}
	}
	return ret
}

// setHTTP creates the HTTP RPC listener interface string from the set
// command line flags, returning empty if the HTTP endpoint is disabled.
func setHTTP(ctx *cli.Context, cfg *node.Config) {
	if ctx.GlobalBool(LegacyRPCEnabledFlag.Name) && cfg.HTTPHost == "" {
		log.Warn("The flag --rpc is deprecated and will be removed in the future, please use --http")
		cfg.HTTPHost = "127.0.0.1"
		if ctx.GlobalIsSet(LegacyRPCListenAddrFlag.Name) {
			cfg.HTTPHost = ctx.GlobalString(LegacyRPCListenAddrFlag.Name)
			log.Warn("The flag --rpcaddr is deprecated and will be removed in the future, please use --http.addr")
		}
	}
	if ctx.GlobalBool(HTTPEnabledFlag.Name) && cfg.HTTPHost == "" {
		cfg.HTTPHost = "127.0.0.1"
		if ctx.GlobalIsSet(HTTPListenAddrFlag.Name) {
			cfg.HTTPHost = ctx.GlobalString(HTTPListenAddrFlag.Name)
		}
	}

	if ctx.GlobalIsSet(LegacyRPCPortFlag.Name) {
		cfg.HTTPPort = ctx.GlobalInt(LegacyRPCPortFlag.Name)
		log.Warn("The flag --rpcport is deprecated and will be removed in the future, please use --http.port")
	}
	if ctx.GlobalIsSet(HTTPPortFlag.Name) {
		cfg.HTTPPort = ctx.GlobalInt(HTTPPortFlag.Name)
	}

	if ctx.GlobalIsSet(LegacyRPCCORSDomainFlag.Name) {
		cfg.HTTPCors = SplitAndTrim(ctx.GlobalString(LegacyRPCCORSDomainFlag.Name))
		log.Warn("The flag --rpccorsdomain is deprecated and will be removed in the future, please use --http.corsdomain")
	}
	if ctx.GlobalIsSet(HTTPCORSDomainFlag.Name) {
		cfg.HTTPCors = SplitAndTrim(ctx.GlobalString(HTTPCORSDomainFlag.Name))
	}

	if ctx.GlobalIsSet(LegacyRPCApiFlag.Name) {
		cfg.HTTPModules = SplitAndTrim(ctx.GlobalString(LegacyRPCApiFlag.Name))
		log.Warn("The flag --rpcapi is deprecated and will be removed in the future, please use --http.api")
	}
	if ctx.GlobalIsSet(HTTPApiFlag.Name) {
		cfg.HTTPModules = SplitAndTrim(ctx.GlobalString(HTTPApiFlag.Name))
	}

	if ctx.GlobalIsSet(LegacyRPCVirtualHostsFlag.Name) {
		cfg.HTTPVirtualHosts = SplitAndTrim(ctx.GlobalString(LegacyRPCVirtualHostsFlag.Name))
		log.Warn("The flag --rpcvhosts is deprecated and will be removed in the future, please use --http.vhosts")
	}
	if ctx.GlobalIsSet(HTTPVirtualHostsFlag.Name) {
		cfg.HTTPVirtualHosts = SplitAndTrim(ctx.GlobalString(HTTPVirtualHostsFlag.Name))
	}
}

// setGraphQL creates the GraphQL listener interface string from the set
// command line flags, returning empty if the GraphQL endpoint is disabled.
func setGraphQL(ctx *cli.Context, cfg *node.Config) {
	if ctx.GlobalIsSet(GraphQLCORSDomainFlag.Name) {
		cfg.GraphQLCors = SplitAndTrim(ctx.GlobalString(GraphQLCORSDomainFlag.Name))
	}
	if ctx.GlobalIsSet(GraphQLVirtualHostsFlag.Name) {
		cfg.GraphQLVirtualHosts = SplitAndTrim(ctx.GlobalString(GraphQLVirtualHostsFlag.Name))
	}
}

// setWS creates the WebSocket RPC listener interface string from the set
// command line flags, returning empty if the HTTP endpoint is disabled.
func setWS(ctx *cli.Context, cfg *node.Config) {
	if ctx.GlobalBool(WSEnabledFlag.Name) && cfg.WSHost == "" {
		cfg.WSHost = "127.0.0.1"
		if ctx.GlobalIsSet(LegacyWSListenAddrFlag.Name) {
			cfg.WSHost = ctx.GlobalString(LegacyWSListenAddrFlag.Name)
			log.Warn("The flag --wsaddr is deprecated and will be removed in the future, please use --ws.addr")
		}
		if ctx.GlobalIsSet(WSListenAddrFlag.Name) {
			cfg.WSHost = ctx.GlobalString(WSListenAddrFlag.Name)
		}
	}
	if ctx.GlobalIsSet(LegacyWSPortFlag.Name) {
		cfg.WSPort = ctx.GlobalInt(LegacyWSPortFlag.Name)
		log.Warn("The flag --wsport is deprecated and will be removed in the future, please use --ws.port")
	}
	if ctx.GlobalIsSet(WSPortFlag.Name) {
		cfg.WSPort = ctx.GlobalInt(WSPortFlag.Name)
	}

	if ctx.GlobalIsSet(LegacyWSAllowedOriginsFlag.Name) {
		cfg.WSOrigins = SplitAndTrim(ctx.GlobalString(LegacyWSAllowedOriginsFlag.Name))
		log.Warn("The flag --wsorigins is deprecated and will be removed in the future, please use --ws.origins")
	}
	if ctx.GlobalIsSet(WSAllowedOriginsFlag.Name) {
		cfg.WSOrigins = SplitAndTrim(ctx.GlobalString(WSAllowedOriginsFlag.Name))
	}

	if ctx.GlobalIsSet(LegacyWSApiFlag.Name) {
		cfg.WSModules = SplitAndTrim(ctx.GlobalString(LegacyWSApiFlag.Name))
		log.Warn("The flag --wsapi is deprecated and will be removed in the future, please use --ws.api")
	}
	if ctx.GlobalIsSet(WSApiFlag.Name) {
		cfg.WSModules = SplitAndTrim(ctx.GlobalString(WSApiFlag.Name))
	}
}

// setIPC creates an IPC path configuration from the set command line flags,
// returning an empty string if IPC was explicitly disabled, or the set path.
func setIPC(ctx *cli.Context, cfg *node.Config) {
	CheckExclusive(ctx, IPCDisabledFlag, IPCPathFlag)
	switch {
	case ctx.GlobalBool(IPCDisabledFlag.Name):
		cfg.IPCPath = ""
	case ctx.GlobalIsSet(IPCPathFlag.Name):
		cfg.IPCPath = ctx.GlobalString(IPCPathFlag.Name)
	}
}

// setLes configures the les server and ultra light client settings from the command line flags.
func setLes(ctx *cli.Context, cfg *eth.Config) {
	if ctx.GlobalIsSet(LightServeFlag.Name) {
		cfg.LightServ = ctx.GlobalInt(LightServeFlag.Name)
	}
	if ctx.GlobalIsSet(LightIngressFlag.Name) {
		cfg.LightIngress = ctx.GlobalInt(LightIngressFlag.Name)
	}
	if ctx.GlobalIsSet(LightEgressFlag.Name) {
		cfg.LightEgress = ctx.GlobalInt(LightEgressFlag.Name)
	}
	if ctx.GlobalIsSet(LightMaxPeersFlag.Name) {
		cfg.LightPeers = ctx.GlobalInt(LightMaxPeersFlag.Name)
	}
	if ctx.GlobalIsSet(LightGatewayFeeFlag.Name) {
		cfg.GatewayFee = GlobalBig(ctx, LightGatewayFeeFlag.Name)
	}
	if ctx.GlobalIsSet(UltraLightServersFlag.Name) {
		cfg.UltraLightServers = strings.Split(ctx.GlobalString(UltraLightServersFlag.Name), ",")
	}
	if ctx.GlobalIsSet(UltraLightFractionFlag.Name) {
		cfg.UltraLightFraction = ctx.GlobalInt(UltraLightFractionFlag.Name)
	}
	if cfg.UltraLightFraction <= 0 && cfg.UltraLightFraction > 100 {
		log.Error("Ultra light fraction is invalid", "had", cfg.UltraLightFraction, "updated", eth.DefaultConfig.UltraLightFraction)
		cfg.UltraLightFraction = eth.DefaultConfig.UltraLightFraction
	}
	if ctx.GlobalIsSet(UltraLightOnlyAnnounceFlag.Name) {
		cfg.UltraLightOnlyAnnounce = ctx.GlobalBool(UltraLightOnlyAnnounceFlag.Name)
	}
	if ctx.GlobalBool(DeveloperFlag.Name) {
		// --dev mode can't use p2p networking.
		cfg.LightPeers = 0
	}
	if ctx.GlobalIsSet(LightNoPruneFlag.Name) {
		cfg.LightNoPrune = ctx.GlobalBool(LightNoPruneFlag.Name)
	}
}

// makeDatabaseHandles raises out the number of allowed file handles per process
// for Geth and returns half of the allowance to assign to the database.
func makeDatabaseHandles() int {
	limit, err := fdlimit.Maximum()
	if err != nil {
		Fatalf("Failed to retrieve file descriptor allowance: %v", err)
	}
	raised, err := fdlimit.Raise(uint64(limit))
	if err != nil {
		log.Warn(fmt.Sprintf("Failed to raise file descriptor allowance: %v", err))
		current, err := fdlimit.Current()
		if err != nil {
			Fatalf("Failed to retrieve current file descriptor limit: %v", err)
		}
		return current / 2
	}
	return int(raised / 2) // Leave half for networking and other stuff
}

// MakeAddress converts an account specified directly as a hex encoded string or
// a key index in the key store to an internal account representation.
func MakeAddress(ks *keystore.KeyStore, account string) (accounts.Account, error) {
	// If the specified account is a valid address, return it
	if common.IsHexAddress(account) {
		return accounts.Account{Address: common.HexToAddress(account)}, nil
	}
	// Otherwise try to interpret the account as a keystore index
	index, err := strconv.Atoi(account)
	if err != nil || index < 0 {
		return accounts.Account{}, fmt.Errorf("invalid account address or index %q", account)
	}
	log.Warn("-------------------------------------------------------------------")
	log.Warn("Referring to accounts by order in the keystore folder is dangerous!")
	log.Warn("This functionality is deprecated and will be removed in the future!")
	log.Warn("Please use explicit addresses! (can search via `geth account list`)")
	log.Warn("-------------------------------------------------------------------")

	accs := ks.Accounts()
	if len(accs) <= index {
		return accounts.Account{}, fmt.Errorf("index %d higher than number of accounts %d", index, len(accs))
	}
	return accs[index], nil
}

// setValidator retrieves the validator address either from the directly specified
// command line flags or from the keystore if CLI indexed.
// `Validator` is the address used to sign consensus messages.
func setValidator(ctx *cli.Context, ks *keystore.KeyStore, cfg *eth.Config) {
	// Extract the current validator, new flag overriding legacy etherbase
	var validator string
	if ctx.GlobalIsSet(EtherbaseFlag.Name) {
		validator = ctx.GlobalString(EtherbaseFlag.Name)
	}
	if ctx.GlobalIsSet(LegacyMinerEtherbaseFlag.Name) {
		validator = ctx.GlobalString(LegacyMinerEtherbaseFlag.Name)
	}
	if ctx.GlobalIsSet(MinerValidatorFlag.Name) {
		if validator != "" {
			Fatalf("`etherbase` and `miner.validator` flag should not be used together. `miner.validator` and `tx-fee-recipient` constitute both of `etherbase`' functions")
		}
		validator = ctx.GlobalString(MinerValidatorFlag.Name)
	}

	// Convert the validator into an address and configure it
	if validator != "" {
		account, err := MakeAddress(ks, validator)
		if err != nil {
			Fatalf("Invalid validator: %v", err)
		}
		cfg.Miner.Validator = account.Address
	}
}

// setTxFeeRecipient retrieves the txFeeRecipient address either from the directly specified
// command line flags or from the keystore if CLI indexed.
// `TxFeeRecipient` is the address earned block transaction fees are sent to.
func setTxFeeRecipient(ctx *cli.Context, ks *keystore.KeyStore, cfg *eth.Config) {
	if ctx.GlobalIsSet(TxFeeRecipientFlag.Name) {
		if !ctx.GlobalIsSet(MinerValidatorFlag.Name) {
			Fatalf("`etherbase` and `tx-fee-recipient` flag should not be used together. `miner.validator` and `tx-fee-recipient` constitute both of `etherbase`' functions")
		}
		txFeeRecipient := ctx.GlobalString(TxFeeRecipientFlag.Name)

		// Convert the txFeeRecipient into an address and configure it
		if txFeeRecipient != "" {
			account, err := MakeAddress(ks, txFeeRecipient)
			if err != nil {
				Fatalf("Invalid txFeeRecipient: %v", err)
			}
			cfg.TxFeeRecipient = account.Address
		}
	} else {
		// Backwards compatibility. If the miner was set by the "etherbase" flag, both should have the same info
		cfg.TxFeeRecipient = cfg.Miner.Validator
	}
}

// setBLSbase retrieves the blsbase either from the directly specified
// command line flags or from the keystore if CLI indexed.
// `BLSbase` is the ethereum address which identifies an ECDSA key
// from which the BLS private key used for block finalization in consensus.
func setBLSbase(ctx *cli.Context, ks *keystore.KeyStore, cfg *eth.Config) {
	// Extract the current blsbase, new flag overriding legacy one
	var blsbase string
	if ctx.GlobalIsSet(BLSbaseFlag.Name) {
		blsbase = ctx.GlobalString(BLSbaseFlag.Name)
	}
	// Convert the blsbase into an address and configure it
	if blsbase != "" {
		account, err := MakeAddress(ks, blsbase)
		if err != nil {
			Fatalf("Invalid blsbase: %v", err)
		}
		cfg.BLSbase = account.Address
	}
}

// MakePasswordList reads password lines from the file specified by the global --password flag.
func MakePasswordList(ctx *cli.Context) []string {
	path := ctx.GlobalString(PasswordFileFlag.Name)
	if path == "" {
		return nil
	}
	text, err := ioutil.ReadFile(path)
	if err != nil {
		Fatalf("Failed to read password file: %v", err)
	}
	lines := strings.Split(string(text), "\n")
	// Sanitise DOS line endings.
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

func SetP2PConfig(ctx *cli.Context, cfg *p2p.Config) {
	setNodeKey(ctx, cfg)
	setNAT(ctx, cfg)
	setListenAddress(ctx, cfg)
	setBootstrapNodes(ctx, cfg)
	setBootstrapNodesV5(ctx, cfg)

	cfg.NetworkId = getNetworkId(ctx)

	lightClient := ctx.GlobalString(SyncModeFlag.Name) == "light"
	lightServer := ctx.GlobalInt(LightServeFlag.Name) != 0

	// LightPeers is in the eth config, not the p2p config, so we don't have it here, so we
	// get it here separately using GlobalInt()
	lightPeers := ctx.GlobalInt(LightMaxPeersFlag.Name)
	if lightClient && !ctx.GlobalIsSet(LightMaxPeersFlag.Name) {
		// dynamic default - for clients we use 1/10th of the default for servers
		lightPeers /= 10
	}

	if ctx.GlobalIsSet(MaxPeersFlag.Name) {
		cfg.MaxPeers = ctx.GlobalInt(MaxPeersFlag.Name)
		if lightServer && !ctx.GlobalIsSet(LightMaxPeersFlag.Name) {
			cfg.MaxPeers += lightPeers
		}
	} else {
		if lightServer {
			cfg.MaxPeers += lightPeers
		}
		if lightClient && (ctx.GlobalIsSet(LightMaxPeersFlag.Name)) && cfg.MaxPeers < lightPeers {
			cfg.MaxPeers = lightPeers
		}
	}
	if !(lightClient || lightServer) {
		lightPeers = 0
	}
	ethPeers := cfg.MaxPeers - lightPeers
	if lightClient {
		ethPeers = 0
	}
	if lightServer {
		cfg.MaxLightClients = lightPeers
	}
	log.Info("Maximum peer count", "ETH", ethPeers, "LES", lightPeers, "total", cfg.MaxPeers)

	if ctx.GlobalIsSet(MaxPendingPeersFlag.Name) {
		cfg.MaxPendingPeers = ctx.GlobalInt(MaxPendingPeersFlag.Name)
	}

	if ctx.GlobalBool(NoDiscoverFlag.Name) || lightClient {
		cfg.NoDiscovery = true
	}
	if ctx.GlobalBool(PingIPFromPacketFlag.Name) {
		cfg.PingIPFromPacket = true
	}
	if ctx.GlobalBool(UseInMemoryDiscoverTableFlag.Name) {
		cfg.UseInMemoryNodeDatabase = true
	}

	// if we're running a light client or server, force enable the v5 peer discovery
	// unless it is explicitly disabled with --nodiscover note that explicitly specifying
	// --v5disc overrides --nodiscover, in which case the later only disables v4 discovery
	forceV5Discovery := (lightClient || lightServer) && !ctx.GlobalBool(NoDiscoverFlag.Name)
	if ctx.GlobalIsSet(DiscoveryV5Flag.Name) {
		cfg.DiscoveryV5 = ctx.GlobalBool(DiscoveryV5Flag.Name)
	} else if forceV5Discovery {
		cfg.DiscoveryV5 = true
	}

	if netrestrict := ctx.GlobalString(NetrestrictFlag.Name); netrestrict != "" {
		list, err := netutil.ParseNetlist(netrestrict)
		if err != nil {
			Fatalf("Option %q: %v", NetrestrictFlag.Name, err)
		}
		cfg.NetRestrict = list
	}

	if ctx.GlobalBool(DeveloperFlag.Name) {
		// --dev mode can't use p2p networking.
		cfg.MaxPeers = 0
		cfg.ListenAddr = ":0"
		cfg.NoDiscovery = true
		cfg.DiscoveryV5 = false
	}
}

// SetNodeConfig applies node-related command line flags to the config.
func SetNodeConfig(ctx *cli.Context, cfg *node.Config) {
	SetP2PConfig(ctx, &cfg.P2P)
	setIPC(ctx, cfg)
	setHTTP(ctx, cfg)
	setGraphQL(ctx, cfg)
	setWS(ctx, cfg)
	setNodeUserIdent(ctx, cfg)
	setDataDir(ctx, cfg)

	if ctx.GlobalIsSet(ExternalSignerFlag.Name) {
		cfg.ExternalSigner = ctx.GlobalString(ExternalSignerFlag.Name)
	}

	if ctx.GlobalIsSet(KeyStoreDirFlag.Name) {
		cfg.KeyStoreDir = ctx.GlobalString(KeyStoreDirFlag.Name)
	}
	if ctx.GlobalIsSet(LightKDFFlag.Name) {
		cfg.UseLightweightKDF = ctx.GlobalBool(LightKDFFlag.Name)
	}
	if ctx.GlobalIsSet(NoUSBFlag.Name) {
		cfg.NoUSB = ctx.GlobalBool(NoUSBFlag.Name)
	}
	if ctx.GlobalIsSet(InsecureUnlockAllowedFlag.Name) {
		cfg.InsecureUnlockAllowed = ctx.GlobalBool(InsecureUnlockAllowedFlag.Name)
	}
}

func setDataDir(ctx *cli.Context, cfg *node.Config) {
	switch {
	case ctx.GlobalIsSet(DataDirFlag.Name):
		cfg.DataDir = ctx.GlobalString(DataDirFlag.Name)
	case ctx.GlobalBool(DeveloperFlag.Name):
		cfg.DataDir = "" // unless explicitly requested, use memory databases
	case ctx.GlobalBool(BaklavaFlag.Name) && cfg.DataDir == node.DefaultDataDir():
		log.Info("setting data dir")
		cfg.DataDir = filepath.Join(node.DefaultDataDir(), "baklava")
	case ctx.GlobalBool(AlfajoresFlag.Name) && cfg.DataDir == node.DefaultDataDir():
		cfg.DataDir = filepath.Join(node.DefaultDataDir(), "alfajores")
	}
}

func setTxPool(ctx *cli.Context, cfg *core.TxPoolConfig) {
	if ctx.GlobalIsSet(TxPoolLocalsFlag.Name) {
		locals := strings.Split(ctx.GlobalString(TxPoolLocalsFlag.Name), ",")
		for _, account := range locals {
			if trimmed := strings.TrimSpace(account); !common.IsHexAddress(trimmed) {
				Fatalf("Invalid account in --txpool.locals: %s", trimmed)
			} else {
				cfg.Locals = append(cfg.Locals, common.HexToAddress(account))
			}
		}
	}
	if ctx.GlobalIsSet(TxPoolNoLocalsFlag.Name) {
		cfg.NoLocals = ctx.GlobalBool(TxPoolNoLocalsFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolJournalFlag.Name) {
		cfg.Journal = ctx.GlobalString(TxPoolJournalFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolRejournalFlag.Name) {
		cfg.Rejournal = ctx.GlobalDuration(TxPoolRejournalFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolPriceLimitFlag.Name) {
		cfg.PriceLimit = ctx.GlobalUint64(TxPoolPriceLimitFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolPriceBumpFlag.Name) {
		cfg.PriceBump = ctx.GlobalUint64(TxPoolPriceBumpFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolAccountSlotsFlag.Name) {
		cfg.AccountSlots = ctx.GlobalUint64(TxPoolAccountSlotsFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolGlobalSlotsFlag.Name) {
		cfg.GlobalSlots = ctx.GlobalUint64(TxPoolGlobalSlotsFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolAccountQueueFlag.Name) {
		cfg.AccountQueue = ctx.GlobalUint64(TxPoolAccountQueueFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolGlobalQueueFlag.Name) {
		cfg.GlobalQueue = ctx.GlobalUint64(TxPoolGlobalQueueFlag.Name)
	}
	if ctx.GlobalIsSet(TxPoolLifetimeFlag.Name) {
		cfg.Lifetime = ctx.GlobalDuration(TxPoolLifetimeFlag.Name)
	}
}

func setMiner(ctx *cli.Context, cfg *miner.Config) {
	if ctx.GlobalIsSet(LegacyMinerExtraDataFlag.Name) {
		cfg.ExtraData = []byte(ctx.GlobalString(LegacyMinerExtraDataFlag.Name))
		log.Warn("The flag --extradata is deprecated and will be removed in the future, please use --miner.extradata")
	}
	if ctx.GlobalIsSet(MinerExtraDataFlag.Name) {
		cfg.ExtraData = []byte(ctx.GlobalString(MinerExtraDataFlag.Name))
	}
}

func setWhitelist(ctx *cli.Context, cfg *eth.Config) {
	whitelist := ctx.GlobalString(WhitelistFlag.Name)
	if whitelist == "" {
		return
	}
	cfg.Whitelist = make(map[uint64]common.Hash)
	for _, entry := range strings.Split(whitelist, ",") {
		parts := strings.Split(entry, "=")
		if len(parts) != 2 {
			Fatalf("Invalid whitelist entry: %s", entry)
		}
		number, err := strconv.ParseUint(parts[0], 0, 64)
		if err != nil {
			Fatalf("Invalid whitelist block number %s: %v", parts[0], err)
		}
		var hash common.Hash
		if err = hash.UnmarshalText([]byte(parts[1])); err != nil {
			Fatalf("Invalid whitelist hash %s: %v", parts[1], err)
		}
		cfg.Whitelist[number] = hash
	}
}

func setIstanbul(ctx *cli.Context, stack *node.Node, cfg *eth.Config) {
	if ctx.GlobalIsSet(LegacyIstanbulRequestTimeoutFlag.Name) {
		log.Warn("Flag value is ignored, and obtained from genesis config", "flag", LegacyIstanbulRequestTimeoutFlag.Name)
	}
	if ctx.GlobalIsSet(LegacyIstanbulBlockPeriodFlag.Name) {
		log.Warn("Flag value is ignored, and obtained from genesis config", "flag", LegacyIstanbulBlockPeriodFlag.Name)
	}
	if ctx.GlobalIsSet(LegacyIstanbulLookbackWindowFlag.Name) {
		log.Warn("Flag value is ignored, and obtained from genesis config", "flag", LegacyIstanbulLookbackWindowFlag.Name)
	}
	if ctx.GlobalIsSet(LegacyIstanbulProposerPolicyFlag.Name) {
		log.Warn("Flag value is ignored, and obtained from genesis config", "flag", LegacyIstanbulProposerPolicyFlag.Name)
	}
	cfg.Istanbul.ReplicaStateDBPath = stack.ResolvePath(cfg.Istanbul.ReplicaStateDBPath)
	cfg.Istanbul.ValidatorEnodeDBPath = stack.ResolvePath(cfg.Istanbul.ValidatorEnodeDBPath)
	cfg.Istanbul.VersionCertificateDBPath = stack.ResolvePath(cfg.Istanbul.VersionCertificateDBPath)
	cfg.Istanbul.RoundStateDBPath = stack.ResolvePath(cfg.Istanbul.RoundStateDBPath)
	cfg.Istanbul.Validator = ctx.GlobalIsSet(MiningEnabledFlag.Name) || ctx.GlobalIsSet(DeveloperFlag.Name)
	cfg.Istanbul.Replica = ctx.GlobalIsSet(IstanbulReplicaFlag.Name)
	if ctx.GlobalIsSet(MetricsLoadTestCSVFlag.Name) {
		cfg.Istanbul.LoadTestCSVFile = ctx.GlobalString(MetricsLoadTestCSVFlag.Name)
	}
}

func setProxyP2PConfig(ctx *cli.Context, proxyCfg *p2p.Config) {
	setNodeKey(ctx, proxyCfg)
	setNAT(ctx, proxyCfg)
	if ctx.GlobalIsSet(ProxyInternalFacingEndpointFlag.Name) {
		proxyCfg.ListenAddr = ctx.GlobalString(ProxyInternalFacingEndpointFlag.Name)
	}

	proxyCfg.NetworkId = getNetworkId(ctx)
}

// Set all of the proxy related configurations.
// These configs span the top level node and istanbul module configuration
func SetProxyConfig(ctx *cli.Context, nodeCfg *node.Config, ethCfg *eth.Config) {
	CheckExclusive(ctx, ProxyFlag, ProxiedFlag)

	if ctx.GlobalIsSet(ProxyFlag.Name) {
		nodeCfg.Proxy = ctx.GlobalBool(ProxyFlag.Name)
		ethCfg.Istanbul.Proxy = ctx.GlobalBool(ProxyFlag.Name)

		// Mining must not be set for proxies
		if ctx.GlobalIsSet(MiningEnabledFlag.Name) {
			Fatalf("Option --%s must not be used if option --%s is used", MiningEnabledFlag.Name, ProxyFlag.Name)
		}
		// Replica must not be set for proxies
		if ctx.GlobalIsSet(IstanbulReplicaFlag.Name) {
			Fatalf("Option --%s must not be used if option --%s is used", IstanbulReplicaFlag.Name, ProxyFlag.Name)
		}

		if !ctx.GlobalIsSet(ProxiedValidatorAddressFlag.Name) {
			Fatalf("Option --%s must be used if option --%s is used", ProxiedValidatorAddressFlag.Name, ProxyFlag.Name)
		} else {
			proxiedValidatorAddress := ctx.String(ProxiedValidatorAddressFlag.Name)
			if !common.IsHexAddress(proxiedValidatorAddress) {
				Fatalf("Invalid address used for option --%s", ProxiedValidatorAddressFlag.Name)
			}
			ethCfg.Istanbul.ProxiedValidatorAddress = common.HexToAddress(proxiedValidatorAddress)
		}

		if !ctx.GlobalIsSet(ProxyInternalFacingEndpointFlag.Name) {
			Fatalf("Option --%s must be used if option --%s is used", ProxyInternalFacingEndpointFlag.Name, ProxyFlag.Name)
		} else {
			setProxyP2PConfig(ctx, &nodeCfg.ProxyP2P)
		}
	}

	if ctx.GlobalIsSet(ProxiedFlag.Name) {
		ethCfg.Istanbul.Proxied = ctx.GlobalBool(ProxiedFlag.Name)

		// Mining must be set for proxied nodes
		if !ctx.GlobalIsSet(MiningEnabledFlag.Name) {
			Fatalf("Option --%s must be used if option --%s is used", MiningEnabledFlag.Name, ProxiedFlag.Name)
		}

		// Extract the proxy enode url pairs, new flag overriding legacy one
		var proxyEnodeURLPairs []string

		if ctx.GlobalIsSet(LegacyProxyEnodeURLPairsFlag.Name) {
			proxyEnodeURLPairs = strings.Split(ctx.String(LegacyProxyEnodeURLPairsFlag.Name), ",")
			log.Warn("The flag --proxy.proxyenodeurlpair is deprecated and will be removed in the future, please use --proxy.proxyenodeurlpairs")
		}

		if ctx.GlobalIsSet(ProxyEnodeURLPairsFlag.Name) {
			proxyEnodeURLPairs = strings.Split(ctx.String(ProxyEnodeURLPairsFlag.Name), ",")
		}
		ethCfg.Istanbul.ProxyConfigs = make([]*istanbul.ProxyConfig, len(proxyEnodeURLPairs))

		for i, proxyEnodeURLPairStr := range proxyEnodeURLPairs {
			proxyEnodeURLPair := strings.Split(proxyEnodeURLPairStr, ";")
			if len(proxyEnodeURLPair) != 2 {
				Fatalf("Invalid format for option --%s", ProxyEnodeURLPairsFlag.Name)
			}

			proxyInternalNode, err := enode.ParseV4(proxyEnodeURLPair[0])
			if err != nil {
				Fatalf("Proxy internal facing enodeURL (%s) invalid with parse err: %v", proxyEnodeURLPair[0], err)
			}

			proxyExternalNode, err := enode.ParseV4(proxyEnodeURLPair[1])
			if err != nil {
				Fatalf("Proxy external facing enodeURL (%s) invalid with parse err: %v", proxyEnodeURLPair[1], err)
			}

			// Check that external IP is not a private IP address.
			if proxyExternalNode.IsPrivateIP() {
				if ctx.GlobalBool(ProxyAllowPrivateIPFlag.Name) {
					log.Warn("Proxy external facing enodeURL (%s) is private IP.", "proxy external enodeURL", proxyEnodeURLPair[1])
				} else {
					Fatalf("Proxy external facing enodeURL (%s) cannot be private IP.", "proxy external enodeURL", proxyEnodeURLPair[1])
				}
			}
			ethCfg.Istanbul.ProxyConfigs[i] = &istanbul.ProxyConfig{
				InternalNode: proxyInternalNode,
				ExternalNode: proxyExternalNode,
			}
		}

		if !ctx.GlobalBool(NoDiscoverFlag.Name) {
			Fatalf("Option --%s must be used if option --%s is used", NoDiscoverFlag.Name, ProxiedFlag.Name)
		}
	}
}

// checkExclusive verifies that only a single instance of the provided flags was
// set by the user. Each flag might optionally be followed by a string type to
// specialize it further.
func CheckExclusive(ctx *cli.Context, args ...interface{}) {
	set := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		// Make sure the next argument is a flag and skip if not set
		flag, ok := args[i].(cli.Flag)
		if !ok {
			panic(fmt.Sprintf("invalid argument, not cli.Flag type: %T", args[i]))
		}
		// Check if next arg extends current and expand its name if so
		name := flag.GetName()

		if i+1 < len(args) {
			switch option := args[i+1].(type) {
			case string:
				// Extended flag check, make sure value set doesn't conflict with passed in option
				if ctx.GlobalString(flag.GetName()) == option {
					name += "=" + option
					set = append(set, "--"+name)
				}
				// shift arguments and continue
				i++
				continue

			case cli.Flag:
			default:
				panic(fmt.Sprintf("invalid argument, not cli.Flag or string extension: %T", args[i+1]))
			}
		}
		// Mark the flag if it's set
		if ctx.GlobalIsSet(flag.GetName()) {
			set = append(set, "--"+name)
		}
	}
	if len(set) > 1 {
		Fatalf("Flags %v can't be used at the same time", strings.Join(set, ", "))
	}
}

// SetShhConfig applies shh-related command line flags to the config.
func SetShhConfig(ctx *cli.Context, stack *node.Node) {
	if ctx.GlobalIsSet(WhisperEnabledFlag.Name) ||
		ctx.GlobalIsSet(WhisperMaxMessageSizeFlag.Name) ||
		ctx.GlobalIsSet(WhisperMinPOWFlag.Name) ||
		ctx.GlobalIsSet(WhisperRestrictConnectionBetweenLightClientsFlag.Name) {
		log.Warn("Whisper support has been deprecated and the code has been moved to github.com/ethereum/whisper")
	}
}

func getNetworkId(ctx *cli.Context) uint64 {
	if ctx.GlobalIsSet(NetworkIdFlag.Name) {
		return ctx.GlobalUint64(NetworkIdFlag.Name)
	}
	switch {
	case ctx.GlobalBool(BaklavaFlag.Name):
		return params.BaklavaNetworkId
	case ctx.GlobalBool(AlfajoresFlag.Name):
		return params.AlfajoresNetworkId
	case ctx.GlobalBool(DeveloperFlag.Name):
		return 1337
	}
	return params.MainnetNetworkId
}

// SetEthConfig applies eth-related command line flags to the config.
func SetEthConfig(ctx *cli.Context, stack *node.Node, cfg *eth.Config) {
	// Avoid conflicting network flags
	CheckExclusive(ctx, DeveloperFlag, BaklavaFlag, AlfajoresFlag)
	CheckExclusive(ctx, LightServeFlag, SyncModeFlag, "light")
	CheckExclusive(ctx, DeveloperFlag, ExternalSignerFlag) // Can't use both ephemeral unlocked and external signer
	CheckExclusive(ctx, GCModeFlag, "archive", TxLookupLimitFlag)
	// todo(rjl493456442) make it available for les server
	// Ancient tx indices pruning is not available for les server now
	// since light client relies on the server for transaction status query.
	CheckExclusive(ctx, LegacyLightServFlag, LightServeFlag, TxLookupLimitFlag)
	var ks *keystore.KeyStore
	if keystores := stack.AccountManager().Backends(keystore.KeyStoreType); len(keystores) > 0 {
		ks = keystores[0].(*keystore.KeyStore)
	}
	setValidator(ctx, ks, cfg)
	setTxFeeRecipient(ctx, ks, cfg)
	setBLSbase(ctx, ks, cfg)
	setTxPool(ctx, &cfg.TxPool)
	setMiner(ctx, &cfg.Miner)
	setWhitelist(ctx, cfg)
	setIstanbul(ctx, stack, cfg)
	setLes(ctx, cfg)

	cfg.NetworkId = params.MainnetNetworkId

	if ctx.GlobalIsSet(SyncModeFlag.Name) {
		cfg.SyncMode = *GlobalTextMarshaler(ctx, SyncModeFlag.Name).(*downloader.SyncMode)
	}
	if ctx.GlobalIsSet(NetworkIdFlag.Name) {
		cfg.NetworkId = ctx.GlobalUint64(NetworkIdFlag.Name)
	}
	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheDatabaseFlag.Name) {
		cfg.DatabaseCache = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheDatabaseFlag.Name) / 100
	}
	cfg.DatabaseHandles = makeDatabaseHandles()
	if ctx.GlobalIsSet(AncientFlag.Name) {
		cfg.DatabaseFreezer = ctx.GlobalString(AncientFlag.Name)
	}

	if gcmode := ctx.GlobalString(GCModeFlag.Name); gcmode != "full" && gcmode != "archive" {
		Fatalf("--%s must be either 'full' or 'archive'", GCModeFlag.Name)
	}
	if ctx.GlobalIsSet(GCModeFlag.Name) {
		cfg.NoPruning = ctx.GlobalString(GCModeFlag.Name) == "archive"
	}
	if ctx.GlobalIsSet(CacheNoPrefetchFlag.Name) {
		cfg.NoPrefetch = ctx.GlobalBool(CacheNoPrefetchFlag.Name)
	}
	// Read the value from the flag no matter if it's set or not.
	cfg.Preimages = ctx.GlobalBool(CachePreimagesFlag.Name)
	if cfg.NoPruning && !cfg.Preimages {
		cfg.Preimages = true
		log.Info("Enabling recording of key preimages since archive mode is used")
	}
	if ctx.GlobalIsSet(TxLookupLimitFlag.Name) {
		cfg.TxLookupLimit = ctx.GlobalUint64(TxLookupLimitFlag.Name)
	}
	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheTrieFlag.Name) {
		cfg.TrieCleanCache = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheTrieFlag.Name) / 100
	}
	if ctx.GlobalIsSet(CacheTrieJournalFlag.Name) {
		cfg.TrieCleanCacheJournal = ctx.GlobalString(CacheTrieJournalFlag.Name)
	}
	if ctx.GlobalIsSet(CacheTrieRejournalFlag.Name) {
		cfg.TrieCleanCacheRejournal = ctx.GlobalDuration(CacheTrieRejournalFlag.Name)
	}
	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheGCFlag.Name) {
		cfg.TrieDirtyCache = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheGCFlag.Name) / 100
	}
	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheSnapshotFlag.Name) {
		cfg.SnapshotCache = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheSnapshotFlag.Name) / 100
	}
	if !ctx.GlobalBool(SnapshotFlag.Name) {
		cfg.TrieCleanCache += cfg.SnapshotCache
		cfg.SnapshotCache = 0 // Disabled
	}
	if ctx.GlobalIsSet(DocRootFlag.Name) {
		cfg.DocRoot = ctx.GlobalString(DocRootFlag.Name)
	}
	if ctx.GlobalIsSet(VMEnableDebugFlag.Name) {
		// TODO(fjl): force-enable this in --dev mode
		cfg.EnablePreimageRecording = ctx.GlobalBool(VMEnableDebugFlag.Name)
	}

	if ctx.GlobalIsSet(EWASMInterpreterFlag.Name) {
		cfg.EWASMInterpreter = ctx.GlobalString(EWASMInterpreterFlag.Name)
	}

	if ctx.GlobalIsSet(EVMInterpreterFlag.Name) {
		cfg.EVMInterpreter = ctx.GlobalString(EVMInterpreterFlag.Name)
	}
	if ctx.GlobalIsSet(RPCGlobalGasCapFlag.Name) {
		cfg.RPCGasCap = ctx.GlobalUint64(RPCGlobalGasCapFlag.Name)
	}
	if cfg.RPCGasCap != 0 {
		log.Info("Set global gas cap", "cap", cfg.RPCGasCap)
	} else {
		log.Info("Global gas cap disabled")
	}
	if ctx.GlobalIsSet(RPCGlobalTxFeeCapFlag.Name) {
		cfg.RPCTxFeeCap = ctx.GlobalFloat64(RPCGlobalTxFeeCapFlag.Name)
	}

	// Disable DNS discovery by default (by using the flag's value even if it hasn't been set and so
	// has the default value ""), since we don't have DNS discovery set up for Celo.
	// Note that passing --discovery.dns "" is the way the Geth docs specify for disabling DNS discovery,
	// so here we just make that be the default.
	if ctx.GlobalIsSet(NoDiscoverFlag.Name) {
		cfg.DiscoveryURLs = []string{}
	} else if urls := ctx.GlobalString(DNSDiscoveryFlag.Name); urls == "" {
		cfg.DiscoveryURLs = []string{}
	} else {
		cfg.DiscoveryURLs = SplitAndTrim(urls)
	}

	// Override any default configs for hard coded networks.
	switch {
	case ctx.GlobalBool(BaklavaFlag.Name):
		if !ctx.GlobalIsSet(NetworkIdFlag.Name) {
			log.Info("Setting baklava id")
			cfg.NetworkId = params.BaklavaNetworkId
		}
		cfg.Genesis = core.DefaultBaklavaGenesisBlock()
		SetDNSDiscoveryDefaults(cfg, params.BaklavaGenesisHash)
	case ctx.GlobalBool(AlfajoresFlag.Name):
		if !ctx.GlobalIsSet(NetworkIdFlag.Name) {
			cfg.NetworkId = params.AlfajoresNetworkId
		}
		cfg.Genesis = core.DefaultAlfajoresGenesisBlock()
		SetDNSDiscoveryDefaults(cfg, params.AlfajoresGenesisHash)
	case ctx.GlobalBool(DeveloperFlag.Name):
		if !ctx.GlobalIsSet(NetworkIdFlag.Name) {
			cfg.NetworkId = 1337
		}
		// Create new developer account or reuse existing one
		var (
			developer  accounts.Account
			passphrase string
			err        error
		)
		if list := MakePasswordList(ctx); len(list) > 0 {
			// Just take the first value. Although the function returns a possible multiple values and
			// some usages iterate through them as attempts, that doesn't make sense in this setting,
			// when we're definitely concerned with only one account.
			passphrase = list[0]
		}
		// setValidator has been called above, configuring the miner address from command line flags.
		if cfg.Miner.Validator != (common.Address{}) {
			developer = accounts.Account{Address: cfg.Miner.Validator}
		} else if accs := ks.Accounts(); len(accs) > 0 {
			developer = ks.Accounts()[0]
		} else {
			key, _ := crypto.HexToECDSA("add67e37fdf5c26743d295b1af6d9b50f2785a6b60bc83a8f05bd1dd4b385c6c")
			developer, err = ks.ImportECDSA(key, passphrase)
			if err != nil {
				Fatalf("Failed to create developer account: %v", err)
			}
		}
		if err := ks.Unlock(developer, passphrase); err != nil {
			Fatalf("Failed to unlock developer account: %v", err)
		}
		log.Info("Using developer account", "address", developer.Address)

		// Create a new developer genesis block or reuse existing one
		cfg.Genesis = core.DeveloperGenesisBlock()
		if ctx.GlobalIsSet(DataDirFlag.Name) {
			// Check if we have an already initialized chain and fall back to
			// that if so. Otherwise we need to generate a new genesis spec.
			chaindb := MakeChainDatabase(ctx, stack)
			if rawdb.ReadCanonicalHash(chaindb, 0) != (common.Hash{}) {
				cfg.Genesis = nil // fallback to db content
			}
			chaindb.Close()
		}
	default:
		if cfg.NetworkId == params.MainnetNetworkId {
			SetDNSDiscoveryDefaults(cfg, params.MainnetGenesisHash)
		}
	}
}

// SetDNSDiscoveryDefaults configures DNS discovery with the given URL if
// no URLs are set.
func SetDNSDiscoveryDefaults(cfg *eth.Config, genesis common.Hash) {
	if cfg.DiscoveryURLs != nil {
		return // already set through flags/config
	}

	protocol := "all"
	if cfg.SyncMode == downloader.LightSync {
		protocol = "les"
	}
	if url := params.KnownDNSNetwork(genesis, protocol); url != "" {
		cfg.DiscoveryURLs = []string{url}
	}
}

// RegisterEthService adds an Ethereum client to the stack.
func RegisterEthService(stack *node.Node, cfg *eth.Config) ethapi.Backend {
	if !cfg.SyncMode.SyncFullBlockChain() {
		backend, err := les.New(stack, cfg)
		if err != nil {
			Fatalf("Failed to register the Ethereum service: %v", err)
		}
		return backend.ApiBackend
	}
	backend, err := eth.New(stack, cfg)
	if err != nil {
		Fatalf("Failed to register the Ethereum service: %v", err)
	}
	if cfg.LightServ > 0 {
		_, err := les.NewLesServer(stack, backend, cfg)
		if err != nil {
			Fatalf("Failed to create the LES server: %v", err)
		}
	}
	return backend.APIBackend
}

// RegisterEthStatsService configures the Ethereum Stats daemon and adds it to
// the given node.
func RegisterEthStatsService(stack *node.Node, backend ethapi.Backend, url string) {
	if err := ethstats.New(stack, backend, backend.Engine(), url); err != nil {
		Fatalf("Failed to register the Ethereum Stats service: %v", err)
	}
}

// RegisterGraphQLService is a utility function to construct a new service and register it against a node.
func RegisterGraphQLService(stack *node.Node, backend ethapi.Backend, cfg node.Config) {
	if err := graphql.New(stack, backend, cfg.GraphQLCors, cfg.GraphQLVirtualHosts); err != nil {
		Fatalf("Failed to register the GraphQL service: %v", err)
	}
}

func SetupMetrics(ctx *cli.Context) {
	if metrics.Enabled {
		log.Info("Enabling metrics collection")

		var (
			enableExport = ctx.GlobalBool(MetricsEnableInfluxDBFlag.Name)
			endpoint     = ctx.GlobalString(MetricsInfluxDBEndpointFlag.Name)
			database     = ctx.GlobalString(MetricsInfluxDBDatabaseFlag.Name)
			username     = ctx.GlobalString(MetricsInfluxDBUsernameFlag.Name)
			password     = ctx.GlobalString(MetricsInfluxDBPasswordFlag.Name)
		)

		if enableExport {
			tagsMap := SplitTagsFlag(ctx.GlobalString(MetricsInfluxDBTagsFlag.Name))

			log.Info("Enabling metrics export to InfluxDB")

			go influxdb.InfluxDBWithTags(metrics.DefaultRegistry, 10*time.Second, endpoint, database, username, password, "geth.", tagsMap)
		}

		if ctx.GlobalIsSet(MetricsHTTPFlag.Name) {
			address := fmt.Sprintf("%s:%d", ctx.GlobalString(MetricsHTTPFlag.Name), ctx.GlobalInt(MetricsPortFlag.Name))
			log.Info("Enabling stand-alone metrics HTTP endpoint", "address", address)
			exp.Setup(address)
		}
	}
}

func SplitTagsFlag(tagsFlag string) map[string]string {
	tags := strings.Split(tagsFlag, ",")
	tagsMap := map[string]string{}

	for _, t := range tags {
		if t != "" {
			kv := strings.Split(t, "=")

			if len(kv) == 2 {
				tagsMap[kv[0]] = kv[1]
			}
		}
	}

	return tagsMap
}

// MakeChainDatabase open an LevelDB using the flags passed to the client and will hard crash if it fails.
func MakeChainDatabase(ctx *cli.Context, stack *node.Node) ethdb.Database {
	var (
		cache   = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheDatabaseFlag.Name) / 100
		handles = makeDatabaseHandles()

		err     error
		chainDb ethdb.Database
	)
	if ctx.GlobalString(SyncModeFlag.Name) == "light" {
		name := "lightchaindata"
		chainDb, err = stack.OpenDatabase(name, cache, handles, "")
	} else if ctx.GlobalString(SyncModeFlag.Name) == "lightest" {
		name := "lightestchaindata"
		chainDb, err = stack.OpenDatabaseWithFreezer(name, cache, handles, ctx.GlobalString(AncientFlag.Name), "")
	} else {
		name := "chaindata"
		chainDb, err = stack.OpenDatabaseWithFreezer(name, cache, handles, ctx.GlobalString(AncientFlag.Name), "")
	}
	if err != nil {
		Fatalf("Could not open database: %v", err)
	}
	return chainDb
}

func MakeGenesis(ctx *cli.Context) *core.Genesis {
	var genesis *core.Genesis
	switch {
	case ctx.GlobalBool(BaklavaFlag.Name):
		genesis = core.DefaultBaklavaGenesisBlock()
	case ctx.GlobalBool(AlfajoresFlag.Name):
		genesis = core.DefaultAlfajoresGenesisBlock()
	case ctx.GlobalBool(DeveloperFlag.Name):
		Fatalf("Developer chains are ephemeral")
	}
	return genesis
}

// MakeChain creates a chain manager from set command line flags.
func MakeChain(ctx *cli.Context, stack *node.Node, readOnly bool) (chain *core.BlockChain, chainDb ethdb.Database) {
	var err error
	chainDb = MakeChainDatabase(ctx, stack)
	config, _, err := core.SetupGenesisBlock(chainDb, MakeGenesis(ctx))
	if err != nil {
		Fatalf("%v", err)
	}
	config.FullHeaderChainAvailable = ctx.GlobalString(SyncModeFlag.Name) != "lightest"
	engine := mockEngine.NewFaker()

	if gcmode := ctx.GlobalString(GCModeFlag.Name); gcmode != "full" && gcmode != "archive" {
		Fatalf("--%s must be either 'full' or 'archive'", GCModeFlag.Name)
	}
	cache := &core.CacheConfig{
		TrieCleanLimit:      eth.DefaultConfig.TrieCleanCache,
		TrieCleanNoPrefetch: ctx.GlobalBool(CacheNoPrefetchFlag.Name),
		TrieDirtyLimit:      eth.DefaultConfig.TrieDirtyCache,
		TrieDirtyDisabled:   ctx.GlobalString(GCModeFlag.Name) == "archive",
		TrieTimeLimit:       eth.DefaultConfig.TrieTimeout,
		SnapshotLimit:       eth.DefaultConfig.SnapshotCache,
		Preimages:           ctx.GlobalBool(CachePreimagesFlag.Name),
	}
	if cache.TrieDirtyDisabled && !cache.Preimages {
		cache.Preimages = true
		log.Info("Enabling recording of key preimages since archive mode is used")
	}
	if !ctx.GlobalBool(SnapshotFlag.Name) {
		cache.SnapshotLimit = 0 // Disabled
	}
	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheTrieFlag.Name) {
		cache.TrieCleanLimit = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheTrieFlag.Name) / 100
	}
	if ctx.GlobalIsSet(CacheFlag.Name) || ctx.GlobalIsSet(CacheGCFlag.Name) {
		cache.TrieDirtyLimit = ctx.GlobalInt(CacheFlag.Name) * ctx.GlobalInt(CacheGCFlag.Name) / 100
	}
	vmcfg := vm.Config{EnablePreimageRecording: ctx.GlobalBool(VMEnableDebugFlag.Name)}
	var limit *uint64
	if ctx.GlobalIsSet(TxLookupLimitFlag.Name) && !readOnly {
		l := ctx.GlobalUint64(TxLookupLimitFlag.Name)
		limit = &l
	}
	chain, err = core.NewBlockChain(chainDb, cache, config, engine, vmcfg, nil, limit)
	if err != nil {
		Fatalf("Can't create BlockChain: %v", err)
	}
	return chain, chainDb
}

// MakeConsolePreloads retrieves the absolute paths for the console JavaScript
// scripts to preload before starting.
func MakeConsolePreloads(ctx *cli.Context) []string {
	// Skip preloading if there's nothing to preload
	if ctx.GlobalString(PreloadJSFlag.Name) == "" {
		return nil
	}
	// Otherwise resolve absolute paths and return them
	var preloads []string

	assets := ctx.GlobalString(JSpathFlag.Name)
	for _, file := range strings.Split(ctx.GlobalString(PreloadJSFlag.Name), ",") {
		preloads = append(preloads, common.AbsolutePath(assets, strings.TrimSpace(file)))
	}
	return preloads
}

// MigrateFlags sets the global flag from a local flag when it's set.
// This is a temporary function used for migrating old command/flags to the
// new format.
//
// e.g. geth account new --keystore /tmp/mykeystore --lightkdf
//
// is equivalent after calling this method with:
//
// geth --keystore /tmp/mykeystore --lightkdf account new
//
// This allows the use of the existing configuration functionality.
// When all flags are migrated this function can be removed and the existing
// configuration functionality must be changed that is uses local flags
func MigrateFlags(action func(ctx *cli.Context) error) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		for _, name := range ctx.FlagNames() {
			if ctx.IsSet(name) {
				ctx.GlobalSet(name, ctx.String(name))
			}
		}
		return action(ctx)
	}
}
