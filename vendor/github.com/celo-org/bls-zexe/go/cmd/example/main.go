package main

import (
	"fmt"

	"github.com/celo-org/bls-zexe"
)

func main() {
	privateKey, _ := ultralight.GeneratePrivateKey()
	defer privateKey.Destroy()
	privateKeyBytes, _ := privateKey.Serialize()
	fmt.Printf("Private key: %x\n", privateKeyBytes)
	publicKey, _ := privateKey.ToPublic()
	publicKeyBytes, _ := publicKey.Serialize()
	fmt.Printf("Public key: %x\n", publicKeyBytes)
	message := []byte("test")
	signature, _ := privateKey.SignMessage(message)
	signatureBytes, _ := signature.Serialize()
	fmt.Printf("Signature: %x\n", signatureBytes)
	err := publicKey.VerifySignature(message, signature)
	fmt.Printf("Verified: %t\n", err == nil)

	privateKey2, _ := ultralight.GeneratePrivateKey()
	defer privateKey2.Destroy()
	privateKeyBytes2, _ := privateKey2.Serialize()
	fmt.Printf("Private key 2: %x\n", privateKeyBytes2)
	publicKey2, _ := privateKey2.ToPublic()
	publicKeyBytes2, _ := publicKey2.Serialize()
	fmt.Printf("Public key 2: %x\n", publicKeyBytes2)
	signature2, _ := privateKey2.SignMessage(message)
	signatureBytes2, _ := signature2.Serialize()
	fmt.Printf("Signature 2: %x\n", signatureBytes2)
	err = publicKey2.VerifySignature(message, signature2)
	fmt.Printf("Verified 2: %t\n", err == nil)

	aggergatedPublicKey, _ := ultralight.AggregatePublicKeys([]*ultralight.PublicKey{publicKey, publicKey2})
	aggregatedPublicKeyBytes, _ := aggergatedPublicKey.Serialize()
	fmt.Printf("Aggregated public key: %x\n", aggregatedPublicKeyBytes)
	aggergatedSignature, _ := ultralight.AggregateSignatures([]*ultralight.Signature{signature, signature2})
	aggregatedSignatureBytes, _ := aggergatedSignature.Serialize()
	fmt.Printf("Aggregated signature: %x\n", aggregatedSignatureBytes)
	err = aggergatedPublicKey.VerifySignature(message, aggergatedSignature)
	fmt.Printf("Aggregated verified: %t\n", err == nil)
	err = publicKey.VerifySignature(message, aggergatedSignature)
	fmt.Printf("Aggregated verified (with wrong pk): %t\n", err == nil)
}
