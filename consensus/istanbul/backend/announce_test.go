package backend

import (
	"crypto/ecdsa"
	"net"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/consensustest"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestHandleIstAnnounce(t *testing.T) {
	_, b := newBlockChain(4, true)
	for b == nil || b.Address() == getAddress() {
		_, b = newBlockChain(4, true)
	}

	block := b.currentBlock()
	valSet := b.getValidators(block.Number().Uint64(), block.Hash())

	val1PrivateKey, _ := generatePrivateKey()
	val1IPAddress := net.ParseIP("1.2.3.4")
	val1Node := enode.NewV4(&val1PrivateKey.PublicKey, val1IPAddress, 0, 0)
	val1Addr := getAddress()
	val1P2pServer := &consensustest.MockP2PServer{Node: val1Node}

	// Set backend to val1
	b.SetP2PServer(val1P2pServer)
	b.Authorize(val1Addr, &val1PrivateKey.PublicKey, decryptFn, SignFn(nil), SignBLSFn(nil))

	val2Address := valSet.GetByIndex(2).Address()

	// Pretend the signed announce version belongs to a different address
	// but use val 1's public key so we can decrypt it using val 1's private key
	destAddresses := []common.Address{val2Address}
	publicKeys := []*ecdsa.PublicKey{b.publicKey}

	// Generate an ist announce message using val1
	istMsg, err := b.generateQueryEnodeMsg(getTimestamp(), destAddresses, publicKeys)
	if err != nil {
		t.Fatalf("Error on generateAnnounce: %s", err)
	}
	istMsg.Sign(b.Sign)
	payload, _ := istMsg.Payload()

	// Set backend to val2
	b.address = val2Address

	// Handle val1's announce message
	if err = b.handleQueryEnodeMsg(nil, payload); err != nil {
		t.Errorf("error %v", err)
	}

	if node, err := b.valEnodeTable.GetNodeFromAddress(val1Addr); err == nil {
		if node == nil || node.String() != val1Node.String() {
			t.Errorf("Expected %v, but got %v instead", val1Node.String(), node)
		}
	} else {
		t.Errorf("Failed to save enode entry")
	}
}
