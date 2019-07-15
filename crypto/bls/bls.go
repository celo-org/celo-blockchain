package blscrypto

import (
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/celo-org/bls-zexe/go"
	"github.com/ethereum/go-ethereum/crypto"
)

const MODULUS377 = "8444461749428370424248824938781546531375899335154063827935233455917409239041"
const PUBLICKEYBYTES = 96
const SIGNATUREBYTES = 192

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

	return privateKeyBLSBytes, nil
}

func PrivateToPublic(privateKeyBytes []byte) ([]byte, error) {
	privateKey, err := bls.DeserializePrivateKey(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	defer privateKey.Destroy()

	publicKey, err := privateKey.ToPublic()
	if err != nil {
		return nil, err
	}
	defer publicKey.Destroy()

	pubKeyBytes, err := publicKey.Serialize()
	if err != nil {
		return nil, err
	}

	return pubKeyBytes, nil
}

func VerifyAggregatedSignature(publicKeys [][]byte, message []byte, signature []byte, shouldUseCompositeHasher bool) error {
	publicKeyObjs := []*bls.PublicKey{}
	for _, publicKey := range publicKeys {
		publicKeyObj, err := bls.DeserializePublicKey(publicKey)
		if err != nil {
			return err
		}
		defer publicKeyObj.Destroy()
		publicKeyObjs = append(publicKeyObjs, publicKeyObj)
	}
	apk, err := bls.AggregatePublicKeys(publicKeyObjs)
	if err != nil {
		return err
	}
	defer apk.Destroy()

	signatureObj, err := bls.DeserializeSignature(signature)
	if err != nil {
		return err
	}
	defer signatureObj.Destroy()

	err = apk.VerifySignature(message, signatureObj, shouldUseCompositeHasher)
	if err != nil {
		return err
	}

	return nil
}

func AggregateSignatures(signatures [][]byte) ([]byte, error) {
	signatureObjs := []*bls.Signature{}
	for _, signature := range signatures {
		signatureObj, err := bls.DeserializeSignature(signature)
		if err != nil {
			return nil, err
		}
		defer signatureObj.Destroy()
		signatureObjs = append(signatureObjs, signatureObj)
	}

	asig, err := bls.AggregateSignatures(signatureObjs)
	if err != nil {
		return nil, err
	}
	defer asig.Destroy()

	asigBytes, err := asig.Serialize()
	if err != nil {
		return nil, err
	}

	return asigBytes, nil
}
