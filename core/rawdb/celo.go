package rawdb

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/ethdb"
)

var genesisSupplyKey = []byte("genesis-supply-genesis")

// ReadGenesisCeloSupply retrieves a CELO token supply at genesis
func ReadGenesisCeloSupply(db ethdb.KeyValueReader) *big.Int {
	data, err := db.Get(genesisSupplyKey)
	if err != nil {
		return nil
	}
	genesisSupply := new(big.Int)
	genesisSupply.SetBytes(data)

	return genesisSupply
}

// WriteGenesisCeloSupply writes the CELO token supply at genesis
func WriteGenesisCeloSupply(db ethdb.KeyValueWriter, genesisSupply *big.Int) error {
	return db.Put(genesisSupplyKey, genesisSupply.Bytes())
}
