package blscrypto

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/celo-org/bls-zexe/go"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	PUBLICKEYBYTES = bls.PUBLICKEYBYTES
	SIGNATUREBYTES = bls.SIGNATUREBYTES
)

type SerializedPublicKey [PUBLICKEYBYTES]byte
type SerializedSignature [SIGNATUREBYTES]byte

func ECDSAToBLS(privateKeyECDSA *ecdsa.PrivateKey) ([]byte, error) {
	for i := 0; i < 256; i++ {
		modulus := big.NewInt(0)
		modulus, ok := modulus.SetString(bls.MODULUS377, 10)
		if !ok {
			return nil, errors.New("can't parse modulus")
		}
		privateKeyECDSABytes := crypto.FromECDSA(privateKeyECDSA)

		keyBytes := []byte("ecdsatobls")
		keyBytes = append(keyBytes, uint8(i))
		keyBytes = append(keyBytes, privateKeyECDSABytes...)

		privateKeyBLSBytes := crypto.Keccak256(keyBytes)
		privateKeyBLSBytes[0] &= bls.MODULUSMASK
		privateKeyBLSBig := big.NewInt(0)
		privateKeyBLSBig.SetBytes(privateKeyBLSBytes)
		if privateKeyBLSBig.Cmp(modulus) >= 0 {
			continue
		}

		privateKeyBytes := privateKeyBLSBig.Bytes()
		for len(privateKeyBytes) < len(privateKeyBLSBytes) {
			privateKeyBytes = append([]byte{0x00}, privateKeyBytes...)
		}
		if !bytes.Equal(privateKeyBLSBytes, privateKeyBytes) {
			return nil, fmt.Errorf("private key bytes should have been the same: %s, %s", hex.EncodeToString(privateKeyBLSBytes), hex.EncodeToString(privateKeyBytes))
		}
		// reverse order, as the BLS library expects little endian
		for i := len(privateKeyBytes)/2 - 1; i >= 0; i-- {
			opp := len(privateKeyBytes) - 1 - i
			privateKeyBytes[i], privateKeyBytes[opp] = privateKeyBytes[opp], privateKeyBytes[i]
		}

		privateKeyBLS, err := bls.DeserializePrivateKey(privateKeyBytes)
		if err != nil {
			return nil, err
		}
		defer privateKeyBLS.Destroy()
		privateKeyBLSBytesFromLib, err := privateKeyBLS.Serialize()
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(privateKeyBytes, privateKeyBLSBytesFromLib) {
			return nil, errors.New("private key bytes from library should have been the same")
		}

		return privateKeyBLSBytesFromLib, nil
	}

	return nil, errors.New("couldn't derive a BLS key from an ECDSA key")
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

func VerifyAggregatedSignature(publicKeys []SerializedPublicKey, message []byte, extraData []byte, signature []byte, shouldUseCompositeHasher bool) error {
	publicKeyObjs := []*bls.PublicKey{}
	for _, publicKey := range publicKeys {
		publicKeyObj, err := bls.DeserializePublicKey(publicKey[:])
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

	err = apk.VerifySignature(message, extraData, signatureObj, shouldUseCompositeHasher)
	return err
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

func VerifySignature(publicKey SerializedPublicKey, message []byte, extraData []byte, signature []byte, shouldUseCompositeHasher bool) error {
	publicKeyObj, err := bls.DeserializePublicKey(publicKey[:])
	if err != nil {
		return err
	}
	defer publicKeyObj.Destroy()

	signatureObj, err := bls.DeserializeSignature(signature)
	if err != nil {
		return err
	}
	defer signatureObj.Destroy()

	err = publicKeyObj.VerifySignature(message, extraData, signatureObj, shouldUseCompositeHasher)
	return err
}

func EncodeEpochSnarkData(newValSet []SerializedPublicKey, epochIndex uint64) ([]byte, error) {
	return nil, nil
}
