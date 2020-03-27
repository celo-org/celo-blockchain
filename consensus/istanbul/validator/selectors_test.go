// Copyright 2019 The Celo Authors
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

package validator

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
)

var testAddresses = []string{
	"0000000000000000000000000000000000000001",
	"0000000000000000000000000000000000000002",
	"0000000000000000000000000000000000000003",
	"0000000000000000000000000000000000000004",
	"0000000000000000000000000000000000000005",
}

func TestStickyProposer(t *testing.T) {
	var addrs []common.Address
	var validators []istanbul.Validator
	for _, strAddr := range testAddresses {
		addr := common.HexToAddress(strAddr)
		addrs = append(addrs, addr)
		validators = append(validators, New(addr, blscrypto.SerializedPublicKey{}))
	}

	v, err := istanbul.CombineIstanbulExtraToValidatorData(addrs, make([]blscrypto.SerializedPublicKey, len(addrs)))
	if err != nil {
		t.Fatalf("CombineIstanbulExtraToValidatorData(...): %v", err)
	}
	valSet := newDefaultSet(v)
	selector := GetProposerSelector(istanbul.Sticky)

	cases := []struct {
		lastProposer common.Address
		round        uint64
		want         istanbul.Validator
	}{{
		lastProposer: addrs[0],
		round:        0,
		want:         validators[0],
	}, {
		lastProposer: addrs[0],
		round:        1,
		want:         validators[1],
	}, {
		lastProposer: addrs[0],
		round:        2,
		want:         validators[2],
	}, {
		lastProposer: addrs[2],
		round:        2,
		want:         validators[4],
	}, {
		lastProposer: addrs[2],
		round:        3,
		want:         validators[0],
	}, {
		lastProposer: common.Address{},
		round:        3,
		want:         validators[3],
	}}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case:%d", i), func(t *testing.T) {
			t.Logf("selectProposer(%s, %d)", c.lastProposer.String(), c.round)
			proposer := selector(valSet, c.lastProposer, c.round)
			if val := proposer; !reflect.DeepEqual(val, c.want) {
				t.Errorf("proposer mismatch: have %v, want %v", val, c.want)
			}
		})
	}
}

func TestRoundRobinProposer(t *testing.T) {
	var addrs []common.Address
	var validators []istanbul.Validator
	for _, strAddr := range testAddresses {
		addr := common.HexToAddress(strAddr)
		addrs = append(addrs, addr)
		validators = append(validators, New(addr, blscrypto.SerializedPublicKey{}))
	}

	v, err := istanbul.CombineIstanbulExtraToValidatorData(addrs, make([]blscrypto.SerializedPublicKey, len(addrs)))
	if err != nil {
		t.Fatalf("CombineIstanbulExtraToValidatorData(...): %v", err)
	}
	valSet := newDefaultSet(v)
	selector := GetProposerSelector(istanbul.RoundRobin)

	cases := []struct {
		lastProposer common.Address
		round        uint64
		want         istanbul.Validator
	}{{
		lastProposer: addrs[0],
		round:        0,
		want:         validators[1],
	}, {
		lastProposer: addrs[0],
		round:        1,
		want:         validators[2],
	}, {
		lastProposer: addrs[0],
		round:        2,
		want:         validators[3],
	}, {
		lastProposer: addrs[2],
		round:        2,
		want:         validators[0],
	}, {
		lastProposer: addrs[2],
		round:        3,
		want:         validators[1],
	}, {
		lastProposer: common.Address{},
		round:        3,
		want:         validators[3],
	}}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case:%d", i), func(t *testing.T) {
			t.Logf("selectProposer(%s, %d)", c.lastProposer.String(), c.round)
			proposer := selector(valSet, c.lastProposer, c.round)
			if val := proposer; !reflect.DeepEqual(val, c.want) {
				t.Errorf("proposer mismatch: have %v, want %v", val, c.want)
			}
		})
	}
}

func TestShuffledRoundRobinProposer(t *testing.T) {
	var addrs []common.Address
	var validators []istanbul.Validator
	for _, strAddr := range testAddresses {
		addr := common.HexToAddress(strAddr)
		addrs = append(addrs, addr)
		validators = append(validators, New(addr, blscrypto.SerializedPublicKey{}))
	}

	v, err := istanbul.CombineIstanbulExtraToValidatorData(addrs, make([]blscrypto.SerializedPublicKey, len(addrs)))
	if err != nil {
		t.Fatalf("CombineIstanbulExtraToValidatorData(...): %v", err)
	}
	valSet := newDefaultSet(v)
	selector := GetProposerSelector(istanbul.ShuffledRoundRobin)

	// Verify a number of explicit cases with expected output.
	testSeed := common.HexToHash("f36aa9716b892ec8")
	cases := []struct {
		lastProposer common.Address
		round        uint64
		seed         common.Hash
		want         istanbul.Validator
	}{{
		lastProposer: common.Address{},
		round:        0,
		want:         validators[0],
	}, {
		lastProposer: common.Address{},
		round:        3,
		want:         validators[2],
	}, {
		lastProposer: addrs[0],
		round:        0,
		want:         validators[4],
	}, {
		lastProposer: addrs[0],
		round:        1,
		want:         validators[3],
	}, {
		lastProposer: addrs[0],
		round:        2,
		want:         validators[2],
	}, {
		lastProposer: addrs[2],
		round:        2,
		want:         validators[4],
	}, {
		lastProposer: addrs[2],
		round:        3,
		want:         validators[3],
	}, {
		lastProposer: common.Address{},
		round:        0,
		seed:         testSeed,
		want:         validators[1],
	}, {
		lastProposer: addrs[0],
		round:        0,
		seed:         testSeed,
		want:         validators[2],
	}, {
		lastProposer: addrs[0],
		round:        1,
		seed:         testSeed,
		want:         validators[1],
	}, {
		lastProposer: addrs[0],
		round:        2,
		seed:         testSeed,
		want:         validators[4],
	}}

	for i, c := range cases {
		t.Run(fmt.Sprintf("case:%d", i), func(t *testing.T) {
			t.Logf("SetRandomness(%s)", c.seed.String())
			valSet.SetRandomness(c.seed)
			t.Logf("selectProposer(%s, %d)", c.lastProposer.String(), c.round)
			proposer := selector(valSet, c.lastProposer, c.round)
			if val := proposer; !reflect.DeepEqual(val, c.want) {
				t.Errorf("proposer mismatch: have %v, want %v", val, c.want)
			}
		})
	}

	// Verify that the ordering is a stable round robin during round changes.
	valSet.SetRandomness(testSeed)
	t.Run("round changes", func(t *testing.T) {
		var lastProposer common.Address
		order := make([]common.Address, len(validators))
		for round := uint64(0); round < 100; round++ {
			proposer := selector(valSet, lastProposer, round)
			index := round % uint64(len(validators))
			if want := order[index]; want != (common.Address{}) {
				if proposer.Address() != want {
					t.Errorf("proposer mismatch on round %d: have %v, want %v", round, proposer.Address(), want)
				}
			} else {
				order[index] = proposer.Address()
			}
		}
	})

	// Verify that the ordering is a stable round robin during sequence advancement.
	t.Run("sequence advancement", func(t *testing.T) {
		var lastProposer common.Address
		order := make([]common.Address, len(validators))
		for seq := 0; seq < 100; seq++ {
			proposer := selector(valSet, lastProposer, 0)
			index := seq % len(validators)
			if want := order[index]; want != (common.Address{}) {
				if proposer.Address() != want {
					t.Errorf("proposer mismatch on sequence %d: have %v, want %v", seq, proposer.Address(), want)
				}
			} else {
				order[index] = proposer.Address()
			}
		}
	})
}
