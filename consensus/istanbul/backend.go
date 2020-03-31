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
	"crypto/ecdsa"
	"math/big"
	"time"

	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Decrypt is a decrypt callback function to request an ECIES ciphertext to be
// decrypted
type DecryptFn func(accounts.Account, []byte, []byte, []byte) ([]byte, error)

// SignerFn is a signer callback function to request a header to be signed by a
// backing account.
type SignerFn func(accounts.Account, string, []byte) ([]byte, error)

// BLSSignerFn is a signer callback function to request a hash to be signed by a
// backing account using BLS.
type BLSSignerFn func(accounts.Account, []byte) (blscrypto.SerializedSignature, error)

// BLSMessageSignerFn is a signer callback function to request a raw message to
// be signed by a backing account.
type BLSMessageSignerFn func(accounts.Account, []byte, []byte) (blscrypto.SerializedSignature, error)

// Backend provides application specific functions for Istanbul core
type Backend interface {
	// Address returns the owner's address
	Address() common.Address

	// Validators returns the validator set
	Validators(proposal Proposal) ValidatorSet
	NextBlockValidators(proposal Proposal) (ValidatorSet, error)

	// EventMux returns the event mux in backend
	EventMux() *event.TypeMux

	// BroadcastConsensusMsg sends a message to all validators (include self)
	BroadcastConsensusMsg(validators []common.Address, payload []byte) error

	// Multicast sends a message to it's connected nodes filtered on the 'addresses' parameter (where each address
	// is associated with those node's signing key)
	// If that parameter is nil, then it will send the message to all it's connected peers.
	Multicast(addresses []common.Address, payload []byte, ethMsgCode uint64) error

	// Commit delivers an approved proposal to backend.
	// The delivered proposal will be put into blockchain.
	Commit(proposal Proposal, aggregatedSeal types.IstanbulAggregatedSeal, aggregatedEpochValidatorSetSeal types.IstanbulEpochValidatorSetSeal) error

	// Verify verifies the proposal. If a consensus.ErrFutureBlock error is returned,
	// the time difference of the proposal and current time is also returned.
	Verify(Proposal) (time.Duration, error)

	// Sign signs input data with the backend's private key
	Sign([]byte) ([]byte, error)
	SignBlockHeader([]byte) (blscrypto.SerializedSignature, error)
	SignBLSWithCompositeHash([]byte) (blscrypto.SerializedSignature, error)

	// CheckSignature verifies the signature by checking if it's signed by
	// the given validator
	CheckSignature(data []byte, addr common.Address, sig []byte) error

	// GetCurrentHeadBlock retrieves the last block
	GetCurrentHeadBlock() Proposal

	// GetCurrentHeadBlockAndAuthor retrieves the last block alongside the author for that block
	GetCurrentHeadBlockAndAuthor() (Proposal, common.Address)

	// LastSubject retrieves latest committed subject (view and digest)
	LastSubject() (Subject, error)

	// HasBlock checks if the combination of the given hash and height matches any existing blocks
	HasBlock(hash common.Hash, number *big.Int) bool

	// AuthorForBlock returns the proposer of the given block height
	AuthorForBlock(number uint64) common.Address

	// ParentBlockValidators returns the validator set of the given proposal's parent block
	ParentBlockValidators(proposal Proposal) ValidatorSet

	// RefreshValPeers will connect with all the validators in the validator connection set and disconnect validator peers that are not in the set
	RefreshValPeers() error

	// Authorize injects a private key into the consensus engine.
	Authorize(address common.Address, publicKey *ecdsa.PublicKey, decryptFn DecryptFn, signFn SignerFn, signHashBLSFn BLSSignerFn, signMessageBLSFn BLSMessageSignerFn)
}
