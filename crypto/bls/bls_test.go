package blscrypto

import (
	"encoding/hex"
	"testing"

	//nolint:goimports
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/celo-org/celo-bls-go/bls"
)

func TestECDSAToBLS(t *testing.T) {
	privateKeyECDSA, _ := crypto.HexToECDSA("4f837096cd8578c1f14c9644692c444bbb61426297ff9e8a78a1e7242f541fb3")
	privateKeyBLSBytes, _ := ECDSAToBLS(privateKeyECDSA)
	t.Logf("private key: %x", privateKeyBLSBytes)
	privateKeyBLS, _ := bls.DeserializePrivateKey(privateKeyBLSBytes)
	publicKeyBLS, _ := privateKeyBLS.ToPublic()
	publicKeyBLSBytes, _ := publicKeyBLS.Serialize()
	t.Logf("public key: %x", publicKeyBLSBytes)

	address, _ := hex.DecodeString("4f837096cd8578c1f14c9644692c444bbb614262")
	pop, _ := privateKeyBLS.SignPoP(address)
	popBytes, _ := pop.Serialize()
	t.Logf("pop: %x", popBytes)
}

func split(buf []byte, lim int) []SerializedPublicKey {
	chunks := make([]SerializedPublicKey, 0, len(buf)/lim+1)
	for len(buf) >= lim {
		pubKeyBytesFixed := SerializedPublicKey{}
		copy(pubKeyBytesFixed[:], buf[:lim])
		buf = buf[lim:]
		chunks = append(chunks, pubKeyBytesFixed)
	}
	if len(buf) > 0 {
		pubKeyBytesFixed := SerializedPublicKey{}
		copy(pubKeyBytesFixed[:], buf)
		chunks = append(chunks, pubKeyBytesFixed)
	}
	return chunks
}

func TestEncodeEpochSnarkData(t *testing.T) {
	// Serialize the public keys for the validators in the validator set.
	pubkeys := "45a3ed64a457fbc0e875b0d6dcc372216f96571eefd7a07d373a4de2b73cbebe6b7d43025a4306d356f5fc189ea720013295a3110785f5f7783e7e22a582b810ffdc5e3b10a61c38d3ee0f70ddc59294dd03d4753c7a3500f3c1456d19571981d13b719de39cbf8c84a840484820d3b80836bfa161971f0c32dcd6b23d72adf3d817b9e648082d7e1c0a39fb6393390153ba4ca1ec7fb74a7c4c4f77c2399a214535b303c629b298fa946bbb4c7325ed3a7ac15fe8fdb311287cb06b75ba94813e511d58c8c12709103dfd66c13797c404509da9659f6395b318866b448a2150ffbc4f3f4524d3c5fc453e7020f7a2009ea4bdceed84a0431f153aa834a947bb1ed239f95d9c32c3110e0937687012e44d5e68cadefdc10f7bea106bfcb07881b66e28d8b7fb1418bf311830eba1b0cffe5ec9348ec6b54f2bb21434dce17176279d5525694499b6988b4ecaa8232000f473f369e191669fa3e5ff781f3040fd3b16f694b6bb6798d7f3067c62d49180022cbb9f33f964bb4ddfb20019c85780"
	pk1, _ := hex.DecodeString(pubkeys)
	blsPubKeys := split(pk1, PUBLICKEYBYTES)

	maxNonSigners := uint32(1)

	// Before the Donut fork, use the snark data encoding without epoch entropy.
	encodedEpochBlock, encodedEpochBlockExtraData, err := EncodeEpochSnarkData(blsPubKeys, maxNonSigners, 1)
	if err != nil {
		t.Log("Error ", err)
	}
	t.Logf("Encoded epoch block: %x", encodedEpochBlock)
	t.Logf("Encoded epoch block extra data: %x", encodedEpochBlockExtraData)
}
