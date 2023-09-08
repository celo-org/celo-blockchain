package leveldb

import (
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// NewInMemory returns a wrapped LevelDB object with an in-memory storage.
func NewInMemory() (*Database, error) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		return nil, err
	}
	return &Database{
		db: db,
	}, nil
}

// NewRangeIterator creates an over a subset of database content starting at
// and ending at a particular key.
func (db *Database) NewRangeIterator(rang *util.Range) ethdb.Iterator {
	return db.db.NewIterator(rang, nil)
}
