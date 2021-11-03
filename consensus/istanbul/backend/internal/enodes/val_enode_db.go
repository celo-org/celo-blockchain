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
	"strings"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/db"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/rlp"
)

// Keys in the node database.
const (
	valEnodeDBVersion = 4
)

// ValidatorEnodeHandler is handler to Add/Remove events. Events execute within write lock
type ValidatorEnodeHandler interface {
	// AddValidatorPeer adds a validator peer
	AddValidatorPeer(node *enode.Node, address common.Address)

	// RemoveValidatorPeer removes a validator peer
	RemoveValidatorPeer(node *enode.Node)

	// ReplaceValidatorPeers replace all validator peers for new list of enodeURLs
	ReplaceValidatorPeers(newNodes []*enode.Node)

	// Clear all validator peers
	ClearValidatorPeers()
}

func addressEntryFromGenericEntry(entry db.GenericEntry) (*istanbul.AddressEntry, error) {
	addressEntry, ok := entry.(*istanbul.AddressEntry)
	if !ok {
		return nil, errIncorrectEntryType
	}
	return addressEntry, nil
}

// ValidatorEnodeDB represents a Map that can be accessed either
// by address or enode
type ValidatorEnodeDB struct {
	gdb     *db.GenericDB
	lock    sync.RWMutex
	handler ValidatorEnodeHandler
	logger  log.Logger
}

// OpenValidatorEnodeDB opens a validator enode database for storing and retrieving infos about validator
// enodes. If no path is given an in-memory, temporary database is constructed.
func OpenValidatorEnodeDB(path string, handler ValidatorEnodeHandler) (*ValidatorEnodeDB, error) {
	logger := log.New("db", "ValidatorEnodeDB")

	gdb, err := db.New(int64(valEnodeDBVersion), path, logger, &opt.WriteOptions{NoWriteMerge: true})
	if err != nil {
		logger.Error("Error creating db", "err", err)
		return nil, err
	}

	return &ValidatorEnodeDB{
		gdb:     gdb,
		handler: handler,
		logger:  logger,
	}, nil
}

// Close flushes and closes the database files.
func (vet *ValidatorEnodeDB) Close() error {
	return vet.gdb.Close()
}

func (vet *ValidatorEnodeDB) String() string {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	var b strings.Builder
	b.WriteString("ValEnodeTable:")

	err := vet.iterateOverAddressEntries(func(address common.Address, entry *istanbul.AddressEntry) error {
		fmt.Fprintf(&b, " [%s => %s]", address.String(), entry.String())
		return nil
	})

	if err != nil {
		vet.logger.Error("ValidatorEnodeDB.String error", "err", err)
	}

	return b.String()
}

// GetNodeFromAddress will return the enodeURL for an address if it's known
func (vet *ValidatorEnodeDB) GetNodeFromAddress(address common.Address) (*enode.Node, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	entry, err := vet.getAddressEntry(address)
	if err != nil {
		return nil, err
	}
	return entry.Node, nil
}

// GetVersionFromAddress will return the version for an address if it's known
func (vet *ValidatorEnodeDB) GetVersionFromAddress(address common.Address) (uint, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	entry, err := vet.getAddressEntry(address)
	if err != nil {
		return 0, err
	}
	return entry.Version, nil
}

// GetAddressFromNodeID will return the address for an nodeID if it's known
func (vet *ValidatorEnodeDB) GetAddressFromNodeID(nodeID enode.ID) (common.Address, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()

	entryBytes, err := vet.gdb.Get(nodeIDKey(nodeID))
	if err != nil {
		return common.ZeroAddress, err
	}
	return common.BytesToAddress(entryBytes), nil
}

// GetHighestKnownVersionFromAddress will return the highest known version for an address if it's known
func (vet *ValidatorEnodeDB) GetHighestKnownVersionFromAddress(address common.Address) (uint, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()

	entry, err := vet.getAddressEntry(address)
	if err != nil {
		return 0, err
	}
	return entry.HighestKnownVersion, nil
}

// GetValEnodes will return entries in the valEnodeDB filtered on the valAddresses parameter.
// If it's set to nil, then no filter will be applied.
func (vet *ValidatorEnodeDB) GetValEnodes(valAddresses []common.Address) (map[common.Address]*istanbul.AddressEntry, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()
	var entries = make(map[common.Address]*istanbul.AddressEntry)
	var valAddressesMap map[common.Address]struct{}

	if valAddresses != nil {
		valAddressesMap = make(map[common.Address]struct{})
		for _, address := range valAddresses {
			valAddressesMap[address] = struct{}{}
		}
	}

	err := vet.iterateOverAddressEntries(func(address common.Address, entry *istanbul.AddressEntry) error {
		if valAddressesMap != nil {
			if _, ok := valAddressesMap[address]; ok {
				entries[address] = entry
			}
		} else {
			entries[address] = entry
		}

		return nil
	})

	if err != nil {
		vet.logger.Error("ValidatorEnodeDB.GetValEnodes error", "err", err)
		return nil, err
	}

	return entries, nil
}

// UpsertHighestKnownVersion function will do the following
// 1. Check if the updated HighestKnownVersion is higher than the existing HighestKnownVersion
// 2. Update the fields HighestKnownVersion, NumQueryAttempsForHKVersion, and PublicKey
func (vet *ValidatorEnodeDB) UpsertHighestKnownVersion(valEnodeEntries []*istanbul.AddressEntry) error {
	logger := vet.logger.New("func", "UpsertHighestKnownVersion")

	onNewEntry := func(batch *leveldb.Batch, entry db.GenericEntry) error {
		addressEntry, err := addressEntryFromGenericEntry(entry)
		if err != nil {
			return err
		}
		entryBytes, err := rlp.EncodeToBytes(addressEntry)
		if err != nil {
			return err
		}
		if addressEntry.Node != nil {
			batch.Put(nodeIDKey(addressEntry.Node.ID()), addressEntry.Address.Bytes())
		}
		batch.Put(addressKey(addressEntry.Address), entryBytes)
		return nil
	}

	onUpdatedEntry := func(batch *leveldb.Batch, existingEntry db.GenericEntry, newEntry db.GenericEntry) error {
		existingAddressEntry, err := addressEntryFromGenericEntry(existingEntry)
		if err != nil {
			return err
		}
		newAddressEntry, err := addressEntryFromGenericEntry(newEntry)
		if err != nil {
			return err
		}

		if newAddressEntry.HighestKnownVersion < existingAddressEntry.HighestKnownVersion {
			logger.Trace("Skipping entry whose HighestKnownVersion is less than the existing entry's", "existing HighestKnownVersion", existingAddressEntry.HighestKnownVersion, "new version", newAddressEntry.HighestKnownVersion)
			return nil
		}

		// "Backfill" all other fields
		newAddressEntry.Node = existingAddressEntry.Node
		newAddressEntry.Version = existingAddressEntry.Version
		newAddressEntry.LastQueryTimestamp = existingAddressEntry.LastQueryTimestamp

		// Set NumQueryAttemptsForHKVersion to 0
		newAddressEntry.NumQueryAttemptsForHKVersion = 0

		return onNewEntry(batch, newAddressEntry)
	}

	if err := vet.upsert(valEnodeEntries, onNewEntry, onUpdatedEntry); err != nil {
		logger.Warn("Error upserting entries", "err", err)
		return err
	}

	return nil
}

// UpsertVersionAndEnode will do the following
// 1. Check if the updated Version higher than the existing Version
// 2. Update Node, Version, HighestKnownVersion (if it's less than the new Version)
// 3. If the Node has been updated, establish new validator peer
func (vet *ValidatorEnodeDB) UpsertVersionAndEnode(valEnodeEntries []*istanbul.AddressEntry) error {
	logger := vet.logger.New("func", "UpsertVersionAndEnode")

	peersToRemove := make([]*enode.Node, 0, len(valEnodeEntries))
	peersToAdd := make(map[common.Address]*enode.Node)

	onNewEntry := func(batch *leveldb.Batch, entry db.GenericEntry) error {
		addressEntry, err := addressEntryFromGenericEntry(entry)
		if err != nil {
			return err
		}
		entryBytes, err := rlp.EncodeToBytes(addressEntry)
		if err != nil {
			return err
		}
		if addressEntry.Node != nil {
			batch.Put(nodeIDKey(addressEntry.Node.ID()), addressEntry.Address.Bytes())
			peersToAdd[addressEntry.Address] = addressEntry.Node
		}
		batch.Put(addressKey(addressEntry.Address), entryBytes)
		return nil
	}

	onUpdatedEntry := func(batch *leveldb.Batch, existingEntry db.GenericEntry, newEntry db.GenericEntry) error {
		existingAddressEntry, err := addressEntryFromGenericEntry(existingEntry)
		if err != nil {
			return err
		}
		newAddressEntry, err := addressEntryFromGenericEntry(newEntry)
		if err != nil {
			return err
		}

		if newAddressEntry.Version < existingAddressEntry.Version {
			logger.Trace("Skipping entry whose Version is less than the existing entry's", "existing Version", existingAddressEntry.Version, "new version", newAddressEntry.Version)
			return nil
		}

		// "Backfill" all other fields
		newAddressEntry.PublicKey = existingAddressEntry.PublicKey
		newAddressEntry.LastQueryTimestamp = existingAddressEntry.LastQueryTimestamp

		// Update HighestKnownVersion, if needed
		if newAddressEntry.Version > existingAddressEntry.HighestKnownVersion {
			newAddressEntry.HighestKnownVersion = newAddressEntry.Version
			newAddressEntry.NumQueryAttemptsForHKVersion = 0
		} else {
			newAddressEntry.HighestKnownVersion = existingAddressEntry.HighestKnownVersion
		}

		enodeChanged := existingAddressEntry.Node != nil && newAddressEntry.Node != nil && existingAddressEntry.Node.String() != newAddressEntry.Node.String()
		if enodeChanged {
			batch.Delete(nodeIDKey(existingAddressEntry.Node.ID()))
			peersToRemove = append(peersToRemove, existingAddressEntry.Node)
		}

		return onNewEntry(batch, newAddressEntry)
	}

	if err := vet.upsert(valEnodeEntries, onNewEntry, onUpdatedEntry); err != nil {
		logger.Warn("Error upserting entries", "err", err)
		return err
	}

	for _, node := range peersToRemove {
		vet.handler.RemoveValidatorPeer(node)
	}

	for address, node := range peersToAdd {
		vet.handler.AddValidatorPeer(node, address)
	}

	return nil
}

// UpdateQueryEnodeStats function will do the following
// 1. Increment each entry's NumQueryAttemptsForHKVersion by 1 is existing HighestKnownVersion is the same
// 2. Set each entry's LastQueryTimestamp to the current time
func (vet *ValidatorEnodeDB) UpdateQueryEnodeStats(valEnodeEntries []*istanbul.AddressEntry) error {
	logger := vet.logger.New("func", "UpdateEnodeQueryStats")

	onNewEntry := func(batch *leveldb.Batch, entry db.GenericEntry) error {
		addressEntry, err := addressEntryFromGenericEntry(entry)
		if err != nil {
			return err
		}
		entryBytes, err := rlp.EncodeToBytes(addressEntry)
		if err != nil {
			return err
		}
		if addressEntry.Node != nil {
			batch.Put(nodeIDKey(addressEntry.Node.ID()), addressEntry.Address.Bytes())
		}
		batch.Put(addressKey(addressEntry.Address), entryBytes)
		return nil
	}

	onUpdatedEntry := func(batch *leveldb.Batch, existingEntry db.GenericEntry, newEntry db.GenericEntry) error {
		existingAddressEntry, err := addressEntryFromGenericEntry(existingEntry)
		if err != nil {
			return err
		}
		newAddressEntry, err := addressEntryFromGenericEntry(newEntry)
		if err != nil {
			return err
		}

		if existingAddressEntry.HighestKnownVersion == newAddressEntry.HighestKnownVersion {
			newAddressEntry.NumQueryAttemptsForHKVersion = existingAddressEntry.NumQueryAttemptsForHKVersion + 1
		}

		currentTime := time.Now()
		newAddressEntry.LastQueryTimestamp = &currentTime

		// "Backfill" all other fields
		newAddressEntry.PublicKey = existingAddressEntry.PublicKey
		newAddressEntry.Node = existingAddressEntry.Node
		newAddressEntry.Version = existingAddressEntry.Version
		newAddressEntry.HighestKnownVersion = existingAddressEntry.HighestKnownVersion

		return onNewEntry(batch, newAddressEntry)
	}

	if err := vet.upsert(valEnodeEntries, onNewEntry, onUpdatedEntry); err != nil {
		logger.Warn("Error upserting entries", "err", err)
		return err
	}

	return nil
}

// upsert will update or insert a validator enode entry given that the existing entry
// is older (determined by the version) than the new one
// TODO - In addition to modifying the val_enode_db, this function also will disconnect
//        and/or connect the corresponding validator connenctions.  The validator connections
//        should be managed be a separate thread (see https://github.com/celo-org/celo-blockchain/issues/607)
func (vet *ValidatorEnodeDB) upsert(valEnodeEntries []*istanbul.AddressEntry,
	onNewEntry func(batch *leveldb.Batch, entry db.GenericEntry) error,
	onUpdatedEntry func(batch *leveldb.Batch, existingEntry db.GenericEntry, newEntry db.GenericEntry) error) error {
	logger := vet.logger.New("func", "Upsert")
	vet.lock.Lock()
	defer vet.lock.Unlock()

	getExistingEntry := func(entry db.GenericEntry) (db.GenericEntry, error) {
		addressEntry, err := addressEntryFromGenericEntry(entry)
		if err != nil {
			return entry, err
		}
		return vet.getAddressEntry(addressEntry.Address)
	}

	entries := make([]db.GenericEntry, len(valEnodeEntries))
	for i, valEnodeEntry := range valEnodeEntries {
		entries[i] = db.GenericEntry(valEnodeEntry)
	}

	if err := vet.gdb.Upsert(entries, getExistingEntry, onUpdatedEntry, onNewEntry); err != nil {
		logger.Warn("Error upserting entries", "err", err)
		return err
	}

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
	return vet.gdb.Write(batch)
}

// PruneEntries will remove entries for all address not present in addressesToKeep
func (vet *ValidatorEnodeDB) PruneEntries(addressesToKeep map[common.Address]bool) error {
	vet.lock.Lock()
	defer vet.lock.Unlock()
	batch := new(leveldb.Batch)
	err := vet.iterateOverAddressEntries(func(address common.Address, entry *istanbul.AddressEntry) error {
		if !addressesToKeep[address] {
			vet.logger.Trace("Deleting entry from valEnodeTable", "address", address)
			return vet.addDeleteToBatch(batch, address)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return vet.gdb.Write(batch)
}

func (vet *ValidatorEnodeDB) RefreshValPeers(valConnSet map[common.Address]bool, ourAddress common.Address) {
	// We use a R lock since we don't modify levelDB table
	vet.lock.RLock()
	defer vet.lock.RUnlock()

	if valConnSet[ourAddress] {
		// transform address to enodeURLs
		newNodes := []*enode.Node{}
		for val := range valConnSet {
			entry, err := vet.getAddressEntry(val)
			if entry != nil && entry.Node != nil {
				if err == nil {
					newNodes = append(newNodes, entry.Node)
				} else if err != leveldb.ErrNotFound {
					vet.logger.Error("Error reading valEnodeTable: GetEnodeURLFromAddress", "err", err)
				}
			}
		}

		vet.handler.ReplaceValidatorPeers(newNodes)
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
	if entry.Node != nil {
		batch.Delete(nodeIDKey(entry.Node.ID()))
		if vet.handler != nil {
			vet.handler.RemoveValidatorPeer(entry.Node)
		}
	}
	return nil
}

func (vet *ValidatorEnodeDB) getAddressEntry(address common.Address) (*istanbul.AddressEntry, error) {
	var entry istanbul.AddressEntry
	entryBytes, err := vet.gdb.Get(addressKey(address))
	if err != nil {
		return nil, err
	}

	if err = rlp.DecodeBytes(entryBytes, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (vet *ValidatorEnodeDB) iterateOverAddressEntries(onEntry func(common.Address, *istanbul.AddressEntry) error) error {
	logger := vet.logger.New("func", "iterateOverAddressEntries")
	// Only target address keys
	keyPrefix := []byte(dbAddressPrefix)

	onDBEntry := func(key []byte, value []byte) error {
		var entry istanbul.AddressEntry
		if err := rlp.DecodeBytes(value, &entry); err != nil {
			return err
		}
		address := common.BytesToAddress(key)
		if err := onEntry(address, &entry); err != nil {
			return err
		}
		return nil
	}

	if err := vet.gdb.Iterate(keyPrefix, onDBEntry); err != nil {
		logger.Warn("Error iterating through db entries", "err", err)
		return err
	}
	return nil
}

// ValEnodeEntryInfo contains information for an entry of the val enode table
type ValEnodeEntryInfo struct {
	PublicKey                    string `json:"publicKey"`
	Enode                        string `json:"enode"`
	Version                      uint   `json:"version"`
	HighestKnownVersion          uint   `json:"highestKnownVersion"`
	NumQueryAttemptsForHKVersion uint   `json:"numQueryAttemptsForHKVersion"`
	LastQueryTimestamp           string `json:"lastQueryTimestamp"` // Unix timestamp
}

// ValEnodeTableInfo gives basic information for each entry of the table
func (vet *ValidatorEnodeDB) ValEnodeTableInfo() (map[string]*ValEnodeEntryInfo, error) {
	vet.lock.RLock()
	defer vet.lock.RUnlock()

	valEnodeTableInfo := make(map[string]*ValEnodeEntryInfo)

	valEnodeTable, err := vet.GetValEnodes(nil)
	if err == nil {
		for address, valEnodeEntry := range valEnodeTable {
			entryInfo := &ValEnodeEntryInfo{
				Version:                      valEnodeEntry.Version,
				HighestKnownVersion:          valEnodeEntry.HighestKnownVersion,
				NumQueryAttemptsForHKVersion: valEnodeEntry.NumQueryAttemptsForHKVersion,
			}
			if valEnodeEntry.PublicKey != nil {
				publicKeyBytes := crypto.CompressPubkey(valEnodeEntry.PublicKey)
				entryInfo.PublicKey = hexutil.Encode(publicKeyBytes)
			}
			if valEnodeEntry.Node != nil {
				entryInfo.Enode = valEnodeEntry.Node.String()
			}
			if valEnodeEntry.LastQueryTimestamp != nil {
				entryInfo.LastQueryTimestamp = valEnodeEntry.LastQueryTimestamp.String()
			}

			valEnodeTableInfo[address.Hex()] = entryInfo
		}
	}

	return valEnodeTableInfo, err
}
