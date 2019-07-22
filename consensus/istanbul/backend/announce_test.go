package backend

import (
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/log"
)
func TestHandleIstAnnounce(t *testing.T) {
	b := newBackend()
	ib := invalidBackend()
	enodeUrl := "wow"

	encryptedEnodeUrls := make(map[common.Address][]byte)
	pubKey := getPublicKey()
	encryptedEnodeUrl, err := ecies.Encrypt(rand.Reader, &pubKey, []byte(enodeUrl), nil, nil)
	if err != nil {
		log.Error("Unable to unmarshal public key", "err", err)
		t.Errorf("error test %v", err)
	} else {
		encryptedEnodeUrls[getAddress()] = encryptedEnodeUrl
	}

	encryptedEnodeData, err := json.Marshal(encryptedEnodeUrls)

	msg := &announceMessage{
		Address:            getInvalidAddress(),
		EncryptedEnodeData: encryptedEnodeData,
		View:               b.core.CurrentView(),
	}

	if err := msg.Sign(ib.Sign); err != nil {
		b.logger.Error("Error in signing an Istanbul Announce Message", "AnnounceMsg", msg.String(), "err", err)
		t.Errorf("error test %v", err)
	}

	payload, err := msg.Payload()
	if err != nil {
		b.logger.Error("Error in converting Istanbul Announce Message to payload", "AnnounceMsg", msg.String(), "err", err)
		t.Errorf("error test %v", err)
	}

	err = b.handleIstAnnounce(payload)

	if err != nil {
		t.Errorf("error %v", err)
		t.Errorf("error test %v", err)
	}

	t.Errorf("asdf %v", b.lastAnnounceGossiped[getInvalidAddress()])

	
	t.Errorf("error mismatch: have %v, want nil", b.valEnodeTable.valEnodeTable[getInvalidAddress()])
}

func getPublicKey() ecies.PublicKey {
	privateKey, _ := generatePrivateKey()
	return ecies.ImportECDSA(privateKey).PublicKey
}

func getInvalidPublicKey() ecies.PublicKey {
	privateKey, _ := generateInvalidPrivateKey()
	return ecies.ImportECDSA(privateKey).PublicKey
}