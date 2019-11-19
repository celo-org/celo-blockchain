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

package istanbul

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// SignerFn is a signer callback function to request a hash to be signed by a
// backing account.
type SignerFn func(accounts.Account, []byte) ([]byte, error)

// MessageSignerFn is a signer callback function to request a raw message to
// be signed by a backing account.
type MessageSignerFn func(accounts.Account, []byte, []byte) ([]byte, error)

// Backend provides application specific functions for Istanbul core
type Backend interface {
	// Address returns the owner's address
	Address() common.Address

	// Validators returns the validator set
	Validators(proposal Proposal) ValidatorSet

	// EventMux returns the event mux in backend
	EventMux() *event.TypeMux

	// BroadcastConsensusMsg sends a message to all validators (include self)
	BroadcastConsensusMsg(validators []common.Address, payload []byte) error

	// Gossip sends a message to all validators (exclude self)
	Gossip(validators []common.Address, payload []byte, ethMsgCode uint64, ignoreCache bool) error

	// Commit delivers an approved proposal to backend.
	// The delivered proposal will be put into blockchain.
	Commit(proposal Proposal, aggregatedSeal types.IstanbulAggregatedSeal) error

	// Verify verifies the proposal. If a consensus.ErrFutureBlock error is returned,
	// the time difference of the proposal and current time is also returned.
	Verify(Proposal) (time.Duration, error)

	// Sign signs input data with the backend's private key
	Sign([]byte) ([]byte, error)
	SignBlockHeader([]byte) ([]byte, error)

	// CheckSignature verifies the signature by checking if it's signed by
	// the given validator
	CheckSignature(data []byte, addr common.Address, sig []byte) error

	// LastProposal retrieves latest committed proposal and the address of proposer
	LastProposal() (Proposal, common.Address)

	// LastSubject retrieves latest committed subject (view and digest)
	LastSubject() (Subject, error)

	// HasProposal checks if the combination of the given hash and height matches any existing blocks
	HasProposal(hash common.Hash, number *big.Int) bool

	// GetProposer returns the proposer of the given block height
	GetProposer(number uint64) common.Address

	// ParentValidators returns the validator set of the given proposal's parent block
	ParentValidators(proposal Proposal) ValidatorSet

	// RefreshValPeers will connect with all the validators in the valset and disconnect validator peers that are not in the set
	RefreshValPeers(valset ValidatorSet)

	// Authorize injects a private key into the consensus engine.
	Authorize(address common.Address, signFn SignerFn, signHashBLSFn SignerFn, signMessageBLSFn MessageSignerFn)

	// GetDataDir returns a read-write enabled data dir in which data will persist across restarts.
	GetDataDir() string
}
