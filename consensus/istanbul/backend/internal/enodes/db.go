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
	"errors"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
	lvlerrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

var (
	errIncorrectEntryType = errors.New("Incorrect entry type")
)

const (
	dbVersionKey = "version" // Version of the database to flush if changes

	dbAddressPrefix = "address:" // Identifier to prefix node entries with
	dbNodeIDPrefix  = "nodeid:"  // Identifier to prefix node entries with
)

func addressKey(address common.Address) []byte {
	return append([]byte(dbAddressPrefix), address.Bytes()...)
}

func nodeIDKey(nodeID enode.ID) []byte {
	return append([]byte(dbNodeIDPrefix), nodeID.Bytes()...)
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
