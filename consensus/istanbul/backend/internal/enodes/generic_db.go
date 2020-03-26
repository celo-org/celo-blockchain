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

// genericDB manages a levelDB database
type genericDB struct {
	db           *leveldb.DB
	writeOptions *opt.WriteOptions
}

type genericEntry interface{}

// newGenericDB will open a new db at the given file path with the given version.
// If the path is empty, the db will be created in memory.
// If there is a version mismatch in the existing db, the contents are flushed.
func newGenericDB(dbVersion int64, path string, logger log.Logger, writeOptions *opt.WriteOptions) (*genericDB, error) {
	db, err := newDB(dbVersion, path, logger)
	if err != nil {
		return nil, err
	}
	return &genericDB{
		db:           db,
		writeOptions: writeOptions,
	}, nil
}

// Close flushes and closes the database files.
func (vedb *genericDB) Close() error {
	return vedb.db.Close()
}

// Upsert iterates through each provided entry and determines if the entry is
// new. If there is an existing entry in the db, `onUpdatedEntry` is called.
// If there is no existing entry, `onNewEntry` is called. Db content modifications are left to those functions
// by providing a leveldb Batch that is written after all entries are processed.
func (vedb *genericDB) Upsert(
	entries []genericEntry,
	getExistingEntry func(entry genericEntry) (genericEntry, error),
	onUpdatedEntry func(batch *leveldb.Batch, existingEntry genericEntry, newEntry genericEntry) error,
	onNewEntry func(batch *leveldb.Batch, entry genericEntry) error,
) error {
	batch := new(leveldb.Batch)
	for _, entry := range entries {
		existingEntry, err := getExistingEntry(entry)
		isNew := err == leveldb.ErrNotFound
		if !isNew && err != nil {
			return err
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
func (vedb *genericDB) Get(key []byte) ([]byte, error) {
	return vedb.db.Get(key, nil)
}

// Write writes a Batch to modify the db
func (vedb *genericDB) Write(batch *leveldb.Batch) error {
	return vedb.db.Write(batch, vedb.writeOptions)
}

// Iterate will iterate through each entry in the db whose key has the prefix
// keyPrefix, and call `onEntry` with the bytes of the key (without the prefix)
// and the bytes of the value
func (vedb *genericDB) Iterate(keyPrefix []byte, onEntry func([]byte, []byte) error) error {
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
