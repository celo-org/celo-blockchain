// Copyright 2017 The go-ethereum Authors
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

// Package ethconfig contains the configuration of the ETH and LES protocols.
package ethconfig

import (
	"math/big"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/miner"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/ethash"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/eth/gasprice"
	"github.com/celo-org/celo-blockchain/miner"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/clique"
	"github.com/celo-org/celo-blockchain/consensus/ethash"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/eth/gasprice"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/miner"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/params"
)

<<<<<<< HEAD:eth/config.go
// DefaultConfig contains default settings for use on the Ethereum main net.
var DefaultConfig = Config{
	SyncMode:                downloader.FastSync,
||||||| e78727290:eth/config.go
// DefaultFullGPOConfig contains default gasprice oracle settings for full node.
var DefaultFullGPOConfig = gasprice.Config{
	Blocks:     20,
	Percentile: 60,
	MaxPrice:   gasprice.DefaultMaxPrice,
}

// DefaultLightGPOConfig contains default gasprice oracle settings for light client.
var DefaultLightGPOConfig = gasprice.Config{
	Blocks:     2,
	Percentile: 60,
	MaxPrice:   gasprice.DefaultMaxPrice,
}

// DefaultConfig contains default settings for use on the Ethereum main net.
var DefaultConfig = Config{
	SyncMode: downloader.FastSync,
	Ethash: ethash.Config{
		CacheDir:         "ethash",
		CachesInMem:      2,
		CachesOnDisk:     3,
		CachesLockMmap:   false,
		DatasetsInMem:    1,
		DatasetsOnDisk:   2,
		DatasetsLockMmap: false,
	},
=======
// FullNodeGPO contains default gasprice oracle settings for full node.
var FullNodeGPO = gasprice.Config{
	Blocks:           20,
	Percentile:       60,
	MaxHeaderHistory: 0,
	MaxBlockHistory:  0,
	MaxPrice:         gasprice.DefaultMaxPrice,
	IgnorePrice:      gasprice.DefaultIgnorePrice,
}

// LightClientGPO contains default gasprice oracle settings for light client.
var LightClientGPO = gasprice.Config{
	Blocks:           2,
	Percentile:       60,
	MaxHeaderHistory: 300,
	MaxBlockHistory:  5,
	MaxPrice:         gasprice.DefaultMaxPrice,
	IgnorePrice:      gasprice.DefaultIgnorePrice,
}

// Defaults contains default settings for use on the Ethereum main net.
var Defaults = Config{
	SyncMode: downloader.SnapSync,
	Ethash: ethash.Config{
		CacheDir:         "ethash",
		CachesInMem:      2,
		CachesOnDisk:     3,
		CachesLockMmap:   false,
		DatasetsInMem:    1,
		DatasetsOnDisk:   2,
		DatasetsLockMmap: false,
	},
>>>>>>> v1.10.7:eth/ethconfig/config.go
	NetworkId:               1,
	TxLookupLimit:           2350000,
	LightPeers:              100,
	LightServ:               0,
	UltraLightFraction:      75,
	DatabaseCache:           512,
	TrieCleanCache:          154,
	TrieCleanCacheJournal:   "triecache",
	TrieCleanCacheRejournal: 60 * time.Minute,
	TrieDirtyCache:          256,
	TrieTimeout:             60 * time.Minute,
	SnapshotCache:           102,
<<<<<<< HEAD:eth/config.go
	GatewayFee:              big.NewInt(0),

||||||| e78727290:eth/config.go
	Miner: miner.Config{
		GasFloor: 8000000,
		GasCeil:  8000000,
		GasPrice: big.NewInt(params.GWei),
		Recommit: 3 * time.Second,
	},
=======
	Miner: miner.Config{
		GasCeil:  8000000,
		GasPrice: big.NewInt(params.GWei),
		Recommit: 3 * time.Second,
	},
>>>>>>> v1.10.7:eth/ethconfig/config.go
	TxPool:      core.DefaultTxPoolConfig,
<<<<<<< HEAD:eth/config.go
	RPCGasCap:   25000000,
	RPCTxFeeCap: 500, // 500 celo
||||||| e78727290:eth/config.go
	RPCGasCap:   25000000,
	GPO:         DefaultFullGPOConfig,
	RPCTxFeeCap: 1, // 1 ether
}
=======
	RPCGasCap:   50000000,
	GPO:         FullNodeGPO,
	RPCTxFeeCap: 1, // 1 ether
}
>>>>>>> v1.10.7:eth/ethconfig/config.go

<<<<<<< HEAD:eth/config.go
	Istanbul: *istanbul.DefaultConfig,
||||||| e78727290:eth/config.go
func init() {
	home := os.Getenv("HOME")
	if home == "" {
		if user, err := user.Current(); err == nil {
			home = user.HomeDir
		}
	}
	if runtime.GOOS == "darwin" {
		DefaultConfig.Ethash.DatasetDir = filepath.Join(home, "Library", "Ethash")
	} else if runtime.GOOS == "windows" {
		localappdata := os.Getenv("LOCALAPPDATA")
		if localappdata != "" {
			DefaultConfig.Ethash.DatasetDir = filepath.Join(localappdata, "Ethash")
		} else {
			DefaultConfig.Ethash.DatasetDir = filepath.Join(home, "AppData", "Local", "Ethash")
		}
	} else {
		DefaultConfig.Ethash.DatasetDir = filepath.Join(home, ".ethash")
	}
=======
func init() {
	home := os.Getenv("HOME")
	if home == "" {
		if user, err := user.Current(); err == nil {
			home = user.HomeDir
		}
	}
	if runtime.GOOS == "darwin" {
		Defaults.Ethash.DatasetDir = filepath.Join(home, "Library", "Ethash")
	} else if runtime.GOOS == "windows" {
		localappdata := os.Getenv("LOCALAPPDATA")
		if localappdata != "" {
			Defaults.Ethash.DatasetDir = filepath.Join(localappdata, "Ethash")
		} else {
			Defaults.Ethash.DatasetDir = filepath.Join(home, "AppData", "Local", "Ethash")
		}
	} else {
		Defaults.Ethash.DatasetDir = filepath.Join(home, ".ethash")
	}
>>>>>>> v1.10.7:eth/ethconfig/config.go
}

//go:generate gencodec -type Config -formats toml -out gen_config.go

// Config contains configuration options for of the ETH and LES protocols.
type Config struct {
	// The genesis block, which is inserted if the database is empty.
	// If nil, the Ethereum main net block is used.
	Genesis *core.Genesis `toml:",omitempty"`

	// Protocol options
	NetworkId uint64 // Network ID to use for selecting peers to connect to
	SyncMode  downloader.SyncMode

	// This can be set to list of enrtree:// URLs which will be queried for
	// for nodes to connect to.
	EthDiscoveryURLs  []string
	SnapDiscoveryURLs []string

	NoPruning  bool // Whether to disable pruning and flush everything to disk
	NoPrefetch bool // Whether to disable prefetching and only load state on demand

	TxLookupLimit uint64 `toml:",omitempty"` // The maximum number of blocks from head whose tx indices are reserved.

	// Whitelist of required block number -> hash values to accept
	Whitelist map[uint64]common.Hash `toml:"-"`

	// Light client options
<<<<<<< HEAD:eth/config.go
	LightServ    int  `toml:",omitempty"` // Maximum percentage of time allowed for serving LES requests
	LightIngress int  `toml:",omitempty"` // Incoming bandwidth limit for light servers
	LightEgress  int  `toml:",omitempty"` // Outgoing bandwidth limit for light servers
	LightPeers   int  `toml:",omitempty"` // Maximum number of LES client peers
	LightNoPrune bool `toml:",omitempty"` // Whether to disable light chain pruning
	// Minimum gateway fee value to serve a transaction from a light client
	GatewayFee *big.Int `toml:",omitempty"`
	// Validator is the address used to sign consensus messages. Also the address for block transaction rewards.
	Validator common.Address `toml:",omitempty"`
	// TxFeeRecipient is the GatewayFeeRecipient light clients need to specify in order for their transactions to be accepted by this node.
	TxFeeRecipient common.Address `toml:",omitempty"`
	BLSbase        common.Address `toml:",omitempty"`
||||||| e78727290:eth/config.go
	LightServ    int  `toml:",omitempty"` // Maximum percentage of time allowed for serving LES requests
	LightIngress int  `toml:",omitempty"` // Incoming bandwidth limit for light servers
	LightEgress  int  `toml:",omitempty"` // Outgoing bandwidth limit for light servers
	LightPeers   int  `toml:",omitempty"` // Maximum number of LES client peers
	LightNoPrune bool `toml:",omitempty"` // Whether to disable light chain pruning
=======
	LightServ          int  `toml:",omitempty"` // Maximum percentage of time allowed for serving LES requests
	LightIngress       int  `toml:",omitempty"` // Incoming bandwidth limit for light servers
	LightEgress        int  `toml:",omitempty"` // Outgoing bandwidth limit for light servers
	LightPeers         int  `toml:",omitempty"` // Maximum number of LES client peers
	LightNoPrune       bool `toml:",omitempty"` // Whether to disable light chain pruning
	LightNoSyncServe   bool `toml:",omitempty"` // Whether to serve light clients before syncing
	SyncFromCheckpoint bool `toml:",omitempty"` // Whether to sync the header chain from the configured checkpoint
>>>>>>> v1.10.7:eth/ethconfig/config.go

	// Ultra Light client options
	UltraLightServers      []string `toml:",omitempty"` // List of trusted ultra light servers
	UltraLightFraction     int      `toml:",omitempty"` // Percentage of trusted servers to accept an announcement
	UltraLightOnlyAnnounce bool     `toml:",omitempty"` // Whether to only announce headers, or also serve them

	// Database options
	SkipBcVersionCheck bool `toml:"-"`
	DatabaseHandles    int  `toml:"-"`
	DatabaseCache      int
	DatabaseFreezer    string

	TrieCleanCache          int
	TrieCleanCacheJournal   string        `toml:",omitempty"` // Disk journal directory for trie cache to survive node restarts
	TrieCleanCacheRejournal time.Duration `toml:",omitempty"` // Time interval to regenerate the journal for clean cache
	TrieDirtyCache          int
	TrieTimeout             time.Duration
	SnapshotCache           int
	Preimages               bool

	// Mining options
	Miner miner.Config

	// Transaction pool options
	TxPool core.TxPoolConfig

	// Enables tracking of SHA3 preimages in the VM
	EnablePreimageRecording bool

	// Istanbul options
	Istanbul istanbul.Config

	// Miscellaneous options
	DocRoot string `toml:"-"`

	// RPCGasCap is the global gas cap for eth-call variants.
	RPCGasCap uint64

	// RPCTxFeeCap is the global transaction fee(price * gaslimit) cap for
	// send-transction variants. The unit is ether.
	RPCTxFeeCap float64

	// Checkpoint is a hardcoded checkpoint which can be nil.
	Checkpoint *params.TrustedCheckpoint `toml:",omitempty"`

	// CheckpointOracle is the configuration for checkpoint oracle.
	CheckpointOracle *params.CheckpointOracleConfig `toml:",omitempty"`
<<<<<<< HEAD:eth/config.go

	// E block override (TODO: remove after the fork)
	OverrideEHardfork *big.Int `toml:",omitempty"`
||||||| e78727290:eth/config.go
=======

	// Berlin block override (TODO: remove after the fork)
	OverrideLondon *big.Int `toml:",omitempty"`
}

// CreateConsensusEngine creates a consensus engine for the given chain configuration.
func CreateConsensusEngine(stack *node.Node, chainConfig *params.ChainConfig, config *ethash.Config, notify []string, noverify bool, db ethdb.Database) consensus.Engine {
	// If proof-of-authority is requested, set it up
	if chainConfig.Clique != nil {
		return clique.New(chainConfig.Clique, db)
	}
	// Otherwise assume proof-of-work
	switch config.PowMode {
	case ethash.ModeFake:
		log.Warn("Ethash used in fake mode")
	case ethash.ModeTest:
		log.Warn("Ethash used in test mode")
	case ethash.ModeShared:
		log.Warn("Ethash used in shared mode")
	}
	engine := ethash.New(ethash.Config{
		PowMode:          config.PowMode,
		CacheDir:         stack.ResolvePath(config.CacheDir),
		CachesInMem:      config.CachesInMem,
		CachesOnDisk:     config.CachesOnDisk,
		CachesLockMmap:   config.CachesLockMmap,
		DatasetDir:       config.DatasetDir,
		DatasetsInMem:    config.DatasetsInMem,
		DatasetsOnDisk:   config.DatasetsOnDisk,
		DatasetsLockMmap: config.DatasetsLockMmap,
		NotifyFull:       config.NotifyFull,
	}, notify, noverify)
	engine.SetThreads(-1) // Disable CPU mining
	return engine
>>>>>>> v1.10.7:eth/ethconfig/config.go
}
