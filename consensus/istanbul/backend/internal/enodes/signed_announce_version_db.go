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
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

// SignedAnnounceVersionDB stores
type SignedAnnounceVersionDB struct {
	db           *leveldb.DB //the actual DB
	logger       log.Logger
	writeOptions *opt.WriteOptions
}

// SignedAnnounceVersion is an entry in the SignedAnnounceVersionDB.
// It's a signed message from a registered or active validator indicating
// the most recent version of its enode.
type SignedAnnounceVersion struct {
	Address   common.Address
	Version   uint
	Signature []byte
}

// EncodeRLP serializes SignedAnnounceVersion into the Ethereum RLP format.
func (sav *SignedAnnounceVersion) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{sav.Address, sav.Version, sav.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the SignedAnnounceVersion fields from a RLP stream.
func (sav *SignedAnnounceVersion) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Address   common.Address
		Version   uint
		Signature []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	sav.Address, sav.Version, sav.Signature = msg.Address, msg.Version, msg.Signature
	return nil
}

// String gives a string representation of SignedAnnounceVersion
func (sav *SignedAnnounceVersion) String() string {
	return fmt.Sprintf("{Address: %v, Version: %v, Signature.length: %v}", sav.Address, sav.Version, len(sav.Signature))
}

// ValidateSignature will return an error if a SignedAnnounceVersion's signature
// is invalid.
func (sav *SignedAnnounceVersion) ValidateSignature() error {
	signedAnnounceVersionNoSig := &SignedAnnounceVersion{
		Address: sav.Address,
		Version: sav.Version,
	}
	bytesNoSignature, err := rlp.EncodeToBytes(signedAnnounceVersionNoSig)
	if err != nil {
		return err
	}
	address, err := istanbul.GetSignatureAddress(bytesNoSignature, sav.Signature)
	if err != nil {
		return err
	}
	if address != sav.Address {
		return errors.New("Signature does not match address")
	}
	return nil
}

// OpenSignedAnnounceVersionDB opens a signed announce version database for storing
// SignedAnnounceVersions. If no path is given an in-memory, temporary database is constructed.
func OpenSignedAnnounceVersionDB(path string) (*SignedAnnounceVersionDB, error) {
	var db *leveldb.DB
	var err error

	logger := log.New("db", "SignedAnnounceVersionDB")
	dbVersion := 0

	if path == "" {
		db, err = newMemoryDB()
	} else {
		db, err = newPersistentDB(int64(dbVersion), path, logger)
	}

	if err != nil {
		return nil, err
	}
	return &SignedAnnounceVersionDB{
		db:      db,
		logger:  logger,
		writeOptions: &opt.WriteOptions{NoWriteMerge: true},
	}, nil
}

// Close flushes and closes the database files.
func (svdb *SignedAnnounceVersionDB) Close() error {
	return svdb.db.Close()
}

// String gives a string representation of the entire db
func (svdb *SignedAnnounceVersionDB) String() string {
	var b strings.Builder
	b.WriteString("SignedAnnounceVersionDB:")

	err := svdb.iterate(func(address common.Address, entry *SignedAnnounceVersion) error {
		fmt.Fprintf(&b, " [%s => %s]", address.String(), entry.String())
		return nil
	})

	if err != nil {
		svdb.logger.Error("ValidatorEnodeDB.String error", "err", err)
	}

	return b.String()
}

// Upsert inserts any new entries or entries with a Version higher than the
// existing version. Returns any new or updated entries
func (svdb *SignedAnnounceVersionDB) Upsert(signedAnnounceVersions []*SignedAnnounceVersion) ([]*SignedAnnounceVersion, error) {
    logger := svdb.logger.New("func", "Upsert")
	batch := new(leveldb.Batch)

	var newEntries []*SignedAnnounceVersion

    for _, signedAnnVersion := range signedAnnounceVersions {
        currentEntry, err := svdb.Get(signedAnnVersion.Address)
        isNew := err == leveldb.ErrNotFound
		if !isNew && err != nil {
			return nil, err
		}
        if !isNew && signedAnnVersion.Version <= currentEntry.Version {
            logger.Trace("Not inserting, version is not greater than the existing entry",
                "address", signedAnnVersion.Address, "existing version", currentEntry.Version,
                "new entry version", signedAnnVersion.Version)
            continue
        }
        entryBytes, err := rlp.EncodeToBytes(signedAnnVersion)
        if err != nil {
            return nil, err
        }
        batch.Put(addressKey(signedAnnVersion.Address), entryBytes)
		newEntries = append(newEntries, signedAnnVersion)
        logger.Trace("Updating with new entry", "isNew", isNew,
            "address", signedAnnVersion.Address, "new version", signedAnnVersion.Version)
    }

    if batch.Len() > 0 {
        err := svdb.db.Write(batch, svdb.writeOptions)
        if err != nil {
            return nil, err
        }
    }
    return newEntries, nil
}

// Get gets the SignedAnnounceVersion entry with address `address`.
// Returns an error if no entry exists.
func (svdb *SignedAnnounceVersionDB) Get(address common.Address) (*SignedAnnounceVersion, error) {
	var entry SignedAnnounceVersion
	rawEntry, err := svdb.db.Get(addressKey(address), nil)
	if err != nil {
		return nil, err
	}

	if err = rlp.DecodeBytes(rawEntry, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// GetAll gets all SignedAnnounceVersions in the db
func (svdb *SignedAnnounceVersionDB) GetAll() ([]*SignedAnnounceVersion, error) {
	var signedAnnounceVersions []*SignedAnnounceVersion
	err := svdb.iterate(func(address common.Address, entry *SignedAnnounceVersion) error {
		signedAnnounceVersions = append(signedAnnounceVersions, entry)
		return nil
	})
	return signedAnnounceVersions, err
}

// Remove will remove an entry from the table
func (svdb *SignedAnnounceVersionDB) Remove(address common.Address) error {
	batch := new(leveldb.Batch)
	batch.Delete(addressKey(address))
	return svdb.db.Write(batch, svdb.writeOptions)
}

// Prune will remove entries for all addresses not present in addressesToKeep
func (svdb *SignedAnnounceVersionDB) Prune(addressesToKeep map[common.Address]bool) error {
	batch := new(leveldb.Batch)
	err := svdb.iterate(func(address common.Address, entry *SignedAnnounceVersion) error {
		if !addressesToKeep[address] {
			svdb.logger.Trace("Deleting entry", "address", address)
			batch.Delete(addressKey(address))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return svdb.db.Write(batch, svdb.writeOptions)
}

// iterate will call `onEntry` for each entry in the db
func (svdb *SignedAnnounceVersionDB) iterate(onEntry func(common.Address, *SignedAnnounceVersion) error) error {
	iter := svdb.db.NewIterator(util.BytesPrefix([]byte(dbAddressPrefix)), nil)
	defer iter.Release()

	for iter.Next() {
		var entry SignedAnnounceVersion
		address := common.BytesToAddress(iter.Key()[len(dbAddressPrefix):])
		rlp.DecodeBytes(iter.Value(), &entry)

		err := onEntry(address, &entry)
		if err != nil {
			return err
		}
	}
	return iter.Error()
}

// SignedAnnounceVersionEntryInfo gives basic information for an entry in the DB
type SignedAnnounceVersionEntryInfo struct {
	Address string `json:"address"`
	Version uint   `json:"version"`
}

// Info gives a map SignedAnnounceVersionEntryInfo where each key is the address.
// Intended for RPC use
func (svdb *SignedAnnounceVersionDB) Info() (map[string]*SignedAnnounceVersionEntryInfo, error) {
	dbInfo := make(map[string]*SignedAnnounceVersionEntryInfo)
	err := svdb.iterate(func(address common.Address, entry *SignedAnnounceVersion) error {
		dbInfo[address.Hex()] = &SignedAnnounceVersionEntryInfo{
			Address: entry.Address.Hex(),
			Version: entry.Version,
		}
		return nil
	})
	return dbInfo, err
}
