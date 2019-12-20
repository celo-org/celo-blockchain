package backend

import (
	"net"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	vet "github.com/ethereum/go-ethereum/consensus/istanbul/backend/internal/enodes"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestHandleValEnodeShareMsg(t *testing.T) {
	_, b := newBlockChain(4, true)
	for b == nil || b.Address() == getAddress() {
		_, b = newBlockChain(4, true)
	}

	senderAddress := b.Address()
	privateKey, _ := generatePrivateKey()

	// Tests that a validator enode share message without any validator info
	// in the payload will not result in errors
	msg, err := b.generateValEnodesShareMsg()
	if err != nil {
		t.Errorf("error %v", err)
	}

	msg.Sign(b.Sign)
	payload, err := msg.Payload()
	if err != nil {
		t.Errorf("error %v", err)
	}

	b.Authorize(getAddress(), signerFn, signerBLSHashFn, signerBLSMessageFn)

	// Set the backend's proxied validator address to itself
	b.config.ProxiedValidatorAddress = senderAddress

	if err = b.handleValEnodesShareMsg(payload); err != nil {
		t.Errorf("error %v", err)
	}

	if entries, err := b.valEnodeTable.GetAllValEnodes(); err != nil {
		t.Errorf("Error in calling GetAllValEndoes: %v", err)
	} else {
		if len(entries) > 0 {
			t.Errorf("The valEnodeTable should be empty")
		}
	}

	testAddress := getAddress()
	testNode := enode.NewV4(&privateKey.PublicKey, net.ParseIP("0.0.0.0"), 0, 0)

	// Test that a validator enode share message will result in the enode
	// being inserted into the valEnodeTable
	b.valEnodeTable.Upsert(map[common.Address]*vet.AddressEntry{testAddress: {
		Node:      testNode,
		Timestamp: 0,
	}})
	senderAddress = b.Address()
	newMsg, err := b.generateValEnodesShareMsg()
	if err != nil {
		t.Errorf("error %v", err)
	}

	newMsg.Sign(b.Sign)
	newPayload, err := newMsg.Payload()
	if err != nil {
		t.Errorf("error %v", err)
	}

	// Delete the entry in the valEnodeTable so that we can check if it's been
	// created after handling
	b.valEnodeTable.RemoveEntry(testAddress)

	b.config.ProxiedValidatorAddress = senderAddress
	if err = b.handleValEnodesShareMsg(newPayload); err != nil {
		t.Errorf("error %v", err)
	}

	if node, err := b.valEnodeTable.GetNodeFromAddress(testAddress); err != nil || node == nil {
		t.Errorf("Failed to save enode entry. err: %v, node: %s", err, node)
	} else if node.String() != testNode.String() {
		t.Errorf("Expected %v, but got %v instead", testNode.String(), node.String())
	}
}
