package enodes

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
)

var (
	addressA  = common.HexToAddress("0x00Ce0d46d924CC8437c806721496599FC3FFA268")
	addressB  = common.HexToAddress("0xFFFFFF46d924CCFFFFc806721496599FC3FFFFFF")
	enodeURLA = "enode://1dd9d65c4552b5eb43d5ad55a2ee3f56c6cbc1c64a5c8d659f51fcd51bace24351232b8d7821617d2b29b54b81cdefb9b3e9c37d7fd5f63270bcc9e1a6f6a439@127.0.0.1:52150"
	enodeURLB = "enode://38b219b54ed49cf7d802e8add586fc75b531ed2c31e43b5da71c35982b2e6f5c56fa9cfbe39606fe71fbee2566b94c2874e950b1ec88323103c835246e3d0023@127.0.0.1:37303"
	nodeA, _  = enode.ParseV4(enodeURLA)
	nodeB, _  = enode.ParseV4(enodeURLB)
)

type mockListener struct{}

func (ml *mockListener) AddValidatorPeer(node *enode.Node, address common.Address) {}
func (ml *mockListener) RemoveValidatorPeer(node *enode.Node)                      {}
func (ml *mockListener) ReplaceValidatorPeers(nodeNodes []*enode.Node)             {}
func (ml *mockListener) ClearValidatorPeers()                                      {}

func TestSimpleCase(t *testing.T) {
	vet, err := OpenValidatorEnodeDB("", &mockListener{})
	if err != nil {
		t.Fatal("Failed to open DB")
	}

	addressEntry := &istanbul.AddressEntry{Address: addressA, Node: nodeA, Version: 1}

	err = vet.UpsertVersionAndEnode([]*istanbul.AddressEntry{addressEntry})
	if err != nil {
		t.Fatal("Failed to upsert")
	}

	addr, err := vet.GetAddressFromNodeID(nodeA.ID())
	if err != nil {
		t.Errorf("got %v", err)
	}
	if addr != addressA {
		t.Error("Invalid address saved")
	}

	node, err := vet.GetNodeFromAddress(addressA)
	if err != nil {
		t.Errorf("got %v", err)
	}
	if node.String() != enodeURLA {
		t.Error("Invalid enode saved")
	}
}

func TestDeleteEntry(t *testing.T) {
	vet, err := OpenValidatorEnodeDB("", &mockListener{})
	if err != nil {
		t.Fatal("Failed to open DB")
	}

	addressEntry := &istanbul.AddressEntry{Address: addressA, Node: nodeA, Version: 2}

	err = vet.UpsertVersionAndEnode([]*istanbul.AddressEntry{addressEntry})
	if err != nil {
		t.Fatal("Failed to upsert")
	}

	err = vet.RemoveEntry(addressA)
	if err != nil {
		t.Fatal("Failed to delete")
	}

	if _, err := vet.GetNodeFromAddress(addressA); err != nil {
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

	batch := []*istanbul.AddressEntry{
		{Address: addressA, Node: nodeA, Version: 2},
		{Address: addressB, Node: nodeB, Version: 2},
	}

	vet.UpsertVersionAndEnode(batch)

	addressesToKeep := make(map[common.Address]bool)
	addressesToKeep[addressB] = true

	vet.PruneEntries(addressesToKeep)

	_, err = vet.GetNodeFromAddress(addressB)
	if err != nil {
		t.Errorf("It should have found %s after prune", addressB.Hex())
	}
	_, err = vet.GetNodeFromAddress(addressA)
	if err == nil {
		t.Errorf("It should have NOT found %s after prune", addressA.Hex())
	}

}

func TestRLPEntries(t *testing.T) {
	original := istanbul.AddressEntry{Address: addressA, Node: nodeA, Version: 1}

	rawEntry, err := rlp.EncodeToBytes(&original)
	if err != nil {
		t.Errorf("Error %v", err)
	}

	var result istanbul.AddressEntry
	if err = rlp.DecodeBytes(rawEntry, &result); err != nil {
		t.Errorf("Error %v", err)
	}

	if result.Node.String() != original.Node.String() {
		t.Errorf("node doesn't match: got: %s expected: %s", result.Node.String(), original.Node.String())
	}
	if result.Version != original.Version {
		t.Errorf("version doesn't match: got: %v expected: %v", result.Version, original.Version)
	}
}

func TestTableToString(t *testing.T) {
	vet, err := OpenValidatorEnodeDB("", &mockListener{})
	if err != nil {
		t.Fatal("Failed to open DB")
	}

	batch := []*istanbul.AddressEntry{
		{Address: addressA, Node: nodeA, Version: 2},
		{Address: addressB, Node: nodeB, Version: 2},
	}

	vet.UpsertVersionAndEnode(batch)

	expected := "ValEnodeTable: [0x00Ce0d46d924CC8437c806721496599FC3FFA268 => {address: 0x00Ce0d46d924CC8437c806721496599FC3FFA268, enodeURL: enode://1dd9d65c4552b5eb43d5ad55a2ee3f56c6cbc1c64a5c8d659f51fcd51bace24351232b8d7821617d2b29b54b81cdefb9b3e9c37d7fd5f63270bcc9e1a6f6a439@127.0.0.1:52150, version: 2, highestKnownVersion: 0, numQueryAttempsForHKVersion: 0, LastQueryTimestamp: 0001-01-01 00:00:00 +0000 UTC}] [0xfFFFff46D924CCfffFc806721496599fC3FFffff => {address: 0xfFFFff46D924CCfffFc806721496599fC3FFffff, enodeURL: enode://38b219b54ed49cf7d802e8add586fc75b531ed2c31e43b5da71c35982b2e6f5c56fa9cfbe39606fe71fbee2566b94c2874e950b1ec88323103c835246e3d0023@127.0.0.1:37303, version: 2, highestKnownVersion: 0, numQueryAttempsForHKVersion: 0, LastQueryTimestamp: 0001-01-01 00:00:00 +0000 UTC}]"

	if vet.String() != expected {
		t.Errorf("String() error: got: %s", vet.String())
	}
}
