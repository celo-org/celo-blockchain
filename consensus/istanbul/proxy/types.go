// Copyright 2017 The celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package proxy

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	// errUnauthorizedMessageFromProxiedValidator is returned when the received message expected to be signed
	// by the proxied validator, but signed from another key
	errUnauthorizedMessageFromProxiedValidator = errors.New("message not authorized by proxied validator")

	// errNoAssignedProxies is returned when there is no assigned proxy
	errNoAssignedProxies = errors.New("no assigned proxies")

	// errNoConnectedProxiedValidator is returned when there is no connected proxied validator
	errNoConnectedProxiedValidator = errors.New("no connected proxied validator")

	// errInvalidEnodeCertificate is returned if the enode certificate is invalid
	errInvalidEnodeCertificate = errors.New("invalid enode certificate")

	// errUnauthorizedProxiedValidator is returned if the peer connecting is not the
	// authorized proxied validator
	errUnauthorizedProxiedValidator = errors.New("unauthorized proxied validator")

	// ErrStoppedProxyHandler is returned if proxy handler is stopped
	ErrStoppedProxyHandler = errors.New("stopped proxy handler")

	// ErrStartedProxyHandler is returned if proxy handler is already started
	ErrStartedProxyHandler = errors.New("started proxy handler")

	// ErrNodeNotProxiedValidator is returned if this node is not a proxied validator
	ErrNodeNotProxiedValidator = errors.New("node not a proxied validator")

	// ErrNodeNotProxy is returned if this node is not a proxy
	ErrNodeNotProxy = errors.New("node not a proxy")
)

// BackendForProxyEngine provides the Istanbul backend application specific functions for Istanbul proxy engine
type BackendForProxyEngine interface {
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
	GetValEnodeTableEntries(validators []common.Address) (map[common.Address]*istanbul.AddressEntry, error)

	// RewriteValEnodeTableEntries will rewrite the val enode table with "entries" rows.
	RewriteValEnodeTableEntries(entries []*istanbul.AddressEntry) error

	// UpdateAnnounceVersion will notify the announce protocol that this validator's valEnodeTable entry has been updated
	UpdateAnnounceVersion()

	// SetEnodeCertificateMsg will set this node's enodeCertificate to be used for connection handshakes
	SetEnodeCertificateMsgMap(enodeCertificateMsgMap map[enode.ID]*istanbul.Message) error

	// RetrieveEnodeCertificateMsgMap will retrieve this node's handshake enodeCertificate
	RetrieveEnodeCertificateMsgMap() map[enode.ID]*istanbul.Message

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

	GenerateEnodeCertificateMsg(externalEnodeURL string) (*istanbul.Message, error)

	// AddPeer will add a static peer
	AddPeer(node *enode.Node, purpose p2p.PurposeFlag)

	// RemovePeer will remove a static peer
	RemovePeer(node *enode.Node, purpose p2p.PurposeFlag)
}

type ProxyEngine interface {
	Start() error
	Stop() error
	HandleMsg(peer consensus.Peer, msgCode uint64, payload []byte) (bool, error)
	AddProxy(node, externalNode *enode.Node) error
	RemoveProxy(node *enode.Node) error
	RegisterProxyPeer(proxyPeer consensus.Peer) error
	UnregisterProxyPeer(proxyPeer consensus.Peer)
	RegisterProxiedValidatorPeer(proxiedValidatorPeer consensus.Peer)
	UnregisterProxiedValidatorPeer(proxiedValidatorPeer consensus.Peer)
	SendEnodeCertificateMsgToProxiedValidator(msg *istanbul.Message) error
	SendForwardMsg(proxyPeers []consensus.Peer, finalDestAddresses []common.Address, ethMsgCode uint64, payload []byte, proxySpecificPayload map[enode.ID][]byte) error
	// SendDelegateSignMsgToProxy(msg []byte) error
	// SendDelegateSignMsgToProxiedValidator(msg []byte) error
	SendValEnodesShareMsg(proxyPeer consensus.Peer, remoteValidators []common.Address) error
	SendValEnodesShareMsgToAllProxies()
	GetValidatorProxyAssignments() (map[common.Address]*enode.Node, error)
	GetProxiesAndValAssignments() ([]*proxy, map[enode.ID][]common.Address, error)
	GetProxiedValidatorsInfo() ([]ProxiedValidatorInfo, error)
}

// ==============================================
//
// define the proxy object

type proxy struct {
	node         *enode.Node    // Enode for the internal network interface
	externalNode *enode.Node    // Enode for the external network interface
	peer         consensus.Peer // Connected proxy peer.  Is nil if this node is not connected to the proxy
	disconnectTS time.Time      // Timestamp when this proxy's peer last disconnected. Initially set to the timestamp of when the proxy was added
}

func (p *proxy) ID() enode.ID {
	return p.node.ID()
}

func (p *proxy) ExternalNode() *enode.Node {
	return p.externalNode
}

func (p *proxy) IsPeered() bool {
	return p.peer != nil
}

func (p *proxy) String() string {
	return fmt.Sprintf("{internalNode: %v, externalNode %v, dcTimestamp: %v, ID: %v}", p.node, p.externalNode, p.disconnectTS, p.ID())
}

// ProxyInfo is used to provide info on a proxy that can be given via an RPC
type ProxyInfo struct {
	InternalNode             *enode.Node      `json:"internalEnodeUrl"`
	ExternalNode             *enode.Node      `json:"externalEnodeUrl"`
	IsPeered                 bool             `json:"isPeered"`
	AssignedRemoteValidators []common.Address `json:"validators"`            // All validator addresses assigned to the proxy
	DisconnectTS             int64            `json:"disconnectedTimestamp"` // Unix time of the last disconnect of the peer
}

func NewProxyInfo(p *proxy, assignedVals []common.Address) *ProxyInfo {
	return &ProxyInfo{
		InternalNode:             p.node,
		ExternalNode:             p.ExternalNode(),
		IsPeered:                 p.IsPeered(),
		DisconnectTS:             p.disconnectTS.Unix(),
		AssignedRemoteValidators: assignedVals,
	}
}

// ==============================================
//
// define the proxied validator info object

type ProxiedValidatorInfo struct {
	Address  common.Address `json:"address"`
	IsPeered bool           `json:"isPeered"`
	Node     *enode.Node    `json:"enodeURL"`
}

// ==============================================
//
// define the validator enode share message

type sharedValidatorEnode struct {
	Address  common.Address
	EnodeURL string
	Version  uint
}

type valEnodesShareData struct {
	ValEnodes []sharedValidatorEnode
}

func (sve *sharedValidatorEnode) String() string {
	return fmt.Sprintf("{Address: %s, EnodeURL: %v, Version: %v}", sve.Address.Hex(), sve.EnodeURL, sve.Version)
}

func (sd *valEnodesShareData) String() string {
	outputStr := "{ValEnodes:"
	for _, valEnode := range sd.ValEnodes {
		outputStr = fmt.Sprintf("%s %s", outputStr, valEnode.String())
	}
	return fmt.Sprintf("%s}", outputStr)
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes sd into the Ethereum RLP format.
func (sd *valEnodesShareData) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{sd.ValEnodes})
}

// DecodeRLP implements rlp.Decoder, and load the sd fields from a RLP stream.
func (sd *valEnodesShareData) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		ValEnodes []sharedValidatorEnode
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	sd.ValEnodes = msg.ValEnodes
	return nil
}
