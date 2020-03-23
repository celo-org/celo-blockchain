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
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
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

// AddressEntry is an entry for the valEnodeTable.
// This implements the versionedEntry interface.
type AddressEntry struct {
	Address common.Address
	Node    *enode.Node
	Version uint
}

func addressEntryFromVersionedEntry(entry versionedEntry) (*AddressEntry, error) {
	addressEntry, ok := entry.(*AddressEntry)
	if !ok {
		return nil, errIncorrectEntryType
	}
	return addressEntry, nil
}

// GetVersion gets the entry's version. This implements the GetVersion function
// for the versionedEntry interface
func (ae *AddressEntry) GetVersion() uint {
	return ae.Version
}

func (ae *AddressEntry) String() string {
	return fmt.Sprintf("{address: %v, enodeURL: %v, version: %v}", ae.Address.String(), ae.Node.String(), ae.Version)
}

// Implement RLP Encode/Decode interface
type rlpEntry struct {
	Address  common.Address
	EnodeURL string
	Version  uint
}

// EncodeRLP serializes AddressEntry into the Ethereum RLP format.
func (ae *AddressEntry) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, rlpEntry{ae.Address, ae.Node.String(), ae.Version})
}

// DecodeRLP implements rlp.Decoder, and load the AddressEntry fields from a RLP stream.
func (ae *AddressEntry) DecodeRLP(s *rlp.Stream) error {
	var entry rlpEntry
	if err := s.Decode(&entry); err != nil {
		return err
	}
	node, err := enode.ParseV4(entry.EnodeURL)
	if err != nil {
		return err
	}

	*ae = AddressEntry{Address: entry.Address, Node: node, Version: entry.Version}
	return nil
}

// ValidatorEnodeDB represents a Map that can be accessed either
// by address or enode
type ValidatorEnodeDB struct {
	vedb    *versionedEntryDB
	lock    sync.RWMutex
	handler ValidatorEnodeHandler
	logger  log.Logger
}

// OpenValidatorEnodeDB opens a validator enode database for storing and retrieving infos about validator
// enodes. If no path is given an in-memory, temporary database is constructed.
func OpenValidatorEnodeDB(path string, handler ValidatorEnodeHandler) (*ValidatorEnodeDB, error) {
	logger := log.New("db", "ValidatorEnodeDB")

	vedb, err := newVersionedEntryDB(int64(valEnodeDBVersion), path, logger, &opt.WriteOptions{NoWriteMerge: true})
	if err != nil {
		logger.Error("Error creating db", "err", err)
		return nil, err
	}

	return &ValidatorEnodeDB{
		vedb:    vedb,
		handler: handler,
		logger:  logger,
	}, nil
}

// Close flushes and closes the database files.
func (vet *ValidatorEnodeDB) Close() error {
	return vet.vedb.Close()
}

func (vet *ValidatorEnodeDB) String() string {
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

	entryBytes, err := vet.vedb.Get(nodeIDKey(nodeID))
	if err != nil {
		return common.ZeroAddress, err
	}
	return common.BytesToAddress(entryBytes), nil
}

// GetAllValEnodes will return all entries in the valEnodeDB
func (vet *ValidatorEnodeDB) GetAllValEnodes() (map[common.Address]*AddressEntry, error) {
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

// Upsert will update or insert a validator enode entry given that the existing entry
// is older (determined by the version) than the new one
// TODO - In addition to modifying the val_enode_db, this function also will disconnect
//        and/or connect the corresponding validator connenctions.  The validator connections
//        should be managed be a separate thread (see https://github.com/celo-org/celo-blockchain/issues/607)
func (vet *ValidatorEnodeDB) Upsert(valEnodeEntries []*AddressEntry) error {
	logger := vet.logger.New("func", "Upsert")
	vet.lock.Lock()
	defer vet.lock.Unlock()

	peersToRemove := make([]*enode.Node, 0, len(valEnodeEntries))
	peersToAdd := make(map[common.Address]*enode.Node)

	getExistingEntry := func(entry versionedEntry) (versionedEntry, error) {
		addressEntry, err := addressEntryFromVersionedEntry(entry)
		if err != nil {
			return entry, err
		}
		return vet.getAddressEntry(addressEntry.Address)
	}

	onNewEntry := func(batch *leveldb.Batch, entry versionedEntry) error {
		addressEntry, err := addressEntryFromVersionedEntry(entry)
		if err != nil {
			return err
		}
		entryBytes, err := rlp.EncodeToBytes(addressEntry)
		if err != nil {
			return err
		}
		batch.Put(nodeIDKey(addressEntry.Node.ID()), addressEntry.Address.Bytes())
		batch.Put(addressKey(addressEntry.Address), entryBytes)
		peersToAdd[addressEntry.Address] = addressEntry.Node
		return nil
	}

	onUpdatedEntry := func(batch *leveldb.Batch, existingEntry versionedEntry, newEntry versionedEntry) error {
		existingAddressEntry, err := addressEntryFromVersionedEntry(existingEntry)
		if err != nil {
			return err
		}
		batch.Delete(nodeIDKey(existingAddressEntry.Node.ID()))
		peersToRemove = append(peersToRemove, existingAddressEntry.Node)
		return onNewEntry(batch, newEntry)
	}

	entries := make([]versionedEntry, len(valEnodeEntries))
	for i, valEnodeEntry := range valEnodeEntries {
		entries[i] = versionedEntry(valEnodeEntry)
	}

	if err := vet.vedb.Upsert(entries, getExistingEntry, onUpdatedEntry, onNewEntry); err != nil {
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

// RemoveEntry will remove an entry from the table
func (vet *ValidatorEnodeDB) RemoveEntry(address common.Address) error {
	vet.lock.Lock()
	defer vet.lock.Unlock()
	batch := new(leveldb.Batch)
	err := vet.addDeleteToBatch(batch, address)
	if err != nil {
		return err
	}
	return vet.vedb.Write(batch)
}

// PruneEntries will remove entries for all address not present in addressesToKeep
func (vet *ValidatorEnodeDB) PruneEntries(addressesToKeep map[common.Address]bool) error {
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
	return vet.vedb.Write(batch)
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

func (vet *ValidatorEnodeDB) addDeleteToBatch(batch *leveldb.Batch, address common.Address) error {
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

func (vet *ValidatorEnodeDB) getAddressEntry(address common.Address) (*AddressEntry, error) {
	var entry AddressEntry
	entryBytes, err := vet.vedb.Get(addressKey(address))
	if err != nil {
		return nil, err
	}

	if err = rlp.DecodeBytes(entryBytes, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (vet *ValidatorEnodeDB) iterateOverAddressEntries(onEntry func(common.Address, *AddressEntry) error) error {
	logger := vet.logger.New("func", "iterateOverAddressEntries")
	// Only target address keys
	keyPrefix := []byte(dbAddressPrefix)

	onDBEntry := func(key []byte, value []byte) error {
		var entry AddressEntry
		if err := rlp.DecodeBytes(value, &entry); err != nil {
			return err
		}
		address := common.BytesToAddress(key)
		if err := onEntry(address, &entry); err != nil {
			return err
		}
		return nil
	}

	if err := vet.vedb.Iterate(keyPrefix, onDBEntry); err != nil {
		logger.Warn("Error iterating through db entries", "err", err)
		return err
	}
	return nil
}

// ValEnodeEntryInfo contains information for an entry of the val enode table
type ValEnodeEntryInfo struct {
	Enode   string `json:"enode"`
	Version uint   `json:"version"`
}

// ValEnodeTableInfo gives basic information for each entry of the table
func (vet *ValidatorEnodeDB) ValEnodeTableInfo() (map[string]*ValEnodeEntryInfo, error) {
	valEnodeTableInfo := make(map[string]*ValEnodeEntryInfo)

	valEnodeTable, err := vet.GetAllValEnodes()
	if err == nil {
		for address, valEnodeEntry := range valEnodeTable {
			valEnodeTableInfo[address.Hex()] = &ValEnodeEntryInfo{
				Enode:   valEnodeEntry.Node.String(),
				Version: valEnodeEntry.Version,
			}
		}
	}

	return valEnodeTableInfo, err
}
