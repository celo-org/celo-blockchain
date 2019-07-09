package blscrypto

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	bls "github.com/celo-org/bls-zexe/go"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
)

const MODULUS377 = "8444461749428370424248824938781546531375899335154063827935233455917409239041"

func ECDSAToBLS(privateKeyECDSA *ecdsa.PrivateKey) ([]byte, error) {
	modulus := big.NewInt(0)
	modulus, ok := modulus.SetString(MODULUS377, 10)
	if !ok {
		return nil, errors.New("can't parse modulus")
	}
	privateKeyECDSABytes := crypto.FromECDSA(privateKeyECDSA)

	part1Bytes := []byte{0x1}
	part1Bytes = append(part1Bytes, privateKeyECDSABytes...)
	part2Bytes := []byte{0x2}
	part2Bytes = append(part2Bytes, privateKeyECDSABytes...)

	privateKeyBLSBytesBeforeMod := crypto.Keccak256(part1Bytes)
	privateKeyBLSBytesBeforeMod = append(privateKeyBLSBytesBeforeMod, crypto.Keccak256(part2Bytes)...)
	privateKeyBLSBig := big.NewInt(0)
	privateKeyBLSBig.SetBytes(privateKeyBLSBytesBeforeMod)
	privateKeyBLSBig.Mod(privateKeyBLSBig, modulus)
	privateKeyBytes := privateKeyBLSBig.Bytes()

	for i := len(privateKeyBytes)/2 - 1; i >= 0; i-- {
		opp := len(privateKeyBytes) - 1 - i
		privateKeyBytes[i], privateKeyBytes[opp] = privateKeyBytes[opp], privateKeyBytes[i]
	}

	privateKeyBLS, err := bls.DeserializePrivateKey(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	defer privateKeyBLS.Destroy()
	privateKeyBLSBytes, err := privateKeyBLS.Serialize()
	if err != nil {
		return nil, err
	}
	log.Debug("ECDSAToBLS", "bytes", hex.EncodeToString(privateKeyBLSBytes))

	return privateKeyBLSBytes, nil
}

func IstanbulSigToPub(pubKeyBytes []byte) ([]byte, error) {
	publicKey, err := bls.DeserializePublicKey(pubKeyBytes)
	if err != nil {
		return nil, err
	}
	defer publicKey.Destroy()

	signature, err := bls.DeserializeSignature(sigBytes)
	if err != nil {
		return nil, err
	}
	defer signature.Destroy()

	err = publicKey.VerifySignature(msg, signature)
	if err != nil {
		return nil, err
	}

	return pubKeyBytes, nil
}
