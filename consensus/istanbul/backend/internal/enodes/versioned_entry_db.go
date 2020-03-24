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
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/ethereum/go-ethereum/log"
)

// versionedEntryDB manages a levelDB database whose entries are versioned.
type versionedEntryDB struct {
	db           *leveldb.DB
	writeOptions *opt.WriteOptions
}

type versionedEntry interface {
	GetVersion() uint
}

// newVersionedEntryDB will open a new db at the given file path with the given version.
// If the path is empty, the db will be created in memory.
// If there is a version mismatch in the existing db, the contents are flushed.
func newVersionedEntryDB(dbVersion int64, path string, logger log.Logger, writeOptions *opt.WriteOptions) (*versionedEntryDB, error) {
	db, err := newDB(dbVersion, path, logger)
	if err != nil {
		return nil, err
	}
	return &versionedEntryDB{
		db:           db,
		writeOptions: writeOptions,
	}, nil
}

// Close flushes and closes the database files.
func (vedb *versionedEntryDB) Close() error {
	return vedb.db.Close()
}

// Upsert iterates through each provided entry and determines if the entry is
// new based off its version. If there is an existing entry in the db and the version of the
// new entry is newer, `onUpdatedEntry` is called. If there is no existing entry,
// `onNewEntry` is called. Db content modifications are left to those functions
// by providing a leveldb Batch that is written after all entries are processed.
func (vedb *versionedEntryDB) Upsert(
	entries []versionedEntry,
	getExistingEntry func(entry versionedEntry) (versionedEntry, error),
	onUpdatedEntry func(batch *leveldb.Batch, existingEntry versionedEntry, newEntry versionedEntry) error,
	onNewEntry func(batch *leveldb.Batch, entry versionedEntry) error,
) error {
	batch := new(leveldb.Batch)
	for _, entry := range entries {
		existingEntry, err := getExistingEntry(entry)
		isNew := err == leveldb.ErrNotFound
		if !isNew && err != nil {
			return err
		}
		if !isNew && entry.GetVersion() <= existingEntry.GetVersion() {
			continue
		}
		if isNew {
			if err := onNewEntry(batch, entry); err != nil {
				return err
			}
		} else {
			if err := onUpdatedEntry(batch, existingEntry, entry); err != nil {
				return err
			}
		}
	}

	if batch.Len() > 0 {
		err := vedb.Write(batch)
		if err != nil {
			return err
		}
	}
	return nil
}

// Get gets the bytes at a given key in the db
func (vedb *versionedEntryDB) Get(key []byte) ([]byte, error) {
	return vedb.db.Get(key, nil)
}

// Write writes a Batch to modify the db
func (vedb *versionedEntryDB) Write(batch *leveldb.Batch) error {
	return vedb.db.Write(batch, vedb.writeOptions)
}

// Iterate will iterate through each entry in the db whose key has the prefix
// keyPrefix, and call `onEntry` with the bytes of the key (without the prefix)
// and the bytes of the value
func (vedb *versionedEntryDB) Iterate(keyPrefix []byte, onEntry func([]byte, []byte) error) error {
	iter := vedb.db.NewIterator(util.BytesPrefix(keyPrefix), nil)
	defer iter.Release()

	for iter.Next() {
		key := iter.Key()[len(keyPrefix):]
		err := onEntry(key, iter.Value())
		if err != nil {
			return err
		}
	}
	return iter.Error()
}
