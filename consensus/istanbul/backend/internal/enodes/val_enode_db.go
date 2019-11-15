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
	"github.com/ethereum/go-ethereum/rlp"
)

// Keys in the node database.
const (
	dbVersionKey    = "version"  // Version of the database to flush if changes
	dbAddressPrefix = "address:" // Identifier to prefix node entries with
	dbEnodePrefix   = "enode:"
)

const (
	// dbNodeExpiration = 24 * time.Hour // Time after which an unseen node should be dropped.
	// dbCleanupCycle   = time.Hour      // Time period for running the expiration task.
	dbVersion = 1
)

var (
	// errOldAnnounceMessage is returned when the received announce message's block number is earlier
	// than a previous received message
	errOldAnnounceMessage = errors.New("old announce message")
)

// ValidatorEnodeHandler is handler to Add/Remove events. Events execute within write lock
type ValidatorEnodeHandler interface {
	// AddValidatorPeer adds a validator peer
	AddValidatorPeer(enodeURL string, address common.Address)

	// RemoveValidatorPeer removes a validator peer
	RemoveValidatorPeer(enodeURL string)

	// ReplaceValidatorPeers replace all validator peers for new list of enodeURLs
	ReplaceValidatorPeers(newEnodeURLs []string)

	// Clear all validator peers
	ClearValidatorPeers()
}

func addressKey(address common.Address) []byte {
	return append([]byte(dbAddressPrefix), address.Bytes()...)
}

func enodeURLKey(enodeURL string) []byte {
	return append([]byte(dbEnodePrefix), []byte(enodeURL)...)
}

// Entries for the valEnodeTable
type addressEntry struct {
	enodeURL string
	view     *istanbul.View
}

func (ve *addressEntry) String() string {
	return fmt.Sprintf("{enodeURL: %v, view: %v}", ve.enodeURL, ve.view)
}

// Implement RLP Encode/Decode interface
type rlpEntry struct {
	EnodeURL string
	View     *istanbul.View
}

// EncodeRLP serializes addressEntry into the Ethereum RLP format.
func (ve *addressEntry) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, rlpEntry{ve.enodeURL, ve.view})
}

// DecodeRLP implements rlp.Decoder, and load the addressEntry fields from a RLP stream.
func (ve *addressEntry) DecodeRLP(s *rlp.Stream) error {
	var entry rlpEntry
	if err := s.Decode(&entry); err != nil {
		return err
	}
	*ve = addressEntry{entry.EnodeURL, entry.View}
	return nil
}

// ValidatorEnodeDB represents a Map that can be accessed either
// by address or enode
type ValidatorEnodeDB struct {
	db      *leveldb.DB //the actual DB
	lock    sync.RWMutex
	handler ValidatorEnodeHandler
	logger  log.Logger
}

// OpenValidatorEnodeDB opens a validator enode database for storing and retrieving infos about validator
// enodes. If no path is given an in-memory, temporary database is constructed.
func OpenValidatorEnodeDB(path string, handler ValidatorEnodeHandler) (*ValidatorEnodeDB, error) {
	var db *leveldb.DB
	var err error
	if path == "" {
		db, err = newMemoryDB()
	} else {
		db, err = newPersistentDB(path)
	}

	if err != nil {
		return nil, err
	}
	return &ValidatorEnodeDB{
		db:      db,
		handler: handler,
		logger:  log.New(),
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

// Close flushes and closes the database files.
func (vet *ValidatorEnodeDB) Close() error {
	return vet.db.Close()
}

func (vet *ValidatorEnodeDB) String() string {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	var b strings.Builder
	b.WriteString("ValEnodeTable:")

	err := vet.iterateOverAddressEntries(func(address common.Address, entry *addressEntry) error {
		fmt.Fprintf(&b, " [%s => %s]", address.String(), entry.String())
		return nil
	})

	if err != nil {
		vet.logger.Error("ValidatorEnodeDB.String error", "err", err)
	}

	return b.String()
}

// GetEnodeURLFromAddress will return the enodeURL for an address if it's known
func (vet *ValidatorEnodeDB) GetEnodeURLFromAddress(address common.Address) (string, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	entry, err := vet.getAddressEntry(address)
	if err != nil {
		return "", err
	}
	return entry.enodeURL, nil
}

// GetAddressFromEnodeURL will return the address for an enodeURL if it's known
func (vet *ValidatorEnodeDB) GetAddressFromEnodeURL(enodeURL string) (common.Address, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	return vet.getAddressFromEnodeURL(enodeURL)
}

func (vet *ValidatorEnodeDB) getAddressFromEnodeURL(enodeURL string) (common.Address, error) {
	rawEntry, err := vet.db.Get(enodeURLKey(enodeURL), nil)
	if err != nil {
		return common.ZeroAddress, err
	}
	return common.BytesToAddress(rawEntry), nil
}

// Upsert will update or insert a validator enode entry; given that the existing entry
// is older (determined by view parameter) that the new one
func (vet *ValidatorEnodeDB) Upsert(remoteAddress common.Address, enodeURL string, view *istanbul.View) error {
	vet.lock.Lock()
	defer vet.lock.Unlock()

	currentEntry, err := vet.getAddressEntry(remoteAddress)
	isNew := err == leveldb.ErrNotFound

	// Check errors
	if !isNew && err != nil {
		return err
	}

	// If it is an old message, ignore it.
	if err == nil && view.Cmp(currentEntry.view) <= 0 {
		return errOldAnnounceMessage
	}

	// new entry
	rawEntry, err := rlp.EncodeToBytes(&addressEntry{enodeURL, view})
	if err != nil {
		return err
	}

	hasOldValueChanged := !isNew && currentEntry.enodeURL == enodeURL

	batch := new(leveldb.Batch)

	if hasOldValueChanged {
		batch.Delete(enodeURLKey(currentEntry.enodeURL))
		batch.Put(enodeURLKey(enodeURL), remoteAddress.Bytes())
	} else if isNew {
		batch.Put(enodeURLKey(enodeURL), remoteAddress.Bytes())
	}
	batch.Put(addressKey(remoteAddress), rawEntry)

	err = vet.db.Write(batch, nil)
	if err != nil {
		return err
	}
	vet.logger.Trace("Upsert an entry in the valEnodeTable", "address", remoteAddress, "enodeURL", enodeURL)

	if hasOldValueChanged {
		vet.handler.RemoveValidatorPeer(currentEntry.enodeURL)
	}
	vet.handler.AddValidatorPeer(enodeURL, remoteAddress)
	return nil
}

// RemoveEntry will remove an entry from the table
func (vet *ValidatorEnodeDB) RemoveEntry(address common.Address) error {
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
func (vet *ValidatorEnodeDB) PruneEntries(addressesToKeep map[common.Address]bool) error {
	vet.lock.Lock()
	defer vet.lock.Unlock()
	batch := new(leveldb.Batch)
	err := vet.iterateOverAddressEntries(func(address common.Address, entry *addressEntry) error {
		if !addressesToKeep[address] {
			vet.logger.Trace("Deleting entry from valEnodeTable", "address", address)
			fmt.Println("Deleting entry for", address.String())
			return vet.addDeleteToBatch(batch, address)
		}
		return nil
	})

	if err != nil {
		return err
	}
	return vet.db.Write(batch, nil)
}

func (vet *ValidatorEnodeDB) RefreshValPeers(valset istanbul.ValidatorSet, ourAddress common.Address) {
	// We use a R lock since we don't modify levelDB table
	vet.lock.RLock()
	defer vet.lock.RUnlock()

	if valset.ContainsByAddress(ourAddress) {
		// transform address to enodeURLs
		newEnodeURLs := []string{}
		for _, val := range valset.List() {
			entry, err := vet.getAddressEntry(val.Address())
			if err == nil {
				newEnodeURLs = append(newEnodeURLs, entry.enodeURL)
			} else if err != leveldb.ErrNotFound {
				vet.logger.Error("Error reading valEnodeTable: GetEnodeURLFromAddress", "err", err)
			}
		}

		vet.handler.ReplaceValidatorPeers(newEnodeURLs)
	} else {
		// Disconnect all validator peers if this node is not in the valset
		vet.handler.ClearValidatorPeers()
	}
}

func (vet *ValidatorEnodeDB) addDeleteToBatch(batch *leveldb.Batch, address common.Address) error {
	entry, err := vet.getAddressEntry(address)
	if err != nil {
		return err
	}

	batch.Delete(addressKey(address))
	batch.Delete(enodeURLKey(entry.enodeURL))
	vet.handler.RemoveValidatorPeer(entry.enodeURL)
	return nil
}

func (vet *ValidatorEnodeDB) getAddressEntry(address common.Address) (*addressEntry, error) {
	var entry addressEntry
	rawEntry, err := vet.db.Get(addressKey(address), nil)
	if err != nil {
		return nil, err
	}

	if err = rlp.DecodeBytes(rawEntry, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (vet *ValidatorEnodeDB) iterateOverAddressEntries(onEntry func(common.Address, *addressEntry) error) error {
	iter := vet.db.NewIterator(util.BytesPrefix([]byte(dbAddressPrefix)), nil)
	defer iter.Release()

	for iter.Next() {
		var entry addressEntry
		address := common.BytesToAddress(iter.Key()[len(dbAddressPrefix):])
		rlp.DecodeBytes(iter.Value(), &entry)

		err := onEntry(address, &entry)
		if err != nil {
			return err
		}
	}
	return iter.Error()
}
