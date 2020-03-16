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
	"fmt"
	"io"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	signedAnnounceVersionDBVersion = 0
)

// SignedAnnounceVersionDB stores
type SignedAnnounceVersionDB struct {
	db           *leveldb.DB //the actual DB
	logger       log.Logger
	writeOptions *opt.WriteOptions
}

// SignedAnnounceVersionEntry is an entry in the SignedAnnounceVersionDB.
// It's a signed message from a registered or active validator indicating
// the most recent version of its enode.
type SignedAnnounceVersionEntry struct {
	Address   common.Address
	Version   uint
	Signature []byte
}

// EncodeRLP serializes SignedAnnounceVersionEntry into the Ethereum RLP format.
func (entry *SignedAnnounceVersionEntry) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{entry.Address, entry.Version, entry.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the SignedAnnounceVersionEntry fields from a RLP stream.
func (entry *SignedAnnounceVersionEntry) DecodeRLP(s *rlp.Stream) error {
	var content struct {
		Address   common.Address
		Version   uint
		Signature []byte
	}

	if err := s.Decode(&content); err != nil {
		return err
	}
	entry.Address, entry.Version, entry.Signature = content.Address, content.Version, content.Signature
	return nil
}

// String gives a string representation of SignedAnnounceVersionEntry
func (entry *SignedAnnounceVersionEntry) String() string {
	return fmt.Sprintf("{Address: %v, Version: %v, Signature.length: %v}", entry.Address, entry.Version, len(entry.Signature))
}

// OpenSignedAnnounceVersionDB opens a signed announce version database for storing
// SignedAnnounceVersions. If no path is given an in-memory, temporary database is constructed.
func OpenSignedAnnounceVersionDB(path string) (*SignedAnnounceVersionDB, error) {
	var db *leveldb.DB
	var err error

	logger := log.New("db", "SignedAnnounceVersionDB")

	if path == "" {
		db, err = newMemoryDB()
	} else {
		db, err = newPersistentDB(int64(signedAnnounceVersionDBVersion), path, logger)
	}

	if err != nil {
		return nil, err
	}
	return &SignedAnnounceVersionDB{
		db:           db,
		logger:       logger,
		writeOptions: &opt.WriteOptions{NoWriteMerge: true},
	}, nil
}

// Close flushes and closes the database files.
func (svdb *SignedAnnounceVersionDB) Close() error {
	return svdb.db.Close()
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
func (svdb *SignedAnnounceVersionDB) Upsert(entries []*SignedAnnounceVersionEntry) ([]*SignedAnnounceVersionEntry, error) {
	logger := svdb.logger.New("func", "Upsert")
	batch := new(leveldb.Batch)

	var newEntries []*SignedAnnounceVersionEntry

	for _, entry := range entries {
		currentEntry, err := svdb.Get(entry.Address)
		isNew := err == leveldb.ErrNotFound
		if !isNew && err != nil {
			return nil, err
		}
		if !isNew && entry.Version <= currentEntry.Version {
			logger.Trace("Not inserting, version is not greater than the existing entry",
				"address", entry.Address, "existing version", currentEntry.Version,
				"new entry version", entry.Version)
			continue
		}
		entryBytes, err := rlp.EncodeToBytes(entry)
		if err != nil {
			return nil, err
		}
		batch.Put(addressKey(entry.Address), entryBytes)
		newEntries = append(newEntries, entry)
		logger.Trace("Updating with new entry", "isNew", isNew,
			"address", entry.Address, "new version", entry.Version)
	}

	if batch.Len() > 0 {
		err := svdb.db.Write(batch, svdb.writeOptions)
		if err != nil {
			return nil, err
		}
	}
	return newEntries, nil
}

// Get gets the SignedAnnounceVersionEntry entry with address `address`.
// Returns an error if no entry exists.
func (svdb *SignedAnnounceVersionDB) Get(address common.Address) (*SignedAnnounceVersionEntry, error) {
	var entry SignedAnnounceVersionEntry
	rawEntry, err := svdb.db.Get(addressKey(address), nil)
	if err != nil {
		return nil, err
	}
	if err = rlp.DecodeBytes(rawEntry, &entry); err != nil {
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
	return svdb.db.Write(batch, svdb.writeOptions)
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
	return svdb.db.Write(batch, svdb.writeOptions)
}

// iterate will call `onEntry` for each entry in the db
func (svdb *SignedAnnounceVersionDB) iterate(onEntry func(common.Address, *SignedAnnounceVersionEntry) error) error {
	iter := svdb.db.NewIterator(util.BytesPrefix([]byte(dbAddressPrefix)), nil)
	defer iter.Release()

	for iter.Next() {
		var entry SignedAnnounceVersionEntry
		address := common.BytesToAddress(iter.Key()[len(dbAddressPrefix):])
		rlp.DecodeBytes(iter.Value(), &entry)

		err := onEntry(address, &entry)
		if err != nil {
			return err
		}
	}
	return iter.Error()
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
