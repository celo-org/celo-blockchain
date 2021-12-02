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

package istanbul

import (
	"github.com/celo-org/celo-blockchain/p2p"
)

// Constants to match up protocol versions and messages
const (
	// No longer supported, can be removed.
	// The corresponding version upstream (eth/65) is removed in upstream PR #22636.
	Celo64 = 64 // eth/63 + the istanbul messages
	Celo65 = 65 // incorporates changes from eth/64 (EIP)

	// Supported versions
	Celo66 = 66 // incorporates changes from eth/65 (EIP-2464)
	Celo67 = 67 // incorporates changes from eth/66 (EIP-2481)
)

// protocolName is the official short name of the protocol used during capability negotiation.
const ProtocolName = "istanbul"

// ProtocolVersions are the supported versions of the istanbul protocol (first is primary).
// (First is primary in the sense that it's the most current one supported)
var ProtocolVersions = []uint{Celo67, Celo66}

// protocolLengths are the number of implemented message corresponding to different protocol versions.
var ProtocolLengths = map[uint]uint64{Celo64: 22, Celo65: 27, Celo66: 27, Celo67: 25}

// NOTICE:
// celo/67, uses as the last message the 0x18, so it has 25 messages (including the 0x00)
// celo/66 has 27 as an unharm effect of removing 2 messages before the mainnet launch, that were
// never updated with the actual number (https://github.com/celo-org/celo-blockchain/pull/893)
// The Celo66 protocol has actually 25 messages, but we keep it with 27 to avoid problems.

// Message codes for istanbul related messages
// If you want to add a code, you need to increment the protocolLengths Array size
// and update the IsIstanbulMsg function below!
const (
	ConsensusMsg           = 0x11
	QueryEnodeMsg          = 0x12
	ValEnodesShareMsg      = 0x13
	FwdMsg                 = 0x14
	DelegateSignMsg        = 0x15
	VersionCertificatesMsg = 0x16
	EnodeCertificateMsg    = 0x17
	ValidatorHandshakeMsg  = 0x18
)

func IsIstanbulMsg(msg p2p.Msg) bool {
	return msg.Code >= ConsensusMsg && msg.Code <= ValidatorHandshakeMsg
}

// IsGossipedMsg specifies which messages should be gossiped throughout the network (as opposed to directly sent to a peer).
func IsGossipedMsg(msgCode uint64) bool {
	return msgCode == QueryEnodeMsg || msgCode == VersionCertificatesMsg
}
