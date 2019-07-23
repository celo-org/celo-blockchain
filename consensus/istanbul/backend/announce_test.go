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
	_, b := newBlockChain(4, true)

	b.sendIstAnnounce()
	enodeUrl := "enodeurl"

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
	validatorAddr := b.Address()

	msg := &announceMessage{
		Address:            validatorAddr,
		EncryptedEnodeData: encryptedEnodeData,
		View:               b.core.CurrentView(),
	}

	if err := msg.Sign(b.Sign); err != nil {
		b.logger.Error("Error in signing an Istanbul Announce Message", "AnnounceMsg", msg.String(), "err", err)
		t.Errorf("error sign %v", err)
	}

	payload, err := msg.Payload()
	if err != nil {
		b.logger.Error("Error in converting Istanbul Announce Message to payload", "AnnounceMsg", msg.String(), "err", err)
		t.Errorf("error test %v", err)
	}

	b.Authorize(getInvalidAddress(), signerFnInvalid)

	if err = b.handleIstAnnounce(payload); err != nil {
		t.Errorf("error %v", err)
	}

	if b.valEnodeTable.valEnodeTable[validatorAddr] != nil {
		t.Errorf("Expected can't decrypt, got %v instead", b.valEnodeTable.valEnodeTable[validatorAddr])
	}

	b.Authorize(getAddress(), signerFn)

	if err = b.handleIstAnnounce(payload); err != nil {
		t.Errorf("error %v", err)
	}

	if b.valEnodeTable.valEnodeTable[validatorAddr] != nil {
		if b.valEnodeTable.valEnodeTable[validatorAddr].enodeURL != enodeUrl {
			t.Errorf("Should have been able to decrypt")
		}
	}
}

func getPublicKey() ecies.PublicKey {
	privateKey, _ := generatePrivateKey()
	return ecies.ImportECDSA(privateKey).PublicKey
}

func getInvalidPublicKey() ecies.PublicKey {
	privateKey, _ := generateInvalidPrivateKey()
	return ecies.ImportECDSA(privateKey).PublicKey
}