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
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	lvlerrors "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// // Keys in the node database.
// const (
// 	dbVersionKey    = "version"  // Version of the database to flush if changes
// 	dbAddressPrefix = "address:" // Identifier to prefix address keys with
// )

const (
	// dbNodeExpiration = 24 * time.Hour // Time after which an unseen node should be dropped.
	// dbCleanupCycle   = time.Hour      // Time period for running the expiration task.
	dbVersionSignedAnnounceVersion = 0
)

// func addressKey(address common.Address) []byte {
// 	return append([]byte(dbAddressPrefix), address.Bytes()...)
// }

// ValidatorEnodeDB represents a Map that can be accessed either
// by address or enode
type SignedAnnounceVersionDB struct {
	db      *leveldb.DB //the actual DB
	logger  log.Logger
}

// OpenSignedAnnounceVersionDB opens a signed announce version database for storing
// signedAnnounceVersions. If no path is given an in-memory, temporary database is constructed.
func OpenSignedAnnounceVersionDB(path string) (*SignedAnnounceVersionDB, error) {
	var db *leveldb.DB
	var err error

	logger := log.New("db", "SignedAnnounceVersionDB")

	if path == "" {
		db, err = newMemoryDB()
	} else {
		db, err = newPersistentDB(path, logger)
	}

	if err != nil {
		return nil, err
	}
	return &SignedAnnounceVersionDB{
		db:      db,
		logger:  logger,
	}, nil
}

// Close flushes and closes the database files.
func (svdb *SignedAnnounceVersionDB) Close() error {
	return vet.db.Close()
}

func (svdb *SignedAnnounceVersionDB) String() string {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	var b strings.Builder
	b.WriteString("ValEnodeTable:")

	err := vet.iterateOverAddressEntries(func(address common.Address, entry *AddressEntry) error {
		fmt.Fprintf(&b, " [%s => %s]", address.String(), entry.String())
		return nil
	})

	if err != nil {
		vet.logger.Error("ValidatorEnodeDB.String error", "err", err)
	}

	return b.String()
}

// GetEnodeURLFromAddress will return the enodeURL for an address if it's known
func (svdb *SignedAnnounceVersionDB) GetNodeFromAddress(address common.Address) (*enode.Node, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	entry, err := vet.getAddressEntry(address)
	if err != nil {
		return nil, err
	}
	return entry.Node, nil
}

// GetVersionFromAddress will return the version for an address if it's known
func (svdb *SignedAnnounceVersionDB) GetVersionFromAddress(address common.Address) (uint, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	entry, err := vet.getAddressEntry(address)
	if err != nil {
		return 0, err
	}
	return entry.Version, nil
}

// GetAddressFromNodeID will return the address for an nodeID if it's known
func (svdb *SignedAnnounceVersionDB) GetAddressFromNodeID(nodeID enode.ID) (common.Address, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()

	rawEntry, err := vet.db.Get(nodeIDKey(nodeID), nil)
	if err != nil {
		return common.ZeroAddress, err
	}
	return common.BytesToAddress(rawEntry), nil
}

// GetAllValEnodes will return all entries in the valEnodeDB
func (svdb *SignedAnnounceVersionDB) GetAllValEnodes() (map[common.Address]*AddressEntry, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	var entries = make(map[common.Address]*AddressEntry)

	err := vet.iterateOverAddressEntries(func(address common.Address, entry *AddressEntry) error {
		entries[address] = entry
		return nil
	})

	if err != nil {
		vet.logger.Error("ValidatorEnodeDB.GetAllAddressEntries error", "err", err)
		return nil, err
	}

	return entries, nil
}

// Upsert TODO give comment
func (svdb *SignedAnnounceVersionDB) Upsert(signedAnnounceVersions []*signedAnnounceVersion) error {
    logger := svdb.logger.New("func", "Upsert")
	batch := new(leveldb.Batch)

    for _, signedAnnVersion := range signedAnnounceVersions {
        currentEntry, err := svdb.getAddressEntry(signedAnnVersion.Address)
        isNew := err == leveldb.ErrNotFound
		if !isNew && err != nil {
			return err
		}
        if !isNew && signedAnnVersion.Version <= currentEntry.Version {
            logger.Trace("Not inserting, version is not greater than the existing entry",
                "address", signedAnnVersion.Address, "existing version", currentEntry.Version,
                "new entry version", signedAnnVersion.Version)
            continue
        }
        newEntry, err := rlp.EncodeToBytes(signedAnnVersion)
        if err != nil {
            return err
        }
        batch.Put(addressKey(signedAnnVersion.Address), newEntry)
        logger.Trace("Updating with new entry", "isNew", isNew,
            "address", signedAnnVersion.Address, "new version", signedAnnVersion.Version)
    }

    if batch.Len() > 0 {
        err := svdb.db.Write(batch, nil)
        if err != nil {
            return err
        }
    }
    return nil
}

// RemoveEntry will remove an entry from the table
func (svdb *SignedAnnounceVersionDB) RemoveEntry(address common.Address) error {
	vet.lock.Lock()
	defer vet.lock.Unlock()
	batch := new(leveldb.Batch)
	err := vet.addDeleteToBatch(batch, address)
	if err != nil {
		return err
	}
	return vet.db.Write(batch, nil)
}

// PruneEntries will remove entries for all address not present in addressesToKeep
func (svdb *SignedAnnounceVersionDB) PruneEntries(addressesToKeep map[common.Address]bool) error {
	vet.lock.Lock()
	defer vet.lock.Unlock()
	batch := new(leveldb.Batch)
	err := vet.iterateOverAddressEntries(func(address common.Address, entry *AddressEntry) error {
		if !addressesToKeep[address] {
			vet.logger.Trace("Deleting entry from valEnodeTable", "address", address)
			return vet.addDeleteToBatch(batch, address)
		}
		return nil
	})

	if err != nil {
		return err
	}
	return vet.db.Write(batch, nil)
}

func (svdb *SignedAnnounceVersionDB) RefreshValPeers(valset istanbul.ValidatorSet, ourAddress common.Address) {
	// We use a R lock since we don't modify levelDB table
	vet.lock.RLock()
	defer vet.lock.RUnlock()

	if valset.ContainsByAddress(ourAddress) {
		// transform address to enodeURLs
		newNodes := []*enode.Node{}
		for _, val := range valset.List() {
			entry, err := vet.getAddressEntry(val.Address())
			if err == nil {
				newNodes = append(newNodes, entry.Node)
			} else if err != leveldb.ErrNotFound {
				vet.logger.Error("Error reading valEnodeTable: GetEnodeURLFromAddress", "err", err)
			}
		}

		vet.handler.ReplaceValidatorPeers(newNodes)
	} else {
		// Disconnect all validator peers if this node is not in the valset
		vet.handler.ClearValidatorPeers()
	}
}

func (svdb *SignedAnnounceVersionDB) addDeleteToBatch(batch *leveldb.Batch, address common.Address) error {
	entry, err := vet.getAddressEntry(address)
	if err != nil {
		return err
	}

	batch.Delete(addressKey(address))
	batch.Delete(nodeIDKey(entry.Node.ID()))
	if vet.handler != nil {
		vet.handler.RemoveValidatorPeer(entry.Node)
	}
	return nil
}

func (svdb *SignedAnnounceVersionDB) getEntry(address common.Address) (*signedAnnounceVersion, error) {
	var entry signedAnnounceVersion
	rawEntry, err := vet.db.Get(addressKey(address), nil)
	if err != nil {
		return nil, err
	}

	if err = rlp.DecodeBytes(rawEntry, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (svdb *SignedAnnounceVersionDB) iterateOverAddressEntries(onEntry func(common.Address, *signedAnnounceVersion) error) error {
	iter := vet.db.NewIterator(util.BytesPrefix([]byte(dbAddressPrefix)), nil)
	defer iter.Release()

	for iter.Next() {
		var entry signedAnnounceVersion
		address := common.BytesToAddress(iter.Key()[len(dbAddressPrefix):])
		rlp.DecodeBytes(iter.Value(), &entry)

		err := onEntry(address, &entry)
		if err != nil {
			return err
		}
	}
	return iter.Error()
}

type ValEnodeEntryInfo struct {
	Enode   string `json:"enode"`
	Version uint   `json:"version"`
}

func (svdb *SignedAnnounceVersionDB) ValEnodeTableInfo() (map[string]*ValEnodeEntryInfo, error) {
	valEnodeTableInfo := make(map[string]*ValEnodeEntryInfo)

	valEnodeTable, err := vet.GetAllValEnodes()
	if err == nil {
		for address, valEnodeEntry := range valEnodeTable {
			valEnodeTableInfo[address.Hex()] = &ValEnodeEntryInfo{Enode: valEnodeEntry.Node.String(),
				Version: valEnodeEntry.Version}
		}
	}

	return valEnodeTableInfo, err
}
