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

// Package consensus implements different Ethereum consensus engines.
package consensus

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rpc"
)

// ChainReader defines a small collection of methods needed to access the local
// blockchain during header verification.
type ChainReader interface {
	// Config retrieves the blockchain's chain configuration.
	Config() *params.ChainConfig

	// CurrentHeader retrieves the current header from the local chain.
	CurrentHeader() *types.Header

	// GetHeader retrieves a block header from the database by hash and number.
	GetHeader(hash common.Hash, number uint64) *types.Header

	// GetHeaderByNumber retrieves a block header from the database by number.
	GetHeaderByNumber(number uint64) *types.Header

	// GetHeaderByHash retrieves a block header from the database by its hash.
	GetHeaderByHash(hash common.Hash) *types.Header

	// GetBlock retrieves a block from the database by hash and number.
	GetBlock(hash common.Hash, number uint64) *types.Block
}

// Engine is an algorithm agnostic consensus engine.
type Engine interface {
	// Author retrieves the Ethereum address of the account that minted the given
	// block, which may be different from the header's coinbase if a consensus
	// engine is based on signatures.
	Author(header *types.Header) (common.Address, error)

	// VerifyHeader checks whether a header conforms to the consensus rules of a
	// given engine. Verifying the seal may be done optionally here, or explicitly
	// via the VerifySeal method.
	VerifyHeader(chain ChainReader, header *types.Header, seal bool) error

	// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
	// concurrently. The method returns a quit channel to abort the operations and
	// a results channel to retrieve the async verifications (the order is that of
	// the input slice).
	VerifyHeaders(chain ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error)

	// VerifySeal checks whether the crypto seal on a header is valid according to
	// the consensus rules of the given engine.
	VerifySeal(chain ChainReader, header *types.Header) error

	// Prepare initializes the consensus fields of a block header according to the
	// rules of a particular engine. The changes are executed inline.
	Prepare(chain ChainReader, header *types.Header) error

	// Finalize runs any post-transaction state modifications (e.g. block rewards)
	// but does not assemble the block.
	//
	// Note: The block header and state database might be updated to reflect any
	// consensus rules that happen at finalization (e.g. block rewards).
	Finalize(chain ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction)

	// FinalizeAndAssemble runs any post-transaction state modifications (e.g. block
	// rewards) and assembles the final block.
	//
	// Note: The block header and state database might be updated to reflect any
	// consensus rules that happen at finalization (e.g. block rewards).
	FinalizeAndAssemble(chain ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction, receipts []*types.Receipt, randomness *types.Randomness) (*types.Block, error)

	// Seal generates a new sealing request for the given input block and pushes
	// the result into the given channel.
	//
	// Note, the method returns immediately and will send the result async. More
	// than one result may also be returned depending on the consensus algorithm.
	Seal(chain ChainReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error

	// SealHash returns the hash of a block prior to it being sealed.
	SealHash(header *types.Header) common.Hash

	// GetValidators returns the list of current validators.
	GetValidators(blockNumber *big.Int, headerHash common.Hash) []istanbul.Validator

	EpochSize() uint64

	// APIs returns the RPC APIs this consensus engine provides.
	APIs(chain ChainReader) []rpc.API

	// Close terminates any background threads maintained by the consensus engine.
	Close() error
}

type Genesis interface {
	GetAlloc() GenesisAlloc

	UnmarshalFromDB(db ethdb.Database) error
}

type GenesisAlloc map[common.Address]GenesisAccount

type GenesisAccount interface {
	GetPublicKey() []byte
}

// Handler should be implemented if the consensus needs to handle and send peer messages
type Handler interface {
	// NewWork handles a new work event from the miner
	NewWork() error

	// HandleMsg handles a message from peer
	HandleMsg(address common.Address, data p2p.Msg, peer Peer) (bool, error)

	// SetBroadcaster sets the broadcaster to send message to peers
	SetBroadcaster(Broadcaster)

	// SetP2PServer sets the p2p server to connect/disconnect to/from peers
	SetP2PServer(P2PServer)

	// RegisterPeer will notify the consensus engine that a new peer has been added
	RegisterPeer(peer Peer, fromProxiedNode bool) error

	// UnregisterPeer will notify the consensus engine that a new peer has been removed
	UnregisterPeer(peer Peer, fromProxiedNode bool)

	// Handshake will begin a handshake with a new peer. It returns if the peer
	// has identified itself as a validator and should bypass any max peer checks.
	Handshake(peer Peer) (bool, error)
}

// PoW is a consensus engine based on proof-of-work.
type PoW interface {
	Engine

	// Hashrate returns the current mining hashrate of a PoW consensus engine.
	Hashrate() float64
}

// Istanbul is a consensus engine to avoid byzantine failure
type Istanbul interface {
	Engine

	// IsProxiedValidator returns true if this node is a proxied validator
	IsProxiedValidator() bool

	// IsProxy returns true if this node is a proxy
	IsProxy() bool

	// IsPrimary returns true if this node is the primary validator
	IsPrimary() bool

	// IsPrimaryForSeq returns true if this node is the primary validator for the sequence
	IsPrimaryForSeq(seq *big.Int) bool

	// SetChain injects the blockchain and related functions to the istanbul consensus engine
	SetChain(chain ChainReader, currentBlock func() *types.Block, stateAt func(common.Hash) (*state.StateDB, error))

	// SetBlockProcessors sets block processors
	SetBlockProcessors(hasBadBlock func(common.Hash) bool,
		processBlock func(*types.Block, *state.StateDB) (types.Receipts, []*types.Log, uint64, error),
		validateState func(*types.Block, *state.StateDB, types.Receipts, uint64) error) error

	// StartValidating starts the validating engine
	StartValidating() error

	// StopValidating stops the validating engine
	StopValidating() error

	// StartAnnouncing starts the announcing
	StartAnnouncing() error

	// StopAnnouncing stops the announcing
	StopAnnouncing() error

	// StartProxiedValidatorEngine starts the proxied validator engine
	StartProxiedValidatorEngine() error

	// StopProxiedValidatorEngine stops the proxied validator engine
	StopProxiedValidatorEngine() error

	// UpdateValSetDiff will update the validator set diff in the header, if the mined header is the last block of the epoch.
	// The changes are executed inline.
	UpdateValSetDiff(chain ChainReader, header *types.Header, state *state.StateDB) error

	// IsLastBlockOfEpoch will check to see if the header is from the last block of an epoch
	IsLastBlockOfEpoch(header *types.Header) bool

	// LookbackWindow returns the size of the lookback window for calculating uptime (in blocks)
	LookbackWindow(header *types.Header, state *state.StateDB) uint64

	// ValidatorAddress will return the istanbul engine's validator address
	ValidatorAddress() common.Address

	// GenerateRandomness will generate the random beacon randomness
	GenerateRandomness(parentHash common.Hash) (common.Hash, common.Hash, error)
}
