package rawdb

import (
	"bytes"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/rlp"
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

// WriteRandomCommitmentCache will write a random beacon commitment's associated block parent hash
// (which is used to calculate the commitmented random number).
func WriteRandomCommitmentCache(db ethdb.KeyValueWriter, commitment common.Hash, parentHash common.Hash) {
	if err := db.Put(randomnessCommitmentKey(commitment), parentHash.Bytes()); err != nil {
		log.Crit("Failed to store randomness commitment cache entry", "err", err)
	}
}

// ReadRandomCommitmentCache will retun the random beacon commit's associated block parent hash.
func ReadRandomCommitmentCache(db ethdb.Reader, commitment common.Hash) common.Hash {
	parentHash, err := db.Get(randomnessCommitmentKey(commitment))
	if err != nil {
		log.Warn("Error in trying to retrieve randomness commitment cache entry", "error", err)
		return common.Hash{}
	}

	return common.BytesToHash(parentHash)
}

// randomnessCommitmentKey will return the key for where the
// given commitment's cached key-value entry
func randomnessCommitmentKey(commitment common.Hash) []byte {
	dbRandomnessPrefix := []byte("db-randomness-prefix")
	return append(dbRandomnessPrefix, commitment.Bytes()...)
}

// Extra hash comparison is necessary since ancient database only maintains
// the canonical data.
func headerHash(data []byte) common.Hash {
	header := new(types.Header)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Error decoding stored block header", "err", err)
	}

	return header.Hash()
}
