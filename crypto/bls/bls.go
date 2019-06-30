package crypto

import (
	"crypto/ecdsa"

	bls "github.com/celo-org/bls-zexe/go"
	"github.com/ethereum/go-ethereum/crypto"
)

func ECDSAToBLS(privateKeyECDSA *ecdsa.PrivateKey) (*bls.PrivateKey, error) {
	privateKeyBytes := crypto.FromECDSA(privateKeyECDSA)
	privateKeyBLS, err := bls.DeserializePrivateKey(privateKeyBytes)
	if err != nil {
		return nil, err
	}

	return privateKeyBLS, nil
}
