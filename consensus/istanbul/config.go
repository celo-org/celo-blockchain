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

	ReplicaStateDBPath       string `toml:",omitempty"` // The location for the validator replica state DB
	ValidatorEnodeDBPath     string `toml:",omitempty"` // The location for the validator enodes DB
	VersionCertificateDBPath string `toml:",omitempty"` // The location for the signed announce version DB
	RoundStateDBPath         string `toml:",omitempty"` // The location for the round states DB
	Validator                bool   `toml:",omitempty"` // Specified if this node is configured to validate  (specifically if --mine command line is set)
	Replica                  bool   `toml:",omitempty"` // Specified if this node is configured to be a replica

	// Announce Configs
	AnnounceQueryEnodeGossipPeriod                 uint64 `toml:",omitempty"` // Time duration (in seconds) between gossiped query enode messages
	AnnounceAggressiveQueryEnodeGossipOnEnablement bool   `toml:",omitempty"` // Specifies if this node should aggressively query enodes on announce enablement
	AnnounceAdditionalValidatorsToGossip           int64  `toml:",omitempty"` // Specifies the number of additional non-elected validators to gossip an announce

	// Load test config
	LoadTestCSVFile string `toml:",omitempty"` // If non-empty, specifies the file to write out csv metrics about the block production cycle to.
}

// DefaultConfig for istanbul consensus engine
var DefaultConfig = &Config{
	RequestTimeout:                 3000,
	TimeoutBackoffFactor:           1000,
	MinResendRoundChangeTimeout:    15 * 1000,
	MaxResendRoundChangeTimeout:    2 * 60 * 1000,
	BlockPeriod:                    5,
	ProposerPolicy:                 ShuffledRoundRobin,
	Epoch:                          30000,
	ReplicaStateDBPath:             "replicastate",
	ValidatorEnodeDBPath:           "validatorenodes",
	VersionCertificateDBPath:       "versioncertificates",
	RoundStateDBPath:               "roundstates",
	Validator:                      false,
	Replica:                        false,
	AnnounceQueryEnodeGossipPeriod: 300, // 5 minutes
	AnnounceAggressiveQueryEnodeGossipOnEnablement: true,
	AnnounceAdditionalValidatorsToGossip:           10,
	LoadTestCSVFile:                                "", // disable by default
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
	config.ProposerPolicy = ProposerPolicy(chainConfig.Istanbul.ProposerPolicy)

	return nil
}
