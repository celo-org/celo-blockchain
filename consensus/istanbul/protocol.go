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
	// Supported versions
	Celo67 = 67 // incorporates changes from eth/66 (EIP-2481)
)

// protocolName is the official short name of the protocol used during capability negotiation.
const ProtocolName = "istanbul"

// ProtocolVersions are the supported versions of the istanbul protocol (first is primary).
// (First is primary in the sense that it's the most current one supported)
var ProtocolVersions = []uint{Celo67}

// protocolLengths are the number of implemented message corresponding to different protocol versions.
// celo/67, uses as the last message the 0x18, so it has 25 messages (including the 0x00)
var ProtocolLengths = map[uint]uint64{Celo67: 25}

// Message codes for istanbul related messages
// If you want to add a code, you need to increment the protocolLengths Array size
// and update the IsIstanbulMsg function below!
const (
	ConsensusMsg           = 0x11
	QueryEnodeMsg          = 0x12
	ValEnodesShareMsg      = 0x13
	VersionCertificatesMsg = 0x16
	EnodeCertificateMsg    = 0x17
	ValidatorHandshakeMsg  = 0x18
)

func IsIstanbulMsg(msg p2p.Msg) bool {
	return msg.Code >= ConsensusMsg && msg.Code <= ValidatorHandshakeMsg
}
