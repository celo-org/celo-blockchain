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

package backend

import (
	"encoding/json"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/log"
)

const (
	dbKeySnapshotPrefix = "istanbul-snapshot"
)

// Snapshot is the state of the authorization voting at a given point in time.
type Snapshot struct {
	Epoch uint64 // The number of blocks for each epoch

	Number uint64                // Block number where the snapshot was created
	Hash   common.Hash           // Block hash where the snapshot was created
	ValSet istanbul.ValidatorSet // Set of authorized validators at this moment
}

// newSnapshot create a new snapshot with the specified startup parameters. This
// method does not initialize the set of recent validators, so only ever use if for
// the genesis block.
func newSnapshot(epoch uint64, number uint64, hash common.Hash, valSet istanbul.ValidatorSet) *Snapshot {
	snap := &Snapshot{
		Epoch:  epoch,
		Number: number,
		Hash:   hash,
		ValSet: valSet,
	}
	return snap
}

// loadSnapshot loads an existing snapshot from the database.
func loadSnapshot(epoch uint64, db ethdb.Database, hash common.Hash) (*Snapshot, error) {
	blob, err := db.Get(append([]byte(dbKeySnapshotPrefix), hash[:]...))
	if err != nil {
		return nil, err
	}
	snap := new(Snapshot)
	if err := json.Unmarshal(blob, snap); err != nil {
		return nil, err
	}

	if !snap.ValSet.HasBLSKeyCache() {
		log.Debug("Updating outdated snapshot", "hash", hash)
		if err := snap.store(db); err != nil {
			return nil, err
		}
	}

	snap.Epoch = epoch

	return snap, nil
}

// store inserts the snapshot into the database.
func (s *Snapshot) store(db ethdb.Database) error {
	s.ValSet.CacheUncompressedBLSKey()
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.Put(append([]byte(dbKeySnapshotPrefix), s.Hash[:]...), blob)
}

// copy creates a deep copy of the snapshot, though not the individual votes.
func (s *Snapshot) copy() *Snapshot {
	cpy := &Snapshot{
		Epoch:  s.Epoch,
		Number: s.Number,
		Hash:   s.Hash,
		ValSet: s.ValSet.Copy(),
	}

	return cpy
}

// apply creates a new authorization snapshot by applying the given headers to
// the original one.
func (s *Snapshot) apply(headers []*types.Header, db ethdb.Database) (*Snapshot, error) {
	// Allow passing in no headers for cleaner code
	if len(headers) == 0 {
		return s, nil
	}

	// Sanity check that the headers can be applied
	for i := 0; i < len(headers)-1; i++ {
		if headers[i+1].Number.Uint64() != headers[i].Number.Uint64()+s.Epoch {
			return nil, errInvalidVotingChain
		}
	}
	if headers[0].Number.Uint64() != s.Number+s.Epoch {
		return nil, errInvalidVotingChain
	}

	// Iterate through the headers and create a new snapshot
	snap := s.copy()

	for _, header := range headers {
		// Resolve the authorization key and check against validators
		validator, err := ecrecover(header)
		if err != nil {
			return nil, err
		}
		if _, v := snap.ValSet.GetByAddress(validator); v == nil {
			return nil, errUnauthorized
		}

		// Ensure that the extra data format is satisfied
		istExtra, err := header.IstanbulExtra()
		if err != nil {
			log.Error("Unable to extract the istanbul extra field from the header", "header", header)
			return nil, err
		}

		validators, err := istanbul.CombineIstanbulExtraToValidatorData(istExtra.AddedValidators, istExtra.AddedValidatorsPublicKeys)
		if err != nil {
			log.Error("Error in combining addresses and public keys")
			return nil, errInvalidValidatorSetDiff
		}

		if !snap.ValSet.RemoveValidators(istExtra.RemovedValidators) {
			log.Error("Error in removing the header's RemovedValidators")
			return nil, errInvalidValidatorSetDiff
		}
		if !snap.ValSet.AddValidators(validators) {
			log.Error("Error in adding the header's AddedValidators")
			return nil, errInvalidValidatorSetDiff
		}

		snap.Epoch = s.Epoch
		snap.Number += s.Epoch
		snap.Hash = header.Hash()
		snap.store(db)
		log.Trace("Stored voting snapshot to disk", "number", snap.Number, "hash", snap.Hash)
	}

	return snap, nil
}

func (s *Snapshot) validators() []istanbul.ValidatorData {
	return validator.MapValidatorsToData(s.ValSet.List())
}

type snapshotJSON struct {
	Epoch  uint64      `json:"epoch"`
	Number uint64      `json:"number"`
	Hash   common.Hash `json:"hash"`

	// for validator set
	Validators []istanbul.ValidatorDataWithBLSKeyCache `json:"validators"`
}

func (s *Snapshot) toJSONStruct() *snapshotJSON {
	validators := validator.MapValidatorsToDataWithBLSKeyCache(s.ValSet.List())
	return &snapshotJSON{
		Epoch:      s.Epoch,
		Number:     s.Number,
		Hash:       s.Hash,
		Validators: validators,
	}
}

// UnmarshalJSON from a json byte array
func (s *Snapshot) UnmarshalJSON(b []byte) error {
	var j snapshotJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}

	s.Epoch = j.Epoch
	s.Number = j.Number
	s.Hash = j.Hash
	s.ValSet = validator.NewSetFromDataWithBLSKeyCache(j.Validators)
	return nil
}

// MarshalJSON to a json byte array
func (s *Snapshot) MarshalJSON() ([]byte, error) {
	j := s.toJSONStruct()
	return json.Marshal(j)
}
