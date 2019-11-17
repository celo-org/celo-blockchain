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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type ProposerPolicy uint64

const (
	RoundRobin ProposerPolicy = iota
	Sticky
	ShuffledRoundRobin
)

type Config struct {
	RequestTimeout       uint64         `toml:",omitempty"` // The timeout for each Istanbul round in milliseconds.
	BlockPeriod          uint64         `toml:",omitempty"` // Default minimum difference between two consecutive block's timestamps in second
	ProposerPolicy       ProposerPolicy `toml:",omitempty"` // The policy for proposer selection
	Epoch                uint64         `toml:",omitempty"` // The number of blocks after which to checkpoint and reset the pending votes
	ValidatorEnodeDBPath string         `toml:",omitempty"` // The location for the validator enodes DB

	// Proxy Configs
	Proxy                   bool           `toml:",omitempty"` // Specifies if this node is a proxy
	ProxiedValidatorAddress common.Address `toml:",omitempty"` // The address of the proxied validator

	// Proxied Validator Configs
	Proxied                 bool        `toml:",omitempty"` // Specifies if this node is proxied
	ProxyInternalFacingNode *enode.Node `toml:",omitempty"` // The internal facing node of the proxy that this proxied validator will contect to
	ProxyExternalFacingNode *enode.Node `toml:",omitempty"` // The external facing node of the proxy that the proxied validator will broadcast via the announce message
}

var DefaultConfig = &Config{
	RequestTimeout:       3000,
	BlockPeriod:          1,
	ProposerPolicy:       RoundRobin,
	Epoch:                30000,
	ValidatorEnodeDBPath: "validatorenodes",
	Proxy:                false,
	Proxied:              false,
}
