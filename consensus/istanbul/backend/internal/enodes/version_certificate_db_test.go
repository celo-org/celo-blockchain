package enodes

import (
	"bytes"
	"crypto/ecdsa"
	"reflect"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
)

var keyA *ecdsa.PrivateKey
var keyB *ecdsa.PrivateKey

func init() {
	var err error
	keyA, err = crypto.GenerateKey()
	if err != nil {
		panic(err.Error())
	}
	keyB, err = crypto.GenerateKey()
	if err != nil {
		panic(err.Error())
	}
}

func signA(data []byte) ([]byte, error) {
	return crypto.Sign(crypto.Keccak256(data), keyA)
}

func signB(data []byte) ([]byte, error) {
	return crypto.Sign(crypto.Keccak256(data), keyB)
}

func TestVersionCertificateDBUpsert(t *testing.T) {
	table, err := OpenVersionCertificateDB("")
	if err != nil {
		t.Fatal("Failed to open DB")
	}
	entryA, err := istanbul.NewVersionCertificate(1, signA)
	require.NoError(t, err)
	entriesToUpsert := []*istanbul.VersionCertificate{entryA}
	newEntries, err := table.Upsert(entriesToUpsert)
	if err != nil {
		t.Fatal("Failed to upsert entry")
	}
	if !reflect.DeepEqual(newEntries, entriesToUpsert) {
		t.Errorf("Upsert did not return the expected new entries %v != %v", newEntries, entriesToUpsert)
	}

	entry, err := table.Get(entryA.Address())
	if err != nil {
		t.Errorf("got %v", err)
	}
	if !versionCertificateEntriesEqual(entry, entryA) {
		t.Error("The upserted entry is not deep equal to the original")
	}

	entryAOld, err := istanbul.NewVersionCertificate(0, signA)
	require.NoError(t, err)
	entriesToUpsert = []*istanbul.VersionCertificate{entryAOld}
	newEntries, err = table.Upsert(entriesToUpsert)
	if err != nil {
		t.Fatal("Failed to upsert old entry")
	}
	if len(newEntries) != 0 {
		t.Errorf("Expected no new entries to be returned by Upsert with old version, got %v", newEntries)
	}

	entry, err = table.Get(entryA.Address())
	if err != nil {
		t.Errorf("got %v", err)
	}
	if !versionCertificateEntriesEqual(entry, entryA) {
		t.Error("Upserting an old version gave a new entry")
	}

	entryANew, err := istanbul.NewVersionCertificate(2, signA)
	require.NoError(t, err)
	entriesToUpsert = []*istanbul.VersionCertificate{entryANew}
	newEntries, err = table.Upsert(entriesToUpsert)
	if err != nil {
		t.Fatal("Failed to upsert old entry")
	}
	if !reflect.DeepEqual(newEntries, entriesToUpsert) {
		t.Errorf("Expected new entries to be returned by Upsert with new version, got %v", newEntries)
	}

	entry, err = table.Get(entryA.Address())
	if err != nil {
		t.Errorf("got %v", err)
	}
	if !reflect.DeepEqual(entry, entryANew) {
		t.Error("Upserting a new version did not give a new entry")
	}
}

func TestVersionCertificateDBRemove(t *testing.T) {
	table, err := OpenVersionCertificateDB("")
	if err != nil {
		t.Fatal("Failed to open DB")
	}

	entryA, err := istanbul.NewVersionCertificate(1, signA)
	require.NoError(t, err)
	entriesToUpsert := []*istanbul.VersionCertificate{entryA}
	_, err = table.Upsert(entriesToUpsert)
	if err != nil {
		t.Fatal("Failed to upsert entry")
	}

	err = table.Remove(entryA.Address())
	if err != nil {
		t.Fatal("Failed to delete")
	}

	if _, err := table.Get(entryA.Address()); err != nil {
		if err != leveldb.ErrNotFound {
			t.Fatalf("Can't get, different error: %v", err)
		}
	} else {
		t.Fatalf("Delete didn't work")
	}
}

func TestVersionCertificateDBPrune(t *testing.T) {
	table, err := OpenVersionCertificateDB("")
	if err != nil {
		t.Fatal("Failed to open DB")
	}

	entryA, err := istanbul.NewVersionCertificate(1, signA)
	require.NoError(t, err)
	entryB, err := istanbul.NewVersionCertificate(1, signB)
	require.NoError(t, err)
	batch := []*istanbul.VersionCertificate{entryA, entryB}

	_, err = table.Upsert(batch)
	if err != nil {
		t.Fatal("Failed to upsert entry")
	}

	addressesToKeep := make(map[common.Address]bool)
	addressesToKeep[entryB.Address()] = true

	table.Prune(addressesToKeep)

	_, err = table.Get(entryB.Address())
	if err != nil {
		t.Errorf("It should have found %s after prune", entryB.Address().Hex())
	}
	_, err = table.Get(entryA.Address())
	if err == nil {
		t.Errorf("It should have NOT found %s after prune", entryA.Address().Hex())
	}

}

func TestVersionCertificateEntryRLP(t *testing.T) {

	original, err := istanbul.NewVersionCertificate(1, signA)
	require.NoError(t, err)

	rawEntry, err := rlp.EncodeToBytes(original)
	if err != nil {
		t.Errorf("Error %v", err)
	}

	var result istanbul.VersionCertificate
	if err = rlp.DecodeBytes(rawEntry, &result); err != nil {
		t.Errorf("Error %v", err)
	}

	if result.Address().String() != original.Address().String() {
		t.Errorf("node doesn't match: got: %s expected: %s", result.Address().String(), original.Address().String())
	}
	if result.Version != original.Version {
		t.Errorf("version doesn't match: got: %v expected: %v", result.Version, original.Version)
	}
	if !bytes.Equal(result.Signature, original.Signature) {
		t.Errorf("version doesn't match: got: %v expected: %v", result.Signature, original.Signature)
	}
}

// Compares the field values of two VersionCertificateEntrys
func versionCertificateEntriesEqual(a, b *istanbul.VersionCertificate) bool {
	return a.Address() == b.Address() &&
		bytes.Equal(crypto.FromECDSAPub(a.PublicKey()), crypto.FromECDSAPub(b.PublicKey())) &&
		a.Version == b.Version &&
		bytes.Equal(a.Signature, b.Signature)
}
