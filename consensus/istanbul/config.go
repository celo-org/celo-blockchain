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

type ProposerPolicy uint64

const (
	RoundRobin ProposerPolicy = iota
	Sticky
)

type FaultyMode uint64

const (
	// Disabled disables the faulty mode
	Disabled FaultyMode = iota
	// Random will, at the time of any of the following actions, engage in faulty behavior with 50% probability
	Random
	// NotBroadcast doesn't broadcast any messages to other validators
	NotBroadcast
	// SendWrongMsg sends the message with the wrong message code
	SendWrongMsg
	// ModifySig modifies the message signature
	ModifySig
	// AlwaysPropose always proposes a proposal to validators
	AlwaysPropose
	// AlwaysRoundChange always sends round change while receiving messages
	AlwaysRoundChange
	// BadBlock always proposes a block with bad body
	BadBlock
)

func (f FaultyMode) Uint64() uint64 {
	return uint64(f)
}

func (f FaultyMode) String() string {
	switch f {
	case Disabled:
		return "Disabled"
	case Random:
		return "Random"
	case NotBroadcast:
		return "NotBroadcast"
	case SendWrongMsg:
		return "SendWrongMsg"
	case ModifySig:
		return "ModifySig"
	case AlwaysPropose:
		return "AlwaysPropose"
	case AlwaysRoundChange:
		return "AlwaysRoundChange"
	case BadBlock:
		return "BadBlock"
	default:
		return "Undefined"
	}
}

type Config struct {
	RequestTimeout uint64         `toml:",omitempty"` // The timeout for each Istanbul round in milliseconds.
	BlockPeriod    uint64         `toml:",omitempty"` // Default minimum difference between two consecutive block's timestamps in second
	ProposerPolicy ProposerPolicy `toml:",omitempty"` // The policy for proposer selection
	Epoch          uint64         `toml:",omitempty"` // The number of blocks after which to checkpoint and reset the pending votes
	FaultyMode     uint64         `toml:",omitempty"` // The faulty node indicates the faulty node's behavior
}

var DefaultConfig = &Config{
	RequestTimeout: 10000,
	BlockPeriod:    1,
	ProposerPolicy: RoundRobin,
	Epoch:          30000,
	FaultyMode:     Disabled.Uint64(),
}
