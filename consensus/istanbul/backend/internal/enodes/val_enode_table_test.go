package enodes

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

var (
	addressA = common.HexToAddress("0x00Ce0d46d924CC8437c806721496599FC3FFA268")
	addressB = common.HexToAddress("0xFFFFFF46d924CCFFFFc806721496599FC3FFFFFF")
)

func view(sequence, round int64) *istanbul.View {
	return &istanbul.View{
		Sequence: big.NewInt(sequence),
		Round:    big.NewInt(round),
	}
}

func TestSimpleCase(t *testing.T) {
	vet := NewValidatorEnodeTable()

	_, err := vet.Upsert(addressA, "http://XXXX", view(0, 1))
	if err != nil {
		t.Fatal("Failed to upsert")
	}

	if addr, ok := vet.GetAddressFromEnodeURL("http://XXXX"); ok {
		if addr != addressA {
			t.Error("Invalid address saved")
		}
	} else {
		t.Error("Address not found")
	}

	if enodeURL, ok := vet.GetEnodeURLFromAddress(addressA); ok {
		if enodeURL != "http://XXXX" {
			t.Error("Invalid enode saved")
		}
	} else {
		t.Error("enode not found")
	}
}

func TestUpsertOldValue(t *testing.T) {
	vet := NewValidatorEnodeTable()

	_, err := vet.Upsert(addressA, "http://XXXX", view(0, 2))
	if err != nil {
		t.Fatal("Failed to upsert")
	}

	// trying to insert an old value
	_, err = vet.Upsert(addressA, "http://YYYY", view(0, 1))
	if err == nil {
		t.Fatal("Upsert should have failed")
	}
}

func TestPruneEntries(t *testing.T) {
	vet := NewValidatorEnodeTable()

	vet.Upsert(addressA, "http://XXXX", view(0, 2))
	vet.Upsert(addressB, "http://YYYY", view(0, 2))

	addressesToKeep := make(map[common.Address]bool)
	addressesToKeep[addressB] = true

	vet.PruneEntries(addressesToKeep)

	_, ok := vet.GetEnodeURLFromAddress(addressB)
	if !ok {
		t.Errorf("It should have found %s after prune", addressB.Hex())
	}
	_, ok = vet.GetEnodeURLFromAddress(addressA)
	if ok {
		t.Errorf("It should have NOT found %s after prune", addressA.Hex())
	}

}
