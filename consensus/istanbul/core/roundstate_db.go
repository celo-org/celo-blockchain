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

package core

import (
	"bytes"
	"encoding/binary"
	"os"

	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
	lvlerrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

const (
	dbVersion    = 1
	dbVersionKey = "version"  // Version of the database to flush if changes
	lastViewKey  = "lastView" // Last View that we know of
)

type RoundStateDB interface {
	GetLastView() (*istanbul.View, error)
	GetRoundStateFor(view *istanbul.View) (RoundState, error)
	UpdateLastRoundState(rs RoundState) error
	Close() error
}

type roundStateDBImpl struct {
	db *leveldb.DB
}

func newRoundStateDB(path string) (RoundStateDB, error) {
	logger := log.New("func", "newRoundStateDB")

	var db *leveldb.DB
	var err error
	if path == "" {
		logger.Info("Open roundstate db", "path", "inmemory")
		db, err = newMemoryDB()
	} else {
		logger.Info("Open roundstate db", "path", path)
		db, err = newPersistentDB(path)
	}

	if err != nil {
		logger.Error("Failed to open roundstate db", "err", err)
		return nil, err
	}

	return &roundStateDBImpl{
		db: db,
	}, nil
}

// newMemoryDB creates a new in-memory node database without a persistent backend.
func newMemoryDB() (*leveldb.DB, error) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// newPersistentNodeDB creates/opens a leveldb backed persistent node database,
// also flushing its contents in case of a version mismatch.
func newPersistentDB(path string) (*leveldb.DB, error) {
	opts := &opt.Options{OpenFilesCacheCapacity: 5}
	db, err := leveldb.OpenFile(path, opts)
	if _, iscorrupted := err.(*lvlerrors.ErrCorrupted); iscorrupted {
		db, err = leveldb.RecoverFile(path, nil)
	}
	if err != nil {
		return nil, err
	}
	// The nodes contained in the cache correspond to a certain protocol version.
	// Flush all nodes if the version doesn't match.
	currentVer := make([]byte, binary.MaxVarintLen64)
	currentVer = currentVer[:binary.PutVarint(currentVer, int64(dbVersion))]

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
			db.Close()
			if err = os.RemoveAll(path); err != nil {
				return nil, err
			}
			return newPersistentDB(path)
		}
	}
	return db, nil
}

// storeRoundState will store the currentRoundState in a Map<view, roundState> schema.
func (rsdb *roundStateDBImpl) UpdateLastRoundState(rs RoundState) error {
	// We store the roundState for each view; since we'll need this
	// information to allow the node to have evidence to show that
	// a validator did a "valid" double signing

	viewKey, err := rlp.EncodeToBytes(rs.View())
	if err != nil {
		return err
	}

	entryBytes, err := rlp.EncodeToBytes(rs)
	if err != nil {
		return err
	}

	batch := new(leveldb.Batch)
	batch.Put([]byte(lastViewKey), viewKey)
	batch.Put(viewKey, entryBytes)

	return rsdb.db.Write(batch, nil)
}

func (rsdb *roundStateDBImpl) GetLastView() (*istanbul.View, error) {
	rawEntry, err := rsdb.db.Get([]byte(lastViewKey), nil)
	if err != nil {
		return nil, err
	}

	var entry istanbul.View
	if err = rlp.DecodeBytes(rawEntry, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (rsdb *roundStateDBImpl) GetRoundStateFor(view *istanbul.View) (RoundState, error) {
	viewKey, err := rlp.EncodeToBytes(view)
	if err != nil {
		return nil, err
	}

	rawEntry, err := rsdb.db.Get(viewKey, nil)
	if err != nil {
		return nil, err
	}

	var entry roundStateImpl
	if err = rlp.DecodeBytes(rawEntry, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (rsdb *roundStateDBImpl) Close() error {
	return rsdb.db.Close()
}
