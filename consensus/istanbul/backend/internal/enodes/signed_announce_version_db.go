// Copyright 2017 The Celo Authors
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

package enodes

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	signedAnnounceVersionDBVersion = 0
)

// SignedAnnounceVersionDB stores
type SignedAnnounceVersionDB struct {
	vedb   *versionedEntryDB
	logger log.Logger
}

// SignedAnnounceVersionEntry is an entry in the SignedAnnounceVersionDB.
// It's a signed message from a registered or active validator indicating
// the most recent version of its enode.
// This implements the versionedEntry interface.
type SignedAnnounceVersionEntry struct {
	Address   common.Address
	PublicKey *ecdsa.PublicKey
	Version   uint
	Signature []byte
}

func signedAnnounceVersionEntryFromVersionedEntry(entry versionedEntry) (*SignedAnnounceVersionEntry, error) {
	signedAnnVersionEntry, ok := entry.(*SignedAnnounceVersionEntry)
	if !ok {
		return nil, errIncorrectEntryType
	}
	return signedAnnVersionEntry, nil
}

// GetVersion gets the entry's version. This implements the GetVersion function
// for the versionedEntry interface
func (entry *SignedAnnounceVersionEntry) GetVersion() uint {
	return entry.Version
}

// EncodeRLP serializes SignedAnnounceVersionEntry into the Ethereum RLP format.
func (entry *SignedAnnounceVersionEntry) EncodeRLP(w io.Writer) error {
	encodedPublicKey := crypto.FromECDSAPub(entry.PublicKey)
	return rlp.Encode(w, []interface{}{entry.Address, encodedPublicKey, entry.Version, entry.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the SignedAnnounceVersionEntry fields from a RLP stream.
func (entry *SignedAnnounceVersionEntry) DecodeRLP(s *rlp.Stream) error {
	var content struct {
		Address   common.Address
		PublicKey []byte
		Version   uint
		Signature []byte
	}

	if err := s.Decode(&content); err != nil {
		return err
	}
	decodedPublicKey, err := crypto.UnmarshalPubkey(content.PublicKey)
	if err != nil {
		return err
	}
	entry.Address, entry.PublicKey, entry.Version, entry.Signature = content.Address, decodedPublicKey, content.Version, content.Signature
	return nil
}

// String gives a string representation of SignedAnnounceVersionEntry
func (entry *SignedAnnounceVersionEntry) String() string {
	return fmt.Sprintf("{Address: %v, Version: %v, Signature: %v}", entry.Address, entry.Version, hex.EncodeToString(entry.Signature))
}

// OpenSignedAnnounceVersionDB opens a signed announce version database for storing
// SignedAnnounceVersions. If no path is given an in-memory, temporary database is constructed.
func OpenSignedAnnounceVersionDB(path string) (*SignedAnnounceVersionDB, error) {
	logger := log.New("db", "SignedAnnounceVersionDB")

	vedb, err := newVersionedEntryDB(int64(signedAnnounceVersionDBVersion), path, logger, &opt.WriteOptions{NoWriteMerge: true})
	if err != nil {
		logger.Error("Error creating db", "err", err)
		return nil, err
	}

	return &SignedAnnounceVersionDB{
		vedb:   vedb,
		logger: logger,
	}, nil
}

// Close flushes and closes the database files.
func (svdb *SignedAnnounceVersionDB) Close() error {
	return svdb.vedb.Close()
}

// String gives a string representation of the entire db
func (svdb *SignedAnnounceVersionDB) String() string {
	var b strings.Builder
	b.WriteString("SignedAnnounceVersionDB:")

	err := svdb.iterate(func(address common.Address, entry *SignedAnnounceVersionEntry) error {
		fmt.Fprintf(&b, " [%s => %s]", address.String(), entry.String())
		return nil
	})

	if err != nil {
		svdb.logger.Error("ValidatorEnodeDB.String error", "err", err)
	}

	return b.String()
}

// Upsert inserts any new entries or entries with a Version higher than the
// existing version. Returns any new or updated entries
func (svdb *SignedAnnounceVersionDB) Upsert(savEntries []*SignedAnnounceVersionEntry) ([]*SignedAnnounceVersionEntry, error) {
	logger := svdb.logger.New("func", "Upsert")

	var newEntries []*SignedAnnounceVersionEntry

	getExistingEntry := func(entry versionedEntry) (versionedEntry, error) {
		savEntry, err := signedAnnounceVersionEntryFromVersionedEntry(entry)
		if err != nil {
			return entry, err
		}
		return svdb.Get(savEntry.Address)
	}

	onNewEntry := func(batch *leveldb.Batch, entry versionedEntry) error {
		savEntry, err := signedAnnounceVersionEntryFromVersionedEntry(entry)
		if err != nil {
			return err
		}
		savEntryBytes, err := rlp.EncodeToBytes(savEntry)
		if err != nil {
			return err
		}
		batch.Put(addressKey(savEntry.Address), savEntryBytes)
		newEntries = append(newEntries, savEntry)
		logger.Trace("Updating with new entry",
			"address", savEntry.Address, "new version", savEntry.Version)
		return nil
	}

	onUpdatedEntry := func(batch *leveldb.Batch, _ versionedEntry, newEntry versionedEntry) error {
		return onNewEntry(batch, newEntry)
	}

	entries := make([]versionedEntry, len(savEntries))
	for i, sav := range savEntries {
		entries[i] = versionedEntry(sav)
	}

	if err := svdb.vedb.Upsert(entries, getExistingEntry, onUpdatedEntry, onNewEntry); err != nil {
		logger.Warn("Error upserting entries", "err", err)
		return nil, err
	}
	return newEntries, nil
}

// Get gets the SignedAnnounceVersionEntry entry with address `address`.
// Returns an error if no entry exists.
func (svdb *SignedAnnounceVersionDB) Get(address common.Address) (*SignedAnnounceVersionEntry, error) {
	var entry SignedAnnounceVersionEntry
	entryBytes, err := svdb.vedb.Get(addressKey(address))
	if err != nil {
		return nil, err
	}
	if err = rlp.DecodeBytes(entryBytes, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// GetVersion gets the version for the entry with address `address`
// Returns an error if no entry exists
func (svdb *SignedAnnounceVersionDB) GetVersion(address common.Address) (uint, error) {
	signedAnnVersion, err := svdb.Get(address)
	if err != nil {
		return 0, err
	}
	return signedAnnVersion.Version, nil
}

// GetAll gets each SignedAnnounceVersionEntry in the db
func (svdb *SignedAnnounceVersionDB) GetAll() ([]*SignedAnnounceVersionEntry, error) {
	var entries []*SignedAnnounceVersionEntry
	err := svdb.iterate(func(address common.Address, entry *SignedAnnounceVersionEntry) error {
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// Remove will remove an entry from the table
func (svdb *SignedAnnounceVersionDB) Remove(address common.Address) error {
	batch := new(leveldb.Batch)
	batch.Delete(addressKey(address))
	return svdb.vedb.Write(batch)
}

// Prune will remove entries for all addresses not present in addressesToKeep
func (svdb *SignedAnnounceVersionDB) Prune(addressesToKeep map[common.Address]bool) error {
	batch := new(leveldb.Batch)
	err := svdb.iterate(func(address common.Address, entry *SignedAnnounceVersionEntry) error {
		if !addressesToKeep[address] {
			svdb.logger.Trace("Deleting entry", "address", address)
			batch.Delete(addressKey(address))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return svdb.vedb.Write(batch)
}

// iterate will call `onEntry` for each entry in the db
func (svdb *SignedAnnounceVersionDB) iterate(onEntry func(common.Address, *SignedAnnounceVersionEntry) error) error {
	logger := svdb.logger.New("func", "iterate")
	// Only target address keys
	keyPrefix := []byte(dbAddressPrefix)

	onDBEntry := func(key []byte, value []byte) error {
		var entry SignedAnnounceVersionEntry
		if err := rlp.DecodeBytes(value, &entry); err != nil {
			return err
		}
		address := common.BytesToAddress(key)
		if err := onEntry(address, &entry); err != nil {
			return err
		}
		return nil
	}

	if err := svdb.vedb.Iterate(keyPrefix, onDBEntry); err != nil {
		logger.Warn("Error iterating through db entries", "err", err)
		return err
	}
	return nil
}

// SignedAnnounceVersionEntryInfo gives basic information for an entry in the DB
type SignedAnnounceVersionEntryInfo struct {
	Address string `json:"address"`
	Version uint   `json:"version"`
}

// Info gives a map SignedAnnounceVersionEntryInfo where each key is the address.
// Intended for RPC use
func (svdb *SignedAnnounceVersionDB) Info() (map[string]*SignedAnnounceVersionEntryInfo, error) {
	dbInfo := make(map[string]*SignedAnnounceVersionEntryInfo)
	err := svdb.iterate(func(address common.Address, entry *SignedAnnounceVersionEntry) error {
		dbInfo[address.Hex()] = &SignedAnnounceVersionEntryInfo{
			Address: entry.Address.Hex(),
			Version: entry.Version,
		}
		return nil
	})
	return dbInfo, err
}
