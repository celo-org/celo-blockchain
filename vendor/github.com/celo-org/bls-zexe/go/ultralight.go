package ultralight

/*
#cgo LDFLAGS: -L../ultralight/target/release -lbls_zexe -ldl -lm
#include "ultralight.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

var GeneralError = errors.New("General error")
var NotVerifiedError = errors.New("Not verified")

type PrivateKey struct {
	ptr *C.struct_PrivateKey
}

type PublicKey struct {
	ptr *C.struct_PublicKey
}

type Signature struct {
	ptr *C.struct_Signature
}

func InitBLSCrypto() {
	C.init()
}

func GeneratePrivateKey() (*PrivateKey, error) {
	privateKey := &PrivateKey{}
	success := C.generate_private_key(&privateKey.ptr)
	if !success {
		return nil, GeneralError
	}

	return privateKey, nil
}

func DeserializePrivateKey(privateKeyBytes []byte) (*PrivateKey, error) {
	privateKey := &PrivateKey{}
	success := C.deserialize_private_key((*C.uchar)(unsafe.Pointer(&privateKeyBytes[0])), C.int(len(privateKeyBytes)), &privateKey.ptr)
	if !success {
		return nil, GeneralError
	}

	return privateKey, nil
}

func (self *PrivateKey) Serialize() ([]byte, error) {
	var bytes *C.uchar
	var size C.int
	success := C.serialize_private_key(self.ptr, &bytes, &size)
	defer C.free_vec(bytes, size)
	if !success {
		return nil, GeneralError
	}

	return C.GoBytes(unsafe.Pointer(bytes), size), nil
}

func (self *PrivateKey) ToPublic() (*PublicKey, error) {
	publicKey := &PublicKey{}
	success := C.private_key_to_public_key(self.ptr, &publicKey.ptr)
	if !success {
		return nil, GeneralError
	}

	return publicKey, nil
}

func (self *PrivateKey) SignMessage(message []byte) (*Signature, error) {
	signature := &Signature{}
	success := C.sign_message(self.ptr, (*C.uchar)(unsafe.Pointer(&message[0])), C.int(len(message)), &signature.ptr)
	if !success {
		return nil, GeneralError
	}

	return signature, nil
}

func (self *PrivateKey) Destroy() {
	C.destroy_private_key(self.ptr)
}

func DeserializePublicKey(publicKeyBytes []byte) (*PublicKey, error) {
	publicKey := &PublicKey{}
	success := C.deserialize_public_key((*C.uchar)(unsafe.Pointer(&publicKeyBytes[0])), C.int(len(publicKeyBytes)), &publicKey.ptr)
	if !success {
		return nil, GeneralError
	}

	return publicKey, nil
}

func (self *PublicKey) Serialize() ([]byte, error) {
	var bytes *C.uchar
	var size C.int
	success := C.serialize_public_key(self.ptr, &bytes, &size)
	defer C.free_vec(bytes, size)
	if !success {
		return nil, GeneralError
	}

	return C.GoBytes(unsafe.Pointer(bytes), size), nil
}

func (self *PublicKey) Destroy() {
	C.destroy_public_key(self.ptr)
}

func (self *PublicKey) VerifySignature(message []byte, signature *Signature) error {
	var verified C.bool
	success := C.verify_signature(self.ptr, (*C.uchar)(unsafe.Pointer(&message[0])), C.int(len(message)), signature.ptr, &verified)
	if !success {
		return GeneralError
	}
	if !verified {
		return NotVerifiedError
	}

	return nil
}

func DeserializeSignature(signatureBytes []byte) (*Signature, error) {
	signature := &Signature{}
	success := C.deserialize_signature((*C.uchar)(unsafe.Pointer(&signatureBytes[0])), C.int(len(signatureBytes)), &signature.ptr)
	if !success {
		return nil, GeneralError
	}
	return signature, nil
}

func (self *Signature) Serialize() ([]byte, error) {
	var bytes *C.uchar
	var size C.int
	success := C.serialize_signature(self.ptr, &bytes, &size)
	defer C.free_vec(bytes, size)
	if !success {
		return nil, GeneralError
	}

	return C.GoBytes(unsafe.Pointer(bytes), size), nil
}

func (self *Signature) Destroy() {
	C.destroy_signature(self.ptr)
}

func AggregatePublicKeys(publicKeys []*PublicKey) (*PublicKey, error) {
	publicKeysPtrs := []*C.struct_PublicKey{}
	for _, pk := range publicKeys {
		publicKeysPtrs = append(publicKeysPtrs, pk.ptr)
	}
	aggregatedPublicKey := &PublicKey{}
	success := C.aggregate_public_keys((**C.struct_PublicKey)(unsafe.Pointer(&publicKeysPtrs[0])), C.int(len(publicKeysPtrs)), &aggregatedPublicKey.ptr)
	if !success {
		return nil, GeneralError
	}

	return aggregatedPublicKey, nil
}

func AggregateSignatures(signatures []*Signature) (*Signature, error) {
	signaturesPtrs := []*C.struct_Signature{}
	for _, pk := range signatures {
		signaturesPtrs = append(signaturesPtrs, pk.ptr)
	}
	aggregatedSignature := &Signature{}
	success := C.aggregate_signatures((**C.struct_Signature)(unsafe.Pointer(&signaturesPtrs[0])), C.int(len(signaturesPtrs)), &aggregatedSignature.ptr)
	if !success {
		return nil, GeneralError
	}

	return aggregatedSignature, nil
}
