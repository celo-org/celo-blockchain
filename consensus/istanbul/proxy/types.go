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
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	// errProxyAlreadySet is returned if a user tries to add a proxy that is already set.
	// TODO - When we support multiple sentries per validator, this error will become irrelevant.
	errProxyAlreadySet = errors.New("proxy already set")

	// errUnauthorizedMessageFromProxiedValidator is returned when the received message expected to be signed
	// by the proxied validator, but signed from another key
	errUnauthorizedMessageFromProxiedValidator = errors.New("message not authorized by proxied validator")

	// errNoConnectedProxy is returned when there is no connected proxy
	errNoConnectedProxy = errors.New("no connected proxy")

	// errNoConnectedProxiedValidator is returned when there is no connected proxied validator
	errNoConnectedProxiedValidator = errors.New("no connected proxied validator")

	// errInvalidEnodeCertificate is returned if the enode certificate is invalid
	errInvalidEnodeCertificate = errors.New("invalid enode certificate")

	// errUnauthorizedProxiedValidator is returned if the peer connecting is not the
	// authorized proxied validator
	errUnauthorizedProxiedValidator = errors.New("unauthorized proxied validator")
)

type ProxyEngine interface {
	Start() error
	Stop() error
	GetProxyExternalNode() *enode.Node
	AddProxy(node, externalNode *enode.Node) error
	RemoveProxy(node *enode.Node)
	HandleMsg(peer consensus.Peer, msgCode uint64, payload []byte) (bool, error)
	SendValEnodesShareMsg()
	RegisterProxiedValidator(proxiedValidatorPeer consensus.Peer)
	RegisterProxy(proxyPeer consensus.Peer) error
	UnregisterProxiedValidator(proxiedValidatorPeer consensus.Peer)
	UnregisterProxy(proxyPeer consensus.Peer)
	SendEnodeCertificateMsgToProxiedValidator(msg *istanbul.Message) error
	SendForwardMsg(finalDestAddresses []common.Address, ethMsgCode uint64, payload []byte) error
	SendDelegateSignMsgToProxy(msg []byte) error
	SendDelegateSignMsgToProxiedValidator(msg []byte) error
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

// ProxyInfo is used to provide info on a proxy that can be given via an RPC
type ProxyInfo struct {
	InternalNode *enode.Node      `json:"internalEnodeUrl"`
	ExternalNode *enode.Node      `json:"externalEnodeUrl"`
	IsPeered     bool             `json:"isPeered"`
	Validators   []common.Address `json:"validators"`            // All validator addresses assigned to the proxy
	DisconnectTS int64            `json:"disconnectedTimestamp"` // Unix time of the last disconnect of the peer
}

func (p proxy) ID() enode.ID {
	return p.node.ID()
}

func (p proxy) Info() ProxyInfo {
	return ProxyInfo{
		InternalNode: p.node,
		ExternalNode: p.externalNode,
		IsPeered:     p.peer != nil,
		DisconnectTS: p.disconnectTS.Unix(),
	}
}

func (p proxy) String() string {
	return fmt.Sprintf("{internalNode: %v, externalNode %v, dcTimestamp: %v, ID: %v}", p.node, p.externalNode, p.disconnectTS, p.ID())
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
