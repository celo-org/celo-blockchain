package backend

import (
	"testing"
	"math/big"

	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

func TestHandleValEnodeShareMsg(t *testing.T) {
	_, b := newBlockChain(4, true)
	for b == nil || b.Address() == getAddress() {
		_, b = newBlockChain(4, true)
	}

	privateKey, _ := generatePrivateKey()
	broadcaster := &MockBroadcaster{privateKey: privateKey}

	// Tests that a validator enode share message without any validator info
	// in the payload will not result in errors
	payload, err := b.generateValEnodeShareMsg()
	if err != nil {
		t.Errorf("error %v", err)
	}

	b.Authorize(getAddress(), signerFn, signerBLSHashFn, signerBLSMessageFn)
	b.SetBroadcaster(broadcaster)

	if err = b.handleValEnodeShareMsg(payload); err != nil {
		t.Errorf("error %v", err)
	}

	if len(b.valEnodeTable.valEnodeTable) > 0 {
		t.Errorf("The valEnodeTable should be empty")
	}

	testAddress := getAddress()
	testEnode := broadcaster.GetLocalNode().String()

	// Test that a validator enode share message will result in the enode
	// being inserted into the valEnodeTable
	b.valEnodeTable.valEnodeTable[testAddress] = &validatorEnode{
		enodeURL: testEnode,
		view: &istanbul.View{
			Round: big.NewInt(0),
			Sequence: big.NewInt(0),
		},
	}
	payload, err = b.generateValEnodeShareMsg()
	if err != nil {
		t.Errorf("error %v", err)
	}

	// Delete the entry in the valEnodeTable so that we can check if it's been
	// created after handling
	delete(b.valEnodeTable.valEnodeTable, testAddress)

	if err = b.handleValEnodeShareMsg(payload); err != nil {
		t.Errorf("error %v", err)
	}
	
	if b.valEnodeTable.valEnodeTable[testAddress] != nil {
		if b.valEnodeTable.valEnodeTable[testAddress].enodeURL != testEnode {
			t.Errorf("Expected %v, but got %v instead", testEnode, b.valEnodeTable.valEnodeTable[testAddress].enodeURL)
		}
	} else {
		t.Errorf("Failed to save enode entry")
	}
}
