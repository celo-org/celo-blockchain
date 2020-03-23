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
	"bytes"
	"encoding/binary"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
	lvlerrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

const (
	dbVersionKey = "version" // Version of the database to flush if changes

	dbAddressPrefix = "address:" // Identifier to prefix node entries with
)

func addressKey(address common.Address) []byte {
	return append([]byte(dbAddressPrefix), address.Bytes()...)
}

// newDB creates/opens a leveldb persistent database at the given path.
// If no path is given, an in-memory, temporary database is constructed.
func newDB(dbVersion int64, path string, logger log.Logger) (*leveldb.DB, error) {
	if path == "" {
		return newMemoryDB()
	}
	return newPersistentDB(int64(valEnodeDBVersion), path, logger)
}

// newMemoryDB creates a new in-memory node database without a persistent backend.
func newMemoryDB() (*leveldb.DB, error) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// newPersistentNodeDB creates/opens a leveldb backed persistent database,
// also flushing its contents in case of a version mismatch.
func newPersistentDB(dbVersion int64, path string, logger log.Logger) (*leveldb.DB, error) {
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
			return newPersistentDB(dbVersion, path, logger)
		}
	}
	return db, nil
}

type versionedEntryDB struct {
	db           *leveldb.DB
	writeOptions *opt.WriteOptions
}

type versionedEntry interface {
	GetVersion() uint
}

func newVersionedEntryDB(dbVersion int64, path string, logger log.Logger, writeOptions *opt.WriteOptions) (*versionedEntryDB, error) {
	db, err := newDB(dbVersion, path, logger)
	if err != nil {
		return nil, err
	}
	return &versionedEntryDB{
		db: db,
		writeOptions: writeOptions,
	}, nil
}

// Close flushes and closes the database files.
func (vedb *versionedEntryDB) Close() error {
	return vedb.db.Close()
}

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


func (vedb *versionedEntryDB) Get(key []byte) ([]byte, error) {
	return vedb.db.Get(key, nil)
}

// Remove will remove an entry from the table
func (vedb *versionedEntryDB) Remove(key []byte) error {
	batch := new(leveldb.Batch)
	batch.Delete(key)
	return vedb.Write(batch)
}

func (vedb *versionedEntryDB) Write(batch *leveldb.Batch) error {
	return vedb.db.Write(batch, vedb.writeOptions)
}

func (vedb *versionedEntryDB) iterate(keyPrefix []byte, onEntry func([]byte, []byte) error) error {
	iter := vedb.db.NewIterator(util.BytesPrefix([]byte(keyPrefix)), nil)
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
