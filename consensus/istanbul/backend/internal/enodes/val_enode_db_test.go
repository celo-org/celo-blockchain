package enodes

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
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

type mockListener struct{}

func (ml *mockListener) AddValidatorPeer(enodeURL string, address common.Address) {}
func (ml *mockListener) RemoveValidatorPeer(enodeURL string)                      {}
func (ml *mockListener) ReplaceValidatorPeers(newEnodeURLs []string)              {}
func (ml *mockListener) ClearValidatorPeers()                                     {}

func TestSimpleCase(t *testing.T) {
	vet, err := OpenValidatorEnodeDB("", &mockListener{})
	if err != nil {
		t.Fatal("Failed to open DB")
	}

	err = vet.Upsert(addressA, "http://XXXX", view(0, 1))
	if err != nil {
		t.Fatal("Failed to upsert")
	}

	addr, err := vet.GetAddressFromEnodeURL("http://XXXX")
	if err != nil {
		t.Errorf("got %v", err)
	}
	if addr != addressA {
		t.Error("Invalid address saved")
	}

	enodeURL, err := vet.GetEnodeURLFromAddress(addressA)
	if err != nil {
		t.Errorf("got %v", err)
	}
	if enodeURL != "http://XXXX" {
		t.Error("Invalid enode saved")
	}
}

func TestUpsertOldValue(t *testing.T) {
	vet, err := OpenValidatorEnodeDB("", &mockListener{})
	if err != nil {
		t.Fatal("Failed to open DB")
	}

	err = vet.Upsert(addressA, "http://XXXX", view(0, 2))
	if err != nil {
		t.Fatal("Failed to upsert")
	}

	// trying to insert an old value
	err = vet.Upsert(addressA, "http://YYYY", view(0, 1))
	if err == nil {
		t.Fatal("Upsert should have failed")
	}
}

func TestDeleteEntry(t *testing.T) {
	vet, err := OpenValidatorEnodeDB("", &mockListener{})
	if err != nil {
		t.Fatal("Failed to open DB")
	}

	err = vet.Upsert(addressA, "http://XXXX", view(0, 2))
	if err != nil {
		t.Fatal("Failed to upsert")
	}

	err = vet.RemoveEntry(addressA)
	if err != nil {
		t.Fatal("Failed to delete")
	}

	if _, err := vet.GetEnodeURLFromAddress(addressA); err != nil {
		if err != leveldb.ErrNotFound {
			t.Fatalf("Can't get, different error: %v", err)
		}
	} else {
		t.Fatalf("Delete didn't work")
	}

}

func TestPruneEntries(t *testing.T) {
	vet, err := OpenValidatorEnodeDB("", &mockListener{})
	if err != nil {
		t.Fatal("Failed to open DB")
	}

	vet.Upsert(addressA, "http://XXXX", view(0, 2))
	vet.Upsert(addressB, "http://YYYY", view(0, 2))

	addressesToKeep := make(map[common.Address]bool)
	addressesToKeep[addressB] = true

	vet.PruneEntries(addressesToKeep)

	_, err = vet.GetEnodeURLFromAddress(addressB)
	if err != nil {
		t.Errorf("It should have found %s after prune", addressB.Hex())
	}
	_, err = vet.GetEnodeURLFromAddress(addressA)
	if err == nil {
		t.Errorf("It should have NOT found %s after prune", addressA.Hex())
	}

}

func TestRLPEntries(t *testing.T) {
	original := addressEntry{enodeURL: "HolaMundo", view: view(0, 1)}

	rawEntry, err := rlp.EncodeToBytes(&original)
	if err != nil {
		t.Errorf("Error %v", err)
	}

	var result addressEntry
	if err = rlp.DecodeBytes(rawEntry, &result); err != nil {
		t.Errorf("Error %v", err)
	}

	if result.enodeURL != original.enodeURL {
		t.Errorf("enodeURL doesn't match: got: %s expected: %s", result.enodeURL, original.enodeURL)
	}
	if result.view.Cmp(original.view) != 0 {
		t.Errorf("view doesn't match: got: %v expected: %v", result.view, original.view)
	}
}

func TestTableToString(t *testing.T) {
	vet, err := OpenValidatorEnodeDB("", &mockListener{})
	if err != nil {
		t.Fatal("Failed to open DB")
	}
	vet.Upsert(addressA, "http://XXXX", view(0, 2))
	vet.Upsert(addressB, "http://YYYY", view(0, 2))

	expected := "ValEnodeTable: [0x00Ce0d46d924CC8437c806721496599FC3FFA268 => {enodeURL: http://XXXX, view: {Round: 2, Sequence: 0}}] [0xfFFFff46D924CCfffFc806721496599fC3FFffff => {enodeURL: http://YYYY, view: {Round: 2, Sequence: 0}}]"

	if vet.String() != expected {
		t.Errorf("String() error: got: %s", vet.String())
	}
}
