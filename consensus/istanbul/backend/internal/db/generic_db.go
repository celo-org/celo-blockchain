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

package db

import (
	"bytes"
	"encoding/binary"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
	lvlerrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/ethereum/go-ethereum/log"
)

const (
	dbVersionKey = "version" // Version of the database to flush if changes
)

// GenericDB manages a levelDB database
type GenericDB struct {
	db           *leveldb.DB
	writeOptions *opt.WriteOptions
}

type GenericEntry interface{}

// New will open a new db at the given file path with the given version.
// If the path is empty, the db will be created in memory.
// If there is a version mismatch in the existing db, the contents are flushed.
func New(dbVersion int64, path string, logger log.Logger, writeOptions *opt.WriteOptions) (*GenericDB, error) {
	db, err := NewDB(dbVersion, path, logger)
	if err != nil {
		return nil, err
	}
	return &GenericDB{
		db:           db,
		writeOptions: writeOptions,
	}, nil
}

// Close flushes and closes the database files.
func (gdb *GenericDB) Close() error {
	return gdb.db.Close()
}

// Upsert iterates through each provided entry and determines if the entry is
// new. If there is an existing entry in the db, `onUpdatedEntry` is called.
// If there is no existing entry, `onNewEntry` is called. Db content modifications are left to those functions
// by providing a leveldb Batch that is written after all entries are processed.
func (gdb *GenericDB) Upsert(
	entries []GenericEntry,
	getExistingEntry func(entry GenericEntry) (GenericEntry, error),
	onUpdatedEntry func(batch *leveldb.Batch, existingEntry GenericEntry, newEntry GenericEntry) error,
	onNewEntry func(batch *leveldb.Batch, entry GenericEntry) error,
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
		err := gdb.Write(batch)
		if err != nil {
			return err
		}
	}
	return nil
}

// Get gets the bytes at a given key in the db
func (gdb *GenericDB) Get(key []byte) ([]byte, error) {
	return gdb.db.Get(key, nil)
}

// Write writes a Batch to modify the db
func (gdb *GenericDB) Write(batch *leveldb.Batch) error {
	return gdb.db.Write(batch, gdb.writeOptions)
}

// Iterate will iterate through each entry in the db whose key has the prefix
// keyPrefix, and call `onEntry` with the bytes of the key (without the prefix)
// and the bytes of the value
func (gdb *GenericDB) Iterate(keyPrefix []byte, onEntry func([]byte, []byte) error) error {
	iter := gdb.db.NewIterator(util.BytesPrefix(keyPrefix), nil)
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

// newDB creates/opens a leveldb persistent database at the given path.
// If no path is given, an in-memory, temporary database is constructed.
func NewDB(dbVersion int64, path string, logger log.Logger) (*leveldb.DB, error) {
	if path == "" {
		return NewMemoryDB()
	}
	return NewPersistentDB(dbVersion, path, logger)
}

// newMemoryDB creates a new in-memory node database without a persistent backend.
func NewMemoryDB() (*leveldb.DB, error) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// newPersistentNodeDB creates/opens a leveldb backed persistent database,
// also flushing its contents in case of a version mismatch.
func NewPersistentDB(dbVersion int64, path string, logger log.Logger) (*leveldb.DB, error) {
	opts := &opt.Options{OpenFilesCacheCapacity: 5}
	db, err := leveldb.OpenFile(path, opts)
	if _, iscorrupted := err.(*lvlerrors.ErrCorrupted); iscorrupted {
		db, err = leveldb.RecoverFile(path, nil)
	}
	if err != nil {
		return nil, err
	}
	currentVer := make([]byte, binary.MaxVarintLen64)
	currentVer = currentVer[:binary.PutVarint(currentVer, dbVersion)]

	blob, err := db.Get([]byte(dbVersionKey), nil)
	switch err {
	case leveldb.ErrNotFound:
		// Version not found (i.e. empty cache), insert it
		if err := db.Put([]byte(dbVersionKey), currentVer, nil); err != nil {
			db.Close()
			return nil, err
		}

	case nil:
		// Version present, flush if different
		if !bytes.Equal(blob, currentVer) {
			oldVersion, _ := binary.Varint(blob)
			newVersion, _ := binary.Varint(currentVer)
			logger.Info("DB version has changed. Creating a new leveldb.", "old version", oldVersion, "new version", newVersion)
			db.Close()
			if err = os.RemoveAll(path); err != nil {
				return nil, err
			}
			return NewPersistentDB(dbVersion, path, logger)
		}
	}
	return db, nil
}
