package bls

import (
	"encoding/hex"
	"fmt"
	"testing"
)

// this test is a copy of the `bls-crypto::keys::test_batch_verify` Rust test
func TestBatchVerify(t *testing.T) {
	InitBLSCrypto()

	testBatchVerify(t, true)
	testBatchVerify(t, false)
}

func testBatchVerify(t *testing.T, mode bool) {
	num_epochs := 10
	num_validators := 7
	var msgs []*SignedBlockHeader
	for i := 0; i < num_epochs; i++ {
		message := []byte(fmt.Sprintf("msg_%d", i))
		extraData := []byte(fmt.Sprintf("extra_%d", i))
		var epoch_sigs []*Signature
		var epoch_pubkeys []*PublicKey
		for j := 0; j < num_validators; j++ {
			// generate a private key
			privateKey, _ := GeneratePrivateKey()

			// sign each message
			signature, _ := privateKey.SignMessage(message, extraData, mode)
			// save the sig to generate the epoch's asig
			epoch_sigs = append(epoch_sigs, signature)

			// save the pubkey to generate the epoch's apubkey
			publicKey, _ := privateKey.ToPublic()
			epoch_pubkeys = append(epoch_pubkeys, publicKey)
		}

		epoch_asig, _ := AggregateSignatures(epoch_sigs)
		epoch_apubkey, _ := AggregatePublicKeys(epoch_pubkeys)

		msg := &SignedBlockHeader{
			Data:   message,
			Extra:  extraData,
			Pubkey: epoch_apubkey,
			Sig:    epoch_asig,
		}
		msgs = append(msgs, msg)
	}

	err := BatchVerifyEpochs(msgs, mode)
	if err != nil {
		t.Fatalf("batch verification failed, err: %s", err)
	}
}

func TestAggregatedSig(t *testing.T) {
	InitBLSCrypto()
	privateKey, _ := GeneratePrivateKey()
	defer privateKey.Destroy()
	publicKey, _ := privateKey.ToPublic()
	message := []byte("test")
	extraData := []byte("extra")
	signature, _ := privateKey.SignMessage(message, extraData, true)
	err := publicKey.VerifySignature(message, extraData, signature, true)
	if err != nil {
		t.Fatalf("failed verifying signature for pk 1, error was: %s", err)
	}

	privateKey2, _ := GeneratePrivateKey()
	defer privateKey2.Destroy()
	publicKey2, _ := privateKey2.ToPublic()
	signature2, _ := privateKey2.SignMessage(message, extraData, true)
	err = publicKey2.VerifySignature(message, extraData, signature2, true)
	if err != nil {
		t.Fatalf("failed verifying signature for pk 2, error was: %s", err)
	}

	aggergatedPublicKey, _ := AggregatePublicKeys([]*PublicKey{publicKey, publicKey2})
	aggergatedSignature, _ := AggregateSignatures([]*Signature{signature, signature2})
	err = aggergatedPublicKey.VerifySignature(message, extraData, aggergatedSignature, true)
	if err != nil {
		t.Fatalf("failed verifying signature for aggregated pk, error was: %s", err)
	}
	err = publicKey.VerifySignature(message, extraData, aggergatedSignature, true)
	if err == nil {
		t.Fatalf("succeeded verifying signature for wrong pk, shouldn't have!")
	}

	subtractedPublicKey, _ := AggregatePublicKeysSubtract(aggergatedPublicKey, []*PublicKey{publicKey2})
	err = subtractedPublicKey.VerifySignature(message, extraData, signature, true)
	if err != nil {
		t.Fatalf("failed verifying signature for subtractedPublicKey pk, error was: %s", err)
	}
}

func TestProofOfPossession(t *testing.T) {
	privateKey, _ := GeneratePrivateKey()
	defer privateKey.Destroy()
	publicKey, _ := privateKey.ToPublic()
	message := []byte("just some message")
	signature, _ := privateKey.SignPoP(message)
	err := publicKey.VerifyPoP(message, signature)
	if err != nil {
		t.Fatalf("failed verifying PoP for pk 1, error was: %s", err)
	}

	privateKey2, _ := GeneratePrivateKey()
	defer privateKey2.Destroy()
	publicKey2, _ := privateKey2.ToPublic()
	if err != nil {
		t.Fatalf("failed verifying PoP for pk 2, error was: %s", err)
	}

	err = publicKey2.VerifyPoP(message, signature)
	if err == nil {
		t.Fatalf("succeeded verifying PoP for wrong pk, shouldn't have!")
	}
}

func TestNonCompositeSig(t *testing.T) {
	privateKey, _ := GeneratePrivateKey()
	defer privateKey.Destroy()
	publicKey, _ := privateKey.ToPublic()
	message := []byte("test")
	extraData := []byte("extra")
	signature, _ := privateKey.SignMessage(message, extraData, false)
	err := publicKey.VerifySignature(message, extraData, signature, false)
	if err != nil {
		t.Fatalf("failed verifying signature for pk 1, error was: %s", err)
	}

	privateKey2, _ := GeneratePrivateKey()
	defer privateKey2.Destroy()
	publicKey2, _ := privateKey2.ToPublic()
	signature2, _ := privateKey2.SignMessage(message, extraData, false)
	err = publicKey2.VerifySignature(message, extraData, signature2, false)
	if err != nil {
		t.Fatalf("failed verifying signature for pk 2, error was: %s", err)
	}

	aggergatedPublicKey, _ := AggregatePublicKeys([]*PublicKey{publicKey, publicKey2})
	aggergatedSignature, _ := AggregateSignatures([]*Signature{signature, signature2})
	err = aggergatedPublicKey.VerifySignature(message, extraData, aggergatedSignature, false)
	if err != nil {
		t.Fatalf("failed verifying signature for aggregated pk, error was: %s", err)
	}
	err = publicKey.VerifySignature(message, extraData, aggergatedSignature, false)
	if err == nil {
		t.Fatalf("succeeded verifying signature for wrong pk, shouldn't have!")
	}
}

func TestEncoding(t *testing.T) {
	InitBLSCrypto()
	privateKey, _ := GeneratePrivateKey()
	defer privateKey.Destroy()
	publicKey, _ := privateKey.ToPublic()

	privateKey2, _ := GeneratePrivateKey()
	defer privateKey2.Destroy()
	publicKey2, _ := privateKey2.ToPublic()

	bytes, err := EncodeEpochToBytes(10, 20, []*PublicKey{publicKey, publicKey2})
	if err != nil {
		t.Fatalf("failed encoding epoch bytes")
	}
	t.Logf("encoding: %s\n", hex.EncodeToString(bytes))
	bytesWithApk, err := EncodeEpochToBytesWithAggregatedKey(10, 20, []*PublicKey{publicKey, publicKey2})
	if err != nil {
		t.Fatalf("failed encoding epoch bytes")
	}
	t.Logf("encoding with aggregated public key: %s\n", hex.EncodeToString(bytes))
	if len(bytesWithApk) <= len(bytes) {
		t.Fatalf("encoding with the aggregated public key should be larger")
	}
}

func TestAggregatePublicKeysErrors(t *testing.T) {
	InitBLSCrypto()
	privateKey, _ := GeneratePrivateKey()
	defer privateKey.Destroy()
	publicKey, _ := privateKey.ToPublic()

	_, err := AggregatePublicKeys([]*PublicKey{publicKey, nil})
	if err != NilPointerError {
		t.Fatalf("should have been a nil pointer")
	}
	_, err = AggregatePublicKeys([]*PublicKey{})
	if err != EmptySliceError {
		t.Fatalf("should have been an empty slice")
	}
	_, err = AggregatePublicKeys(nil)
	if err != EmptySliceError {
		t.Fatalf("should have been an empty slice")
	}
}

func TestAggregateSignaturesErrors(t *testing.T) {
	InitBLSCrypto()
	privateKey, _ := GeneratePrivateKey()
	defer privateKey.Destroy()
	message := []byte("test")
	extraData := []byte("extra")
	signature, _ := privateKey.SignMessage(message, extraData, true)

	_, err := AggregateSignatures([]*Signature{signature, nil})
	if err != NilPointerError {
		t.Fatalf("should have been a nil pointer")
	}
	_, err = AggregateSignatures([]*Signature{})
	if err != EmptySliceError {
		t.Fatalf("should have been an empty slice")
	}
	_, err = AggregateSignatures(nil)
	if err != EmptySliceError {
		t.Fatalf("should have been an empty slice")
	}
}

func TestEncodeErrors(t *testing.T) {
	InitBLSCrypto()

	_, err := EncodeEpochToBytes(0, 0, []*PublicKey{})
	if err != EmptySliceError {
		t.Fatalf("should have been an empty slice")
	}
	_, err = EncodeEpochToBytes(0, 0, nil)
	if err != EmptySliceError {
		t.Fatalf("should have been an empty slice")
	}
}

func TestVerifyPoPErrors(t *testing.T) {
	InitBLSCrypto()
	privateKey, _ := GeneratePrivateKey()
	defer privateKey.Destroy()
	publicKey, _ := privateKey.ToPublic()
	message := []byte("test")
	err := publicKey.VerifyPoP(message, nil)
	if err != NilPointerError {
		t.Fatalf("should have been a nil pointer")
	}
}

func TestVerifySignatureErrors(t *testing.T) {
	InitBLSCrypto()
	privateKey, _ := GeneratePrivateKey()
	defer privateKey.Destroy()
	publicKey, _ := privateKey.ToPublic()
	message := []byte("test")
	extraData := []byte("extra")
	err := publicKey.VerifySignature(message, extraData, nil, false)
	if err != NilPointerError {
		t.Fatalf("should have been a nil pointer")
	}

	err = publicKey.VerifySignature(message, extraData, nil, true)
	if err != NilPointerError {
		t.Fatalf("should have been a nil pointer")
	}

}
