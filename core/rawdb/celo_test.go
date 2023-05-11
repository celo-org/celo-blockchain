package rawdb

import (
	"math/big"
	"testing"
)

// Tests Genesis CELO supply storage and retrieval operations.
func TestGenesisCeloSupply(t *testing.T) {
	db := NewMemoryDatabase()

	if supply := ReadGenesisCeloSupply(db); supply != nil {
		t.Fatalf("Non existent CELO token supply returned: %v", supply)
	}

	initialSupply := big.NewInt(10000)
	// Write and verify the supply in the database
	if err := WriteGenesisCeloSupply(db, initialSupply); err != nil {
		t.Fatalf("Failed to store CELO token supply: err %v", err)
	}

	if supply := ReadGenesisCeloSupply(db); supply == nil {
		t.Fatalf("Stored CELO token supply not found")
	} else if initialSupply.Cmp(supply) != 0 {
		t.Fatalf("Retrieved CELO token supply mismatch: have %v, want %v", supply, initialSupply)
	}
}
