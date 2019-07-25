package backend

import (
	"crypto/ecdsa"
	// "crypto/rand"
	// "encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/types"
	// "github.com/ethereum/go-ethereum/crypto/ecies"
	// "github.com/ethereum/go-ethereum/log"
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
	return
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

func TestHandleIstAnnounce(t *testing.T) {
	_, b := newBlockChain(4, true)
	for b == nil || b.Address() == getAddress() {
		_, b = newBlockChain(4, true)
	}

	enodeUrl := b.Enode().String()
	validatorAddr := b.Address()

	privateKey, _ := generatePrivateKey()
	broadcaster := &MockBroadcaster{privateKey: privateKey}
	b.valEnodeTable.valEnodeTable[getAddress()] = &validatorEnode{enodeURL: broadcaster.GetLocalNode().String()}

	payload, err := b.generateIstAnnounce()

	b.Authorize(getInvalidAddress(), signerFnInvalid)
	invalidPrivateKey, _ := generateInvalidPrivateKey()
	b.SetBroadcaster(&MockBroadcaster{privateKey: invalidPrivateKey})

	if err = b.handleIstAnnounce(payload); err != nil {
		t.Errorf("error %v", err)
	}

	if b.valEnodeTable.valEnodeTable[validatorAddr] != nil {
		if b.valEnodeTable.valEnodeTable[validatorAddr].enodeURL != enodeUrl[:strings.Index(enodeUrl, "@")] {
			t.Errorf("Expected %v, got %v instead", enodeUrl[:strings.Index(enodeUrl, "@")], b.valEnodeTable.valEnodeTable[validatorAddr])
		}
	} else {
		t.Errorf("Failed to save enode entry")
	}
	delete(b.valEnodeTable.valEnodeTable, validatorAddr)

	b.Authorize(getAddress(), signerFn)
	b.SetBroadcaster(broadcaster)

	if err = b.handleIstAnnounce(payload); err != nil {
		t.Errorf("error %v", err)
	}

	if b.valEnodeTable.valEnodeTable[validatorAddr] != nil {
		if b.valEnodeTable.valEnodeTable[validatorAddr].enodeURL != enodeUrl {
			t.Errorf("Expected %v, but got %v instead", enodeUrl, b.valEnodeTable.valEnodeTable[validatorAddr].enodeURL)
		}
	} else {
		t.Errorf("Failed to save enode entry")
	}
}

func getPublicKey() ecdsa.PublicKey {
	privateKey, _ := generatePrivateKey()
	return privateKey.PublicKey
}
