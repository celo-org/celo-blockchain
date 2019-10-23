package backend

import (
	"math/big"
	"net"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestHandleValEnodeShareMsg(t *testing.T) {
	_, b := newBlockChain(4, true)
	for b == nil || b.Address() == getAddress() {
		_, b = newBlockChain(4, true)
	}

	privateKey, _ := generatePrivateKey()

	// Tests that a validator enode share message without any validator info
	// in the payload will not result in errors
	msg, err := b.generateValEnodeShareMsg()
	if err != nil {
		t.Errorf("error %v", err)
	}

	msg.Sign(b.Sign)
	payload, err := msg.Payload()

	b.Authorize(getAddress(), signerFn, signerBLSHashFn, signerBLSMessageFn)

	if err = b.handleValEnodeShareMsg(payload); err != nil {
		t.Errorf("error %v", err)
	}

	if len(b.valEnodeTable.valEnodeTable) > 0 {
		t.Errorf("The valEnodeTable should be empty")
	}

	testAddress := getAddress()
	testNode := enode.NewV4(&privateKey.PublicKey, net.ParseIP("0.0.0.0"), 0, 0)

	// Test that a validator enode share message will result in the enode
	// being inserted into the valEnodeTable
	b.valEnodeTable.valEnodeTable[testAddress] = &validatorEnode{
		node: testNode,
		view: &istanbul.View{
			Round:    big.NewInt(0),
			Sequence: big.NewInt(0),
		},
	}
	newMsg, err := b.generateValEnodeShareMsg()
	if err != nil {
		t.Errorf("error %v", err)
	}

	newMsg.Sign(b.Sign)
	newPayload, err := newMsg.Payload()

	// Delete the entry in the valEnodeTable so that we can check if it's been
	// created after handling
	delete(b.valEnodeTable.valEnodeTable, testAddress)

	if err = b.handleValEnodeShareMsg(newPayload); err != nil {
		t.Errorf("error %v", err)
	}

	if b.valEnodeTable.valEnodeTable[testAddress] != nil {
		if b.valEnodeTable.valEnodeTable[testAddress].node.String() != testNode.String() {
			t.Errorf("Expected %v, but got %v instead", testNode.String(), b.valEnodeTable.valEnodeTable[testAddress].node.String())
		}
	} else {
		t.Errorf("Failed to save enode entry")
	}
}
