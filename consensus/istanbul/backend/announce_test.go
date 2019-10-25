package backend

import (
	"crypto/ecdsa"
	"math/big"
	"net"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

type MockBroadcaster struct {
	privateKey *ecdsa.PrivateKey
}

func (mb *MockBroadcaster) GetLocalNode() *enode.Node {
	publicKey := mb.privateKey.PublicKey
	return enode.NewV4(&publicKey, net.ParseIP("10.3.58.6"), 30303, 30301)
}

func (mb *MockBroadcaster) GetNodeKey() *ecdsa.PrivateKey {
	return mb.privateKey
}

func (mb *MockBroadcaster) Enqueue(id string, block *types.Block) {
}

func (mb *MockBroadcaster) FindPeers(map[common.Address]bool) map[common.Address]consensus.Peer {
	return nil
}

func (mb *MockBroadcaster) AddValidatorPeer(enodeURL string) error {
	return nil
}

func (mb *MockBroadcaster) RemoveValidatorPeer(enodeURL string) error {
	return nil
}

func (mb *MockBroadcaster) GetValidatorPeers() []string {
	return nil
}

func view(sequence, round int64) *istanbul.View {
	return &istanbul.View{
		Sequence: big.NewInt(sequence),
		Round:    big.NewInt(round),
	}
}
func TestHandleIstAnnounce(t *testing.T) {
	_, b := newBlockChain(4, true)
	for b == nil || b.Address() == getAddress() {
		_, b = newBlockChain(4, true)
	}

	enodeURL := b.Enode().String()
	validatorAddr := b.Address()

	privateKey, _ := generatePrivateKey()
	broadcaster := &MockBroadcaster{privateKey: privateKey}

	b.valEnodeTable.Upsert(getAddress(), broadcaster.GetLocalNode().String(), view(0, 1))

	payload, err := b.generateIstAnnounce()

	b.Authorize(getInvalidAddress(), signerFnInvalid, signerBLSHashFn, signerBLSMessageFn)
	invalidPrivateKey, _ := generateInvalidPrivateKey()
	b.SetBroadcaster(&MockBroadcaster{privateKey: invalidPrivateKey})

	if err = b.handleIstAnnounce(payload); err != nil {
		t.Errorf("error %v", err)
	}

	if url, ok := b.valEnodeTable.GetEnodeURLFromAddress(validatorAddr); ok {
		if url != enodeURL[:strings.Index(enodeURL, "@")] {
			t.Errorf("Expected %v, got %v instead", enodeURL[:strings.Index(enodeURL, "@")], url)
		}
	} else {
		t.Errorf("Failed to save enode entry")
	}

	b.valEnodeTable.RemoveEntry(validatorAddr)

	b.Authorize(getAddress(), signerFn, signerBLSHashFn, signerBLSMessageFn)
	b.SetBroadcaster(broadcaster)

	if err = b.handleIstAnnounce(payload); err != nil {
		t.Errorf("error %v", err)
	}

	if url, ok := b.valEnodeTable.GetEnodeURLFromAddress(validatorAddr); ok {
		if url != enodeURL {
			t.Errorf("Expected %v, but got %v instead", enodeURL, url)
		}
	} else {
		t.Errorf("Failed to save enode entry")
	}
}

func getPublicKey() ecdsa.PublicKey {
	privateKey, _ := generatePrivateKey()
	return privateKey.PublicKey
}
