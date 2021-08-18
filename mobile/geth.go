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

// Contains all the wrappers from the node package to support client side node
// management on mobile platforms.

package geth

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/eth"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/ethstats"
	"github.com/celo-org/celo-blockchain/internal/debug"
	"github.com/celo-org/celo-blockchain/les"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/nat"
	"github.com/celo-org/celo-blockchain/params"
)

// I am intentionally duplicating these constants different from downloader.SyncMode integer values, to ensure the
// backwards compatibility where the mobile node defaults to LightSync.
const SyncModeUnset = 0 // will be treated as SyncModeLightSync
const SyncModeFullSync = 1
const SyncModeFastSync = 2
const SyncModeLightSync = 3

// Deprecated: This used to be SyncModeCeloLatestSync. Geth will panic if started in this mode.
// Use LightestSync instead.
const SyncModeDeprecatedSync = 4
const LightestSync = 5

// NodeConfig represents the collection of configuration values to fine tune the Geth
// node embedded into a mobile process. The available values are a subset of the
// entire API provided by go-ethereum to reduce the maintenance surface and dev
// complexity.
type NodeConfig struct {
	// Bootstrap nodes used to establish connectivity with the rest of the network.
	BootstrapNodes *Enodes

	// MaxPeers is the maximum number of peers that can be connected. If this is
	// set to zero, then only the configured static and trusted peers can connect.
	MaxPeers int

	// NoDiscovery indicates whether the node should not participate in p2p discovery
	NoDiscovery bool

	// EthereumEnabled specifies whether the node should run the Ethereum protocol.
	EthereumEnabled bool

	// EthereumNetworkID is the network identifier used by the Ethereum protocol to
	// decide if remote peers should be accepted or not.
	EthereumNetworkID int64 // uint64 in truth, but Java can't handle that...

	// EthereumGenesis is the genesis JSON to use to seed the blockchain with. An
	// empty genesis state is equivalent to using the mainnet's state.
	EthereumGenesis string

	// EthereumDatabaseCache is the system memory in MB to allocate for database caching.
	// A minimum of 16MB is always reserved.
	EthereumDatabaseCache int

	// EthereumNetStats is a netstats connection string to use to report various
	// chain, transaction and node stats to a monitoring server.
	//
	// It has the form "nodename:secret@host:port"
	EthereumNetStats string

	// HTTPHost is the host interface on which to start the HTTP RPC server. If this
	// field is empty, no HTTP API endpoint will be started.
	HTTPHost string

	// HTTPPort is the TCP port number on which to start the HTTP RPC server. The
	// default zero value is/ valid and will pick a port number randomly (useful
	// for ephemeral nodes).
	HTTPPort int

	// HTTPVirtualHosts is a comma separated list of virtual hostnames which are allowed on incoming requests.
	// This is by default {'localhost'}. Using this prevents attacks like
	// DNS rebinding, which bypasses SOP by simply masquerading as being within the same
	// origin. These attacks do not utilize CORS, since they are not cross-domain.
	// By explicitly checking the Host-header, the server will not allow requests
	// made against the server with a malicious host domain.
	// Requests using ip address directly are not affected
	HTTPVirtualHosts string

	// HTTPModules is a comma separated list of API modules to expose via the HTTP RPC interface.
	// If the module list is empty, all RPC API endpoints designated public will be
	// exposed.
	HTTPModules string

	// Listening address of pprof server.
	PprofAddress string

	// Sync mode for the node (eth/downloader/modes.go)
	// This has to be integer since Enum exports to Java are not supported by "gomobile"
	// See getSyncMode(syncMode int)
	SyncMode int

	// UseLightweightKDF lowers the memory and CPU requirements of the key store
	// scrypt KDF at the expense of security.
	// See https://geth.ethereum.org/doc/Mobile_Account-management for reference
	UseLightweightKDF bool

	// IPCPath is the requested location to place the IPC endpoint. If the path is
	// a simple file name, it is placed inside the data directory (or on the root
	// pipe path on Windows), whereas if it's a resolvable path name (absolute or
	// relative), then that specific path is enforced. An empty path disables IPC.
	IPCPath string
}

// defaultNodeConfig contains the default node configuration values to use if all
// or some fields are missing from the user's specified list.
var defaultNodeConfig = &NodeConfig{
	BootstrapNodes:        FoundationBootnodes(),
	MaxPeers:              25,
	NoDiscovery:           true,
	EthereumEnabled:       true,
	EthereumNetworkID:     1,
	EthereumDatabaseCache: 16,
}

// NewNodeConfig creates a new node option set, initialized to the default values.
func NewNodeConfig() *NodeConfig {
	config := *defaultNodeConfig
	return &config
}

// AddBootstrapNode adds an additional bootstrap node to the node config.
func (conf *NodeConfig) AddBootstrapNode(node *Enode) {
	conf.BootstrapNodes.Append(node)
}

// EncodeJSON encodes a NodeConfig into a JSON data dump.
func (conf *NodeConfig) EncodeJSON() (string, error) {
	data, err := json.Marshal(conf)
	return string(data), err
}

// String returns a printable representation of the node config.
func (conf *NodeConfig) String() string {
	return encodeOrError(conf)
}

// Node represents a Geth Ethereum node instance.
type Node struct {
	node *node.Node
	les  *les.LightEthereum
}

// NewNode creates and configures a new Geth node.
func NewNode(datadir string, config *NodeConfig) (stack *Node, _ error) {
	// If no or partial configurations were specified, use defaults
	if config == nil {
		config = NewNodeConfig()
	}
	if config.BootstrapNodes == nil || config.BootstrapNodes.Size() == 0 {
		config.BootstrapNodes = defaultNodeConfig.BootstrapNodes
	}

	// gomobile doesn't allow arrays to be passed in, so comma separated strings are used
	var httpVirtualHosts []string
	if config.HTTPVirtualHosts != "" {
		httpVirtualHosts = strings.Split(config.HTTPVirtualHosts, ",")
	}
	var httpModules []string
	if config.HTTPModules != "" {
		httpModules = strings.Split(config.HTTPModules, ",")
	}

	if config.PprofAddress != "" {
		debug.StartPProf(config.PprofAddress, true)
	}

	// Create the empty networking stack
	nodeConf := &node.Config{
		Name:              clientIdentifier,
		Version:           params.VersionWithMeta,
		DataDir:           datadir,
		KeyStoreDir:       filepath.Join(datadir, "keystore"), // Mobile should never use internal keystores!
		UseLightweightKDF: config.UseLightweightKDF,
		IPCPath:           config.IPCPath,
		HTTPHost:          config.HTTPHost,
		HTTPPort:          config.HTTPPort,
		HTTPVirtualHosts:  httpVirtualHosts,
		HTTPModules:       httpModules,
		P2P: p2p.Config{
			NoDiscovery:      config.NoDiscovery,
			DiscoveryV5:      !config.NoDiscovery,
			BootstrapNodesV5: config.BootstrapNodes.nodes,
			ListenAddr:       ":0",
			NAT:              nat.Any(),
			MaxPeers:         config.MaxPeers,
		},
		NoUSB: true,
	}

	rawStack, err := node.New(nodeConf)
	if err != nil {
		return nil, err
	}

	debug.Memsize.Add("node", rawStack)

	var genesis *core.Genesis
	var nodeResponse = Node{}
	nodeResponse.node = rawStack
	if config.EthereumGenesis != "" {
		// Parse the user supplied genesis spec if not mainnet
		genesis = new(core.Genesis)
		if err := json.Unmarshal([]byte(config.EthereumGenesis), genesis); err != nil {
			return nil, fmt.Errorf("invalid genesis spec: %v", err)
		}
		// If we have the testnet, hard code the chain configs too
		if config.EthereumGenesis == AlfajoresGenesis() {
			genesis.Config = params.AlfajoresChainConfig
			if config.EthereumNetworkID == 1 {
				config.EthereumNetworkID = int64(params.AlfajoresNetworkId)
			}
		} else if config.EthereumGenesis == BaklavaGenesis() {
			genesis.Config = params.BaklavaChainConfig
			if config.EthereumNetworkID == 1 {
				config.EthereumNetworkID = int64(params.BaklavaNetworkId)
			}
		}
	}
	// Register the Ethereum protocol if requested
	if config.EthereumEnabled {
		ethConf := eth.DefaultConfig
		ethConf.Genesis = genesis

		ethConf.SyncMode = getSyncMode(config.SyncMode)
		ethConf.NetworkId = uint64(config.EthereumNetworkID)
		ethConf.DatabaseCache = config.EthereumDatabaseCache
		// Use an in memory DB for replica state
		ethConf.Istanbul.ReplicaStateDBPath = ""
		// Use an in memory DB for validatorEnode table
		ethConf.Istanbul.ValidatorEnodeDBPath = ""
		ethConf.Istanbul.VersionCertificateDBPath = ""
		// Use an in memory DB for roundState table
		ethConf.Istanbul.RoundStateDBPath = ""
		lesBackend, err := les.New(rawStack, &ethConf)
		if err != nil {
			return nil, fmt.Errorf("ethereum init: %v", err)
		}
		nodeResponse.les = lesBackend
		// If netstats reporting is requested, do it
		if config.EthereumNetStats != "" {
			if err := ethstats.New(rawStack, lesBackend.ApiBackend, lesBackend.Engine(), config.EthereumNetStats); err != nil {
				return nil, fmt.Errorf("netstats init: %v", err)
			}
		}
	}
	return &nodeResponse, nil
}

func getSyncMode(syncMode int) downloader.SyncMode {
	switch syncMode {
	case SyncModeFullSync:
		return downloader.FullSync
	case SyncModeFastSync:
		return downloader.FastSync
	case SyncModeUnset:
		fallthrough
		// If unset, default to light sync.
		// This maintains backward compatibility.
	case SyncModeLightSync:
		return downloader.LightSync
	case SyncModeDeprecatedSync:
		panic("'celolatest' mode is no longer supported. Use 'lightest' instead")
	case LightestSync:
		return downloader.LightestSync
	default:
		panic(fmt.Sprintf("Unexpected sync mode value: %d", syncMode))
	}
}

// Close terminates a running node along with all it's services, tearing internal state
// down. It is not possible to restart a closed node.
func (n *Node) Close() error {
	return n.node.Close()
}

// Start creates a live P2P node and starts running it.
func (n *Node) Start() error {
	// TODO: recreate the node so it can be started multiple times
	return n.node.Start()
}

// Stop terminates a running node along with all its services. If the node was not started,
// an error is returned. It is not possible to restart a stopped node.
//
// Deprecated: use Close()
func (n *Node) Stop() error {
	return n.node.Close()
}

// GetEthereumClient retrieves a client to access the Ethereum subsystem.
func (n *Node) GetEthereumClient() (client *EthereumClient, _ error) {
	rpc, err := n.node.Attach()
	if err != nil {
		return nil, err
	}
	return &EthereumClient{ethclient.NewClient(rpc)}, nil
}

// GetNodeInfo gathers and returns a collection of metadata known about the host.
func (n *Node) GetNodeInfo() *NodeInfo {
	return &NodeInfo{n.node.Server().NodeInfo()}
}

// GetPeerInfos returns an array of metadata objects describing connected peers.
func (n *Node) GetPeerInfos() *PeerInfos {
	return &PeerInfos{n.node.Server().PeersInfo()}
}

func (n *Node) GetGethStats() (*Stats, error) {
	stats := NewStats()

	stats.SyncToRegistryStats()
	if err := n.GetBlockchainStats(stats); err != nil {
		return nil, err
	}

	return stats, nil
}

func (n *Node) GetBlockchainStats(stats *Stats) error {
	if n.les == nil {
		return fmt.Errorf("les was not initialised")
	}

	header := n.les.BlockChain().CurrentHeader()
	downloaderSync := n.les.Downloader().Progress()
	lastBlockNumber := n.les.BlockChain().CurrentHeader().Number.Uint64()
	stats.SetBool("chain/syncing", lastBlockNumber >= downloaderSync.HighestBlock)
	stats.SetUInt("chain/downloader/highestBlockNumber", downloaderSync.HighestBlock)
	stats.SetInt("chain/lastBlockTotalDifficulty", n.les.BlockChain().GetTd(header.Hash(), header.Number.Uint64()).Int64())
	stats.SetUInt("chain/lastBlockTimestamp", header.Time)

	return nil
}
