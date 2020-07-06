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
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

// Decrypt is a decrypt callback function to request an ECIES ciphertext to be
// decrypted
type DecryptFn func(accounts.Account, []byte, []byte, []byte) ([]byte, error)

// SignerFn is a signer callback function to request a header to be signed by a
// backing account.
type SignerFn func(accounts.Account, string, []byte) ([]byte, error)

// BLSSignerFn is a signer callback function to request a message and extra data to be signed by a
// backing account using BLS with a direct or composite hasher
type BLSSignerFn func(accounts.Account, []byte, []byte, bool) (blscrypto.SerializedSignature, error)

// BackendForCore provides the Istanbul backend application specific functions for Istanbul core
type BackendForCore interface {
	// Address returns the owner's address
	Address() common.Address

	// Validators returns the validator set
	Validators(proposal Proposal) ValidatorSet
	NextBlockValidators(proposal Proposal) (ValidatorSet, error)

	// EventMux returns the event mux in backend
	EventMux() *event.TypeMux

	// Gossip will send a message to all connnected peers
	Gossip(payload []byte, ethMsgCode uint64) error

	// Multicast sends a message to it's connected nodes filtered on the 'addresses' parameter (where each address
	// is associated with those node's signing key)
	// If sendToSelf is set to true, then the function will send an event to self via a message event
	Multicast(addresses []common.Address, payload []byte, ethMsgCode uint64, sendToSelf bool) error

	// Commit delivers an approved proposal to backend.
	// The delivered proposal will be put into blockchain.
	Commit(proposal Proposal, aggregatedSeal types.IstanbulAggregatedSeal, aggregatedEpochValidatorSetSeal types.IstanbulEpochValidatorSetSeal) error

	// Verify verifies the proposal. If a consensus.ErrFutureBlock error is returned,
	// the time difference of the proposal and current time is also returned.
	Verify(Proposal) (time.Duration, error)

	// Sign signs input data with the backend's private key
	Sign([]byte) ([]byte, error)

	// Sign with the data with the BLS key, using either a direct or composite hasher
	SignBLS([]byte, []byte, bool) (blscrypto.SerializedSignature, error)

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

	// Authorize injects a private key into the consensus engine.
	Authorize(address common.Address, publicKey *ecdsa.PublicKey, decryptFn DecryptFn, signFn SignerFn, signBLSFn BLSSignerFn)
}

// BackendForProxy provides the Istanbul backend application specific functions for Istanbul proxy
type BackendForProxy interface {
	// Address returns the owner's address
	Address() common.Address

	// IsProxiedValidator returns true if this node is a proxied validator
	IsProxiedValidator() bool

	// IsProxy returns true if this node is a proxy
	IsProxy() bool

	// SelfNode returns the owner's node (if this is a proxy, it will return the external node)
	SelfNode() *enode.Node

	// Sign signs input data with the backend's private key
	Sign([]byte) ([]byte, error)

	// Multicast sends a message to it's connected nodes filtered on the 'addresses' parameter (where each address
	// is associated with those node's signing key)
	// If sendToSelf is set to true, then the function will send an event to self via a message event
	Multicast(addresses []common.Address, payload []byte, ethMsgCode uint64, sendToSelf bool) error

	// GetValEnodeTableEntries retrieves the entries in the valEnodeTable filtered on the "validators" parameter.
	// If the parameter is nil, then no filter will be applied.
	GetValEnodeTableEntries(validators []common.Address) (map[common.Address]*AddressEntry, error)

	// RewriteValEnodeTableEntries will rewrite the val enode table with "entries" rows.
	RewriteValEnodeTableEntries(entries []*AddressEntry) error

	// UpdateAnnounceVersion will notify the announce protocol that this validator's valEnodeTable entry has been updated
	UpdateAnnounceVersion()

	// SetEnodeCertificateMsg will set this node's enodeCertificate to be used for connection handshakes
	SetEnodeCertificateMsg(enodeCertificateMsg *Message) error

	// RetrieveEnodeCertificateMsg will retrieve this node's handshake enodeCertificate
	RetrieveEnodeCertificateMsg() (*Message, error)

	// VerifyPendingBlockValidatorSignature is a message validation function to verify that a message's sender is within the validator set
	// of the current pending block and that the message's address field matches the message's signature's signer
	VerifyPendingBlockValidatorSignature(data []byte, sig []byte) (common.Address, error)

	// VerifyValidatorConnectionSetSignature is a message validation function to verify that a message's sender is within the
	// validator connection set and that the message's address field matches the message's signature's signer
	VerifyValidatorConnectionSetSignature(data []byte, sig []byte) (common.Address, error)

	// RetrieveValidatorConnSet will retrieve the validator connection set.
	// The parameter `retrieveCachedVersion` will specify if the function should retrieve the
	// set directly from making an EVM call (which is relatively expensive), or from the cached
	// version (which will be no more than one minute old).
	RetrieveValidatorConnSet(retrieveCachedVersion bool) (map[common.Address]bool, error)

	GenerateEnodeCertificateMsg(externalEnodeURL string) (*Message, error)

	// AddPeer will add a static peer
	AddPeer(node *enode.Node, purpose p2p.PurposeFlag)

	// RemovePeer will remove a static peer
	RemovePeer(node *enode.Node, purpose p2p.PurposeFlag)
}
