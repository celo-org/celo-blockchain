package blscrypto

import (
	"testing"

	"github.com/celo-org/bls-zexe/go"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestECDSAToBLS(t *testing.T) {
	privateKeyECDSA, _ := crypto.HexToECDSA("4f837096cd8578c1f14c9644692c444bbb61426297ff9e8a78a1e7242f541fb3")
	privateKeyBLSBytes, _ := ECDSAToBLS(privateKeyECDSA)
	t.Logf("private key: %x", privateKeyBLSBytes)
	privateKeyBLS, _ := bls.DeserializePrivateKey(privateKeyBLSBytes)
	publicKeyBLS, _ := privateKeyBLS.ToPublic()
	publicKeyBLSBytes, _ := publicKeyBLS.Serialize()
	t.Logf("public key: %x", publicKeyBLSBytes)

	pop, _ := privateKeyBLS.SignPoP()
	popBytes, _ := pop.Serialize()
	t.Logf("pop: %x", popBytes)
}
