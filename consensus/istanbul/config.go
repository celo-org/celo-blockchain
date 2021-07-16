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
	"fmt"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/params"
)

const (
	//MinEpochSize represents the minimum permissible epoch size
	MinEpochSize = 3
)

// ProposerPolicy represents the policy used to order elected validators within an epoch
type ProposerPolicy uint64

const (
	RoundRobin ProposerPolicy = iota
	Sticky
	ShuffledRoundRobin
)

// Config represents the istanbul consensus engine
type Config struct {
	RequestTimeout              uint64         `toml:",omitempty"` // The timeout for each Istanbul round in milliseconds.
	TimeoutBackoffFactor        uint64         `toml:",omitempty"` // Timeout at subsequent rounds is: RequestTimeout + 2**round * TimeoutBackoffFactor (in milliseconds)
	MinResendRoundChangeTimeout uint64         `toml:",omitempty"` // Minimum interval with which to resend RoundChange messages for same round
	MaxResendRoundChangeTimeout uint64         `toml:",omitempty"` // Maximum interval with which to resend RoundChange messages for same round
	BlockPeriod                 uint64         `toml:",omitempty"` // Default minimum difference between two consecutive block's timestamps in second
	ProposerPolicy              ProposerPolicy `toml:",omitempty"` // The policy for proposer selection
	Epoch                       uint64         `toml:",omitempty"` // The number of blocks after which to checkpoint and reset the pending votes
	DefaultLookbackWindow       uint64         `toml:",omitempty"` // The default value for how many blocks in a row a validator must miss to be considered "down"
	ReplicaStateDBPath          string         `toml:",omitempty"` // The location for the validator replica state DB
	ValidatorEnodeDBPath        string         `toml:",omitempty"` // The location for the validator enodes DB
	VersionCertificateDBPath    string         `toml:",omitempty"` // The location for the signed announce version DB
	RoundStateDBPath            string         `toml:",omitempty"` // The location for the round states DB
	Validator                   bool           `toml:",omitempty"` // Specified if this node is configured to validate  (specifically if --mine command line is set)
	Replica                     bool           `toml:",omitempty"` // Specified if this node is configured to be a replica

	// Proxy Configs
	Proxy                   bool           `toml:",omitempty"` // Specifies if this node is a proxy
	ProxiedValidatorAddress common.Address `toml:",omitempty"` // The address of the proxied validator

	// Proxied Validator Configs
	Proxied      bool           `toml:",omitempty"` // Specifies if this node is proxied
	ProxyConfigs []*ProxyConfig `toml:",omitempty"` // The set of proxy configs for this proxied validator at startup

	// Announce Configs
	QueryEnodeGossipPeriod                 uint64 `toml:",omitempty"` // Time duration (in seconds) between gossiped query enode messages
	AggressiveQueryEnodeGossipOnEnablement bool   `toml:",omitempty"` // Specifies if this node should aggressively query enodes on announce enablement
	AdditionalValidatorsToGossip           int64  `toml:",omitempty"` // Specifies the number of additional non-elected validators to gossip an announce

	// Load test config
	LoadTestCSVFile string `toml:",omitempty"` // If non-empty, specifies the file to write out csv metrics about the block production cycle to.
}

// ProxyConfig represents the configuration for validator's proxies
type ProxyConfig struct {
	InternalNode *enode.Node `toml:",omitempty"` // The internal facing node of the proxy that this proxied validator will peer with
	ExternalNode *enode.Node `toml:",omitempty"` // The external facing node of the proxy that the proxied validator will broadcast via the announce message
}

// DefaultConfig for istanbul consensus engine
var DefaultConfig = &Config{
	RequestTimeout:                         3000,
	TimeoutBackoffFactor:                   1000,
	MinResendRoundChangeTimeout:            15 * 1000,
	MaxResendRoundChangeTimeout:            2 * 60 * 1000,
	BlockPeriod:                            5,
	ProposerPolicy:                         ShuffledRoundRobin,
	Epoch:                                  30000,
	DefaultLookbackWindow:                  12,
	ReplicaStateDBPath:                     "replicastate",
	ValidatorEnodeDBPath:                   "validatorenodes",
	VersionCertificateDBPath:               "versioncertificates",
	RoundStateDBPath:                       "roundstates",
	Validator:                              false,
	Replica:                                false,
	Proxy:                                  false,
	Proxied:                                false,
	QueryEnodeGossipPeriod:                 300, // 5 minutes
	AggressiveQueryEnodeGossipOnEnablement: true,
	AdditionalValidatorsToGossip:           10,
	LoadTestCSVFile:                        "", // disable by default
}

//ApplyParamsChainConfigToConfig applies the istanbul config values from params.chainConfig to the istanbul.Config config
func ApplyParamsChainConfigToConfig(chainConfig *params.ChainConfig, config *Config) error {
	if chainConfig.Istanbul.Epoch != 0 {
		if chainConfig.Istanbul.Epoch < MinEpochSize {
			return fmt.Errorf("istanbul.Epoch must be greater than %d", MinEpochSize-1)
		}

		config.Epoch = chainConfig.Istanbul.Epoch
	}
	if chainConfig.Istanbul.RequestTimeout != 0 {
		config.RequestTimeout = chainConfig.Istanbul.RequestTimeout
	}
	if chainConfig.Istanbul.BlockPeriod != 0 {
		config.BlockPeriod = chainConfig.Istanbul.BlockPeriod
	}
	if chainConfig.Istanbul.LookbackWindow != 0 {
		config.DefaultLookbackWindow = chainConfig.Istanbul.LookbackWindow
	}
	if chainConfig.Istanbul.LookbackWindow >= chainConfig.Istanbul.Epoch-2 {
		return fmt.Errorf("istanbul.lookbackwindow must be less than istanbul.epoch-2")
	}
	config.ProposerPolicy = ProposerPolicy(chainConfig.Istanbul.ProposerPolicy)

	return nil
}
