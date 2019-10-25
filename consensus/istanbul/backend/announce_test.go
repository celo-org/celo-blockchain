package backend

/*

import (
	"net"
	"testing"

	"github.com/ethereum/go-ethereum/consensus/consensustest"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestHandleIstAnnounce(t *testing.T) {
	_, b := newBlockChain(4, true)
	for b == nil || b.Address() == getAddress() {
		_, b = newBlockChain(4, true)
	}

	valAddresses := b.GetValidators()

	val1PrivateKey, _ := generatePrivateKey()
	val1IPAddress := net.ParseIP("1.2.3.4")
	val1Node := enode.NewV4(&val1PrivateKey.PublicKey, val1IPAddress, 0, 0)
	val1Addr := getAddress()
	val1P2pServer := &consensustest.MockP2PServer{Node: val1Node}

	// Caution - "generateInvalidPrivateKey" is a misnomer.  We just need to get another key that is different
	//          than the one returned by "generatePrivateKey", and "generateInvalidPrivateKey" does just that.
	val2PrivateKey, _ := generateInvalidPrivateKey()
	val2IPAddress := net.ParseIP("4.3.2.1")
	val2Node := enode.NewV4(&val2PrivateKey.PublicKey, val2IPAddress, 0, 0)
	val2Addr := getInvalidAddress()
	val2P2pServer := &consensustest.MockP2PServer{Node: val2Node}

	// Set backend to val1
	b.SetP2PServer(val1P2pServer)
	b.Authorize(val1Addr, signerFn, signerBLSHashFn, signerBLSMessageFn)

	// Generate an ist announce message using val1
	istMsg, err := b.generateIstAnnounce()
	istMsg.Sign(b.Sign)
	payload, _ := istMsg.Payload()

	// Set backend to val2
	b.SetP2PServer(val2P2pServer)
	b.Authorize(val2Addr, signerFnInvalid, signerBLSHashFn, signerBLSMessageFn)

	// Handle val1's announce message
	if err = b.handleIstAnnounce(payload); err != nil {
		t.Errorf("error %v", err)
	}

	// Verify that the valenodetable entry correctly got set.
	if b.valEnodeTable.valEnodeTable[val1Addr] != nil {
		if b.valEnodeTable.valEnodeTable[val1Addr].node.String() != val1Node.String() {
			t.Errorf("Expected %v, got %v instead", val1Node.String(), b.valEnodeTable.valEnodeTable[val1Addr].node.String())
		}
	} else {
		t.Errorf("Failed to save enode entry")
	}
}
*/
