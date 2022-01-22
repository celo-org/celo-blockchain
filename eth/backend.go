// Copyright 2014 The go-ethereum Authors
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

// Package eth implements the Ethereum protocol.
package eth

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/accounts"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	istanbulBackend "github.com/celo-org/celo-blockchain/consensus/istanbul/backend"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/bloombits"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/state/pruner"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/eth/ethconfig"
	"github.com/celo-org/celo-blockchain/eth/filters"
	"github.com/celo-org/celo-blockchain/eth/protocols/eth"

	// "github.com/celo-org/celo-blockchain/eth/protocols/snap"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/event"
	"github.com/celo-org/celo-blockchain/internal/debug"
	"github.com/celo-org/celo-blockchain/internal/ethapi"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/miner"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/dnsdisc"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-blockchain/rpc"
)

// Config contains the configuration options of the ETH protocol.
// Deprecated: use ethconfig.Config instead.
type Config = ethconfig.Config

// Ethereum implements the Ethereum full node service.
type Ethereum struct {
	config *ethconfig.Config

	// Handlers
	txPool             *core.TxPool
	blockchain         *core.BlockChain
	handler            *handler
	ethDialCandidates  enode.Iterator
	snapDialCandidates enode.Iterator

	// DB interfaces
	chainDb ethdb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests     chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer      *core.ChainIndexer             // Bloom indexer operating during block imports
	closeBloomHandler chan struct{}

	APIBackend *EthAPIBackend

	miner          *miner.Miner
	gatewayFee     *big.Int
	validator      common.Address
	txFeeRecipient common.Address
	blsbase        common.Address

	networkID     uint64
	netRPCService *ethapi.PublicNetAPI

	p2pServer *p2p.Server

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price, validator and txFeeRecipient)
}

// New creates a new Ethereum object (including the
// initialisation of the common Ethereum object)
func New(stack *node.Node, config *ethconfig.Config) (*Ethereum, error) {
	// Ensure configuration values are compatible and sane
	if !config.SyncMode.SyncFullBlockChain() {
		return nil, errors.New("can't run eth.Ethereum in light sync mode or lightest sync mode, use les.LightEthereum")
	}
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	if config.NoPruning && config.TrieDirtyCache > 0 {
		if config.SnapshotCache > 0 {
			config.TrieCleanCache += config.TrieDirtyCache * 3 / 5
			config.SnapshotCache += config.TrieDirtyCache * 2 / 5
		} else {
			config.TrieCleanCache += config.TrieDirtyCache
		}
		config.TrieDirtyCache = 0
	}
	log.Info("Allocated trie memory caches", "clean", common.StorageSize(config.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(config.TrieDirtyCache)*1024*1024)

	if config.GatewayFee == nil || config.GatewayFee.Cmp(common.Big0) < 0 {
		log.Warn("Sanitizing invalid gateway fee", "provided", config.GatewayFee, "updated", ethconfig.Defaults.GatewayFee)
		config.GatewayFee = new(big.Int).Set(ethconfig.Defaults.GatewayFee)
	}
	// Assemble the Ethereum object
	chainDb, err := stack.OpenDatabaseWithFreezer("chaindata", config.DatabaseCache, config.DatabaseHandles, config.DatabaseFreezer, "eth/db/chaindata/", false)
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlockWithOverride(chainDb, config.Genesis, config.OverrideEHardfork)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)
	chainConfig.FullHeaderChainAvailable = config.SyncMode.SyncFullHeaderChain()

	if err := pruner.RecoverPruning(stack.ResolvePath(""), chainDb, stack.ResolvePath(config.TrieCleanCacheJournal)); err != nil {
		log.Error("Failed to recover state", "error", err)
	}
	eth := &Ethereum{
		config:            config,
		chainDb:           chainDb,
		eventMux:          stack.EventMux(),
		accountManager:    stack.AccountManager(),
		engine:            ethconfig.CreateConsensusEngine(stack, chainConfig, config, chainDb),
		closeBloomHandler: make(chan struct{}),
		networkID:         config.NetworkId,
		validator:         config.Miner.Validator,
		txFeeRecipient:    config.TxFeeRecipient,
		gatewayFee:        config.GatewayFee,
		blsbase:           config.BLSbase,
		bloomRequests:     make(chan chan *bloombits.Retrieval),
		bloomIndexer:      core.NewBloomIndexer(chainDb, params.BloomBitsBlocks, params.BloomConfirms, chainConfig.FullHeaderChainAvailable),
		p2pServer:         stack.Server(),
	}

	bcVersion := rawdb.ReadDatabaseVersion(chainDb)
	var dbVer = "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	log.Info("Initialising Ethereum protocol", "versions", istanbul.ProtocolVersions, "network", config.NetworkId, "dbversion", dbVer)

	if !config.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > core.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, Geth %s only supports v%d", *bcVersion, params.VersionWithMeta, core.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < core.BlockChainVersion {
			if bcVersion != nil { // only print warning on upgrade, not on init
				log.Warn("Upgrade blockchain database version", "from", dbVer, "to", core.BlockChainVersion)
			}
			rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
		}
	}
	var (
		vmConfig = vm.Config{
			EnablePreimageRecording: config.EnablePreimageRecording,
		}
		cacheConfig = &core.CacheConfig{
			TrieCleanLimit:      config.TrieCleanCache,
			TrieCleanJournal:    stack.ResolvePath(config.TrieCleanCacheJournal),
			TrieCleanRejournal:  config.TrieCleanCacheRejournal,
			TrieCleanNoPrefetch: config.NoPrefetch,
			TrieDirtyLimit:      config.TrieDirtyCache,
			TrieDirtyDisabled:   config.NoPruning,
			TrieTimeLimit:       config.TrieTimeout,
			SnapshotLimit:       config.SnapshotCache,
			Preimages:           config.Preimages,
		}
	)
	eth.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, chainConfig, eth.engine, vmConfig, eth.shouldPreserve, &config.TxLookupLimit)
	if err != nil {
		return nil, err
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		eth.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	eth.bloomIndexer.Start(eth.blockchain)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = stack.ResolvePath(config.TxPool.Journal)
	}

	eth.txPool = core.NewTxPool(config.TxPool, chainConfig, eth.blockchain)

	// Permit the downloader to use the trie cache allowance during fast sync
	cacheLimit := cacheConfig.TrieCleanLimit + cacheConfig.TrieDirtyLimit + cacheConfig.SnapshotLimit
	checkpoint := config.Checkpoint
	if checkpoint == nil {
		checkpoint = params.TrustedCheckpoints[genesisHash]
	}
	if eth.handler, err = newHandler(&handlerConfig{
		Database:    chainDb,
		Chain:       eth.blockchain,
		TxPool:      eth.txPool,
		Network:     config.NetworkId,
		Sync:        config.SyncMode,
		BloomCache:  uint64(cacheLimit),
		EventMux:    eth.eventMux,
		Checkpoint:  checkpoint,
		Whitelist:   config.Whitelist,
		server:      stack.Server(),
		proxyServer: stack.ProxyServer(),
	}); err != nil {
		return nil, err
	}

	// If the engine is istanbul, then inject the blockchain
	if istanbul, isIstanbul := eth.engine.(*istanbulBackend.Backend); isIstanbul {
		istanbul.SetChain(
			eth.blockchain, eth.blockchain.CurrentBlock,
			func(hash common.Hash) (*state.StateDB, error) {
				stateRoot := eth.blockchain.GetHeaderByHash(hash).Root
				return eth.blockchain.StateAt(stateRoot)
			})
	}

	eth.miner = miner.New(eth, &config.Miner, chainConfig, eth.EventMux(), eth.engine, chainDb)
	eth.miner.SetExtra(makeExtraData(config.Miner.ExtraData))

	eth.APIBackend = &EthAPIBackend{stack.Config().ExtRPCEnabled(), true, eth}

	// Setup DNS discovery iterators.
	dnsclient := dnsdisc.NewClient(dnsdisc.Config{})
	eth.ethDialCandidates, err = dnsclient.NewIterator(eth.config.EthDiscoveryURLs...)
	if err != nil {
		return nil, err
	}
	eth.snapDialCandidates, err = dnsclient.NewIterator(eth.config.SnapDiscoveryURLs...)
	if err != nil {
		return nil, err
	}

	// Start the RPC service
	eth.netRPCService = ethapi.NewPublicNetAPI(eth.p2pServer, config.NetworkId)

	// Register the backend on the node
	stack.RegisterAPIs(eth.APIs())
	stack.RegisterProtocols(eth.Protocols())
	stack.RegisterLifecycle(eth)
	// Check for unclean shutdown
	if uncleanShutdowns, discards, err := rawdb.PushUncleanShutdownMarker(chainDb); err != nil {
		log.Error("Could not update unclean-shutdown-marker list", "error", err)
	} else {
		if discards > 0 {
			log.Warn("Old unclean shutdowns found", "count", discards)
		}
		for _, tstamp := range uncleanShutdowns {
			t := time.Unix(int64(tstamp), 0)
			log.Warn("Unclean shutdown detected", "booted", t,
				"age", common.PrettyAge(t))
		}
	}
	debug.Memsize.Add("chainDb", eth.handler.database)
	debug.Memsize.Add("blockchain", eth.blockchain)
	debug.Memsize.Add("server(p2p)", eth.handler.server)
	debug.Memsize.Add("miner", eth.miner)
	return eth, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"geth",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// APIs return the collection of RPC services the ethereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Ethereum) APIs() []rpc.API {
	apis := ethapi.GetAPIs(s.APIBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicEthereumAPI(s),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.handler.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.APIBackend, false, 5*time.Minute),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *Ethereum) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *Ethereum) Validator() (val common.Address, err error) {
	s.lock.RLock()
	validator := s.validator
	s.lock.RUnlock()

	if validator != (common.Address{}) {
		return validator, nil
	}
	return common.Address{}, fmt.Errorf("validator must be explicitly specified")
}

func (s *Ethereum) TxFeeRecipient() (common.Address, error) {
	s.lock.RLock()
	txFeeRecipient := s.txFeeRecipient
	s.lock.RUnlock()

	if txFeeRecipient != (common.Address{}) {
		return txFeeRecipient, nil
	}
	return common.Address{}, fmt.Errorf("txFeeRecipient must be explicitly specified")
}

func (s *Ethereum) BLSbase() (eb common.Address, err error) {
	s.lock.RLock()
	blsbase := s.blsbase
	s.lock.RUnlock()

	if blsbase != (common.Address{}) {
		return blsbase, nil
	}

	return s.Validator()
}

// isLocalBlock checks whether the specified block is mined
// by local miner accounts.
//
// We regard two types of accounts as local miner account: the validator
// address and accounts specified via `txpool.locals` flag.
func (s *Ethereum) isLocalBlock(block *types.Block) bool {
	author, err := s.engine.Author(block.Header())
	if err != nil {
		log.Warn("Failed to retrieve block author", "number", block.NumberU64(), "hash", block.Hash(), "err", err)
		return false
	}
	// Check whether the given address is configured validator.
	s.lock.RLock()
	validator := s.validator
	s.lock.RUnlock()
	if author == validator {
		return true
	}
	// Check whether the given address is specified by `txpool.local`
	// CLI flag.
	for _, account := range s.config.TxPool.Locals {
		if account == author {
			return true
		}
	}
	return false
}

// shouldPreserve checks whether we should preserve the given block
// during the chain reorg depending on whether the author of block
// is a local account.
func (s *Ethereum) shouldPreserve(block *types.Block) bool {
	return s.isLocalBlock(block)
}

// SetValidator sets the address to sign consensus messages.
func (s *Ethereum) SetValidator(validator common.Address) {
	s.lock.Lock()
	s.validator = validator
	s.lock.Unlock()

	s.miner.SetValidator(validator)
}

// SetTxFeeRecipient sets the mining reward address.
func (s *Ethereum) SetTxFeeRecipient(txFeeRecipient common.Address) {
	s.lock.Lock()
	s.txFeeRecipient = txFeeRecipient
	s.lock.Unlock()

	s.miner.SetTxFeeRecipient(txFeeRecipient)
}

// StartMining starts the miner
func (s *Ethereum) StartMining() error {
	// If the miner was not running, initialize it
	if !s.IsMining() {

		// Configure the local mining address
		validator, err := s.Validator()
		if err != nil {
			log.Error("Cannot start mining without validator", "err", err)
			return fmt.Errorf("validator missing: %v", err)
		}

		txFeeRecipient, err := s.TxFeeRecipient()
		if err != nil {
			log.Error("Cannot start mining without txFeeRecipient", "err", err)
			return fmt.Errorf("txFeeRecipient missing: %v", err)
		}

		blsbase, err := s.BLSbase()
		if err != nil {
			log.Error("Cannot start mining without blsbase", "err", err)
			return fmt.Errorf("blsbase missing: %v", err)
		}

		if istanbul, isIstanbul := s.engine.(*istanbulBackend.Backend); isIstanbul {
			valAccount := accounts.Account{Address: validator}
			wallet, err := s.accountManager.Find(valAccount)
			if wallet == nil || err != nil {
				log.Error("Validator account unavailable locally", "err", err)
				return fmt.Errorf("signer missing: %v", err)
			}
			publicKey, err := wallet.GetPublicKey(valAccount)
			if err != nil {
				return fmt.Errorf("ECDSA public key missing: %v", err)
			}
			blswallet, err := s.accountManager.Find(accounts.Account{Address: blsbase})
			if blswallet == nil || err != nil {
				log.Error("BLSbase account unavailable locally", "err", err)
				return fmt.Errorf("BLS signer missing: %v", err)
			}

			istanbul.Authorize(validator, blsbase, publicKey, wallet.Decrypt, wallet.SignData, blswallet.SignBLS, wallet.SignHash)

			if istanbul.IsProxiedValidator() {
				if err := istanbul.StartProxiedValidatorEngine(); err != nil {
					log.Error("Error in starting proxied validator engine", "err", err)
					return err
				}
			}
		}

		// If mining is started, we can disable the transaction rejection mechanism
		// introduced to speed sync times.
		atomic.StoreUint32(&s.handler.acceptTxs, 1)

		go s.miner.Start(validator, txFeeRecipient)
	}
	return nil
}

// StopMining terminates the miner, both at the consensus engine level as well as
// at the block creation level.
func (s *Ethereum) StopMining() {
	// Stop the block creating itself
	s.miner.Stop()

	// Stop the proxied validator engine
	if istanbul, isIstanbul := s.engine.(*istanbulBackend.Backend); isIstanbul {
		if istanbul.IsProxiedValidator() {
			if err := istanbul.StopProxiedValidatorEngine(); err != nil {
				log.Warn("Error in stopping proxied validator engine", "err", err)
			}
		}
	}
}

func (s *Ethereum) startAnnounce() error {
	if istanbul, ok := s.engine.(consensus.Istanbul); ok {
		return istanbul.StartAnnouncing()
	}

	return nil
}

func (s *Ethereum) stopAnnounce() error {
	if istanbul, ok := s.engine.(consensus.Istanbul); ok {
		return istanbul.StopAnnouncing()
	}

	return nil
}

func (s *Ethereum) IsMining() bool      { return s.miner.Mining() }
func (s *Ethereum) Miner() *miner.Miner { return s.miner }

func (s *Ethereum) AccountManager() *accounts.Manager   { return s.accountManager }
func (s *Ethereum) BlockChain() *core.BlockChain        { return s.blockchain }
func (s *Ethereum) Config() *Config                     { return s.config }
func (s *Ethereum) TxPool() *core.TxPool                { return s.txPool }
func (s *Ethereum) EventMux() *event.TypeMux            { return s.eventMux }
func (s *Ethereum) Engine() consensus.Engine            { return s.engine }
func (s *Ethereum) ChainDb() ethdb.Database             { return s.chainDb }
func (s *Ethereum) IsListening() bool                   { return true } // Always listening
func (s *Ethereum) EthVersion() int                     { return int(istanbul.ProtocolVersions[0]) }
func (s *Ethereum) NetVersion() uint64                  { return s.networkID }
func (s *Ethereum) Downloader() *downloader.Downloader  { return s.handler.downloader }
func (s *Ethereum) GatewayFeeRecipient() common.Address { return common.Address{} } // Full-nodes do not make use of gateway fee.
func (s *Ethereum) GatewayFee() *big.Int                { return common.Big0 }
func (s *Ethereum) Synced() bool                        { return atomic.LoadUint32(&s.handler.acceptTxs) == 1 }
func (s *Ethereum) ArchiveMode() bool                   { return s.config.NoPruning }
func (s *Ethereum) BloomIndexer() *core.ChainIndexer    { return s.bloomIndexer }

// Protocols returns all the currently configured
// network protocols to start.
func (s *Ethereum) Protocols() []p2p.Protocol {
	protos := eth.MakeProtocols((*ethHandler)(s.handler), s.networkID, s.ethDialCandidates)
	if s.config.SnapshotCache > 0 {
		// protos = append(protos, snap.MakeProtocols((*snapHandler)(s.handler), s.snapDialCandidates)...)
	}
	return protos
}

// Start implements node.Lifecycle, starting all internal goroutines needed by the
// Ethereum protocol implementation.
func (s *Ethereum) Start() error {
	eth.StartENRUpdater(s.blockchain, s.p2pServer.LocalNode())

	// Start the bloom bits servicing goroutines
	s.startBloomHandlers(params.BloomBitsBlocks)

	// Figure out a max peers count based on the server limits
	maxPeers := s.p2pServer.MaxPeers
	if s.config.LightServ > 0 {
		if s.config.LightPeers != 0 && s.config.LightPeers >= s.p2pServer.MaxPeers {
			return fmt.Errorf("invalid peer config: light peer count (%d) >= total peer count (%d)", s.config.LightPeers, s.p2pServer.MaxPeers)
		}
		maxPeers -= s.config.LightPeers
	}
	// Start the networking layer and the light server if requested
	s.handler.Start(maxPeers)

	if err := s.startAnnounce(); err != nil {
		return err
	}

	return nil
}

// Stop implements node.Lifecycle, terminating all internal goroutines used by the
// Ethereum protocol.
func (s *Ethereum) Stop() error {
	// Stop all the peer-related stuff first.
	s.stopAnnounce()
	s.ethDialCandidates.Close()
	s.snapDialCandidates.Close()
	s.handler.Stop()

	// Then stop everything else.
	s.bloomIndexer.Close()
	close(s.closeBloomHandler)
	s.txPool.Stop()
	s.miner.Stop()
	s.blockchain.Stop()
	s.engine.Close()
	rawdb.PopUncleanShutdownMarker(s.chainDb)
	s.chainDb.Close()
	s.eventMux.Stop()

	return nil
}
