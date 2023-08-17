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
	"fmt"
	"math/big"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	istanbulBackend "github.com/celo-org/celo-blockchain/consensus/istanbul/backend"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/miner"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/params"
)

// Defaults contains default settings for use on the Ethereum main net.
var Defaults = Config{
	SyncMode:                downloader.FastSync,
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
	GatewayFee:              big.NewInt(0),

	TxPool:                core.DefaultTxPoolConfig,
	RPCGasInflationRate:   1.3,
	RPCGasPriceMultiplier: big.NewInt(200),
	RPCGasCap:             25000000,
	RPCTxFeeCap:           500, // 500 celo

	Istanbul: *istanbul.DefaultConfig,
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
	LightServ          int  `toml:",omitempty"` // Maximum percentage of time allowed for serving LES requests
	LightIngress       int  `toml:",omitempty"` // Incoming bandwidth limit for light servers
	LightEgress        int  `toml:",omitempty"` // Outgoing bandwidth limit for light servers
	LightPeers         int  `toml:",omitempty"` // Maximum number of LES client peers
	LightNoPrune       bool `toml:",omitempty"` // Whether to disable light chain pruning
	LightNoSyncServe   bool `toml:",omitempty"` // Whether to serve light clients before syncing
	SyncFromCheckpoint bool `toml:",omitempty"` // Whether to sync the header chain from the configured checkpoint
	// Minimum gateway fee value to serve a transaction from a light client
	GatewayFee *big.Int `toml:",omitempty"`
	// Validator is the address used to sign consensus messages. Also the address for block transaction rewards.
	Validator common.Address `toml:",omitempty"`
	// TxFeeRecipient is the GatewayFeeRecipient light clients need to specify in order for their transactions to be accepted by this node.
	TxFeeRecipient common.Address `toml:",omitempty"`
	BLSbase        common.Address `toml:",omitempty"`

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

	// RPCGasInflationRate is a global multiplier applied to the gas estimations
	RPCGasInflationRate float64

	// RPCGasPriceMultiplier is a global multiplier applied to the gas price
	// It's a percent value, e.g. 120 means a multiplication factor of 1.2
	RPCGasPriceMultiplier *big.Int

	// RPCGasCap is the global gas cap for eth-call variants.
	RPCGasCap uint64

	// RPCTxFeeCap is the global transaction fee(price * gaslimit) cap for
	// send-transction variants. The unit is ether.
	RPCTxFeeCap float64

	// RPCEthCompatibility is used to determine whether the 'gaslimit' end
	// 'baseFeePerGas' fields should be added to blocks returned by the RPC
	// API. Where true indicates the fields should be added.
	RPCEthCompatibility bool

	// Checkpoint is a hardcoded checkpoint which can be nil.
	Checkpoint *params.TrustedCheckpoint `toml:",omitempty"`

	// CheckpointOracle is the configuration for checkpoint oracle.
	CheckpointOracle *params.CheckpointOracleConfig `toml:",omitempty"`

	// Gingerbread block override (TODO: remove after the fork)
	OverrideGingerbread *big.Int `toml:",omitempty"`

	// Gingerbread block override (TODO: remove after the fork)
	OverrideGingerbreadP2 *big.Int `toml:",omitempty"`

	// The minimum required peers in order for syncing to be initiated, if left
	// at 0 then the default will be used.
	MinSyncPeers int `toml:",omitempty"`
}

// CreateConsensusEngine creates the required type of consensus engine instance for an Ethereum service
func CreateConsensusEngine(stack *node.Node, chainConfig *params.ChainConfig, config *Config, db ethdb.Database) consensus.Engine {
	if chainConfig.Faker {
		return mockEngine.NewFaker()
	}
	// If Istanbul is requested, set it up
	if chainConfig.Istanbul != nil {
		log.Debug("Setting up Istanbul consensus engine")
		if err := istanbul.ApplyParamsChainConfigToConfig(chainConfig, &config.Istanbul); err != nil {
			log.Crit("Invalid Configuration for Istanbul Engine", "err", err)
		}
		return istanbulBackend.New(&config.Istanbul, db)
	}
	log.Error(fmt.Sprintf("Only Istanbul Consensus is supported: %v", chainConfig))
	return nil
}
