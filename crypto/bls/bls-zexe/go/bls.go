package bls

/*
#include "bls.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

const MODULUS377 = "8444461749428370424248824938781546531375899335154063827935233455917409239041"
const MODULUSBITS = 253
const MODULUSMASK = 31 // == 2**(253-(256-8)) - 1
const PRIVATEKEYBYTES = 32
const PUBLICKEYBYTES = 96
const SIGNATUREBYTES = 48

var GeneralError = errors.New("General error")
var NotVerifiedError = errors.New("Not verified")
var IncorrectSizeError = errors.New("Input had incorrect size")
var NilPointerError = errors.New("Pointer was nil")
var EmptySliceError = errors.New("Slice was empty")

func validatePrivateKey(privateKey []byte) error {
	if len(privateKey) != PRIVATEKEYBYTES {
		return IncorrectSizeError
	}

	return nil
}

func validatePublicKey(publicKey []byte) error {
	if len(publicKey) != PUBLICKEYBYTES {
		return IncorrectSizeError
	}

	return nil
}

func validateSignature(signature []byte) error {
	if len(signature) != SIGNATUREBYTES {
		return IncorrectSizeError
	}

	return nil

}

func sliceToPtr(slice []byte) (*C.uchar, C.int) {
	if len(slice) == 0 {
		return nil, 0
	} else {
		return (*C.uchar)(unsafe.Pointer(&slice[0])), C.int(len(slice))
	}
}

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
	err := validatePrivateKey(privateKeyBytes)
	if err != nil {
		return nil, err
	}

	privateKeyPtr, privateKeyLen := sliceToPtr(privateKeyBytes)

	privateKey := &PrivateKey{}
	success := C.deserialize_private_key(privateKeyPtr, privateKeyLen, &privateKey.ptr)
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

func (self *PrivateKey) SignMessage(message []byte, extraData []byte, shouldUseCompositeHasher bool) (*Signature, error) {
	signature := &Signature{}
	messagePtr, messageLen := sliceToPtr(message)
	extraDataPtr, extraDataLen := sliceToPtr(extraData)

	success := C.sign_message(self.ptr, messagePtr, messageLen, extraDataPtr, extraDataLen, C.bool(shouldUseCompositeHasher), &signature.ptr)
	if !success {
		return nil, GeneralError
	}

	return signature, nil
}

func (self *PrivateKey) SignPoP(message []byte) (*Signature, error) {
	signature := &Signature{}
	messagePtr, messageLen := sliceToPtr(message)
	success := C.sign_pop(self.ptr, messagePtr, messageLen, &signature.ptr)
	if !success {
		return nil, GeneralError
	}

	return signature, nil
}

func HashDirect(message []byte, usePoP bool) ([]byte, error) {
	messagePtr, messageLen := sliceToPtr(message)
	var hashPtr *C.uchar
	var hashLen C.int
	success := C.hash_direct(messagePtr, messageLen, &hashPtr, &hashLen, C.bool(usePoP))
	defer C.free_vec(hashPtr, hashLen)
	if !success {
		return nil, GeneralError
	}
	hash := C.GoBytes(unsafe.Pointer(hashPtr), hashLen)
	return hash, nil
}

func HashComposite(message []byte, extraData []byte) ([]byte, error) {
	messagePtr, messageLen := sliceToPtr(message)
	extraDataPtr, extraDataLen := sliceToPtr(extraData)
	var hashLen C.int
	var hashPtr *C.uchar
	success := C.hash_composite(messagePtr, messageLen, extraDataPtr, extraDataLen, &hashPtr, &hashLen)
	if !success {
		return nil, GeneralError
	}
	hash := C.GoBytes(unsafe.Pointer(hashPtr), hashLen)
	return hash, nil
}

func CompressSignature(signature []byte) ([]byte, error) {
	signaturePtr, signatureLen := sliceToPtr(signature)
	var compressedLen C.int
	var compressedPtr *C.uchar
	success := C.compress_signature(signaturePtr, signatureLen, &compressedPtr, &compressedLen)
	if !success {
		return nil, GeneralError
	}
	compressedSignature := C.GoBytes(unsafe.Pointer(compressedPtr), compressedLen)
	return compressedSignature, nil
}

func CompressPublickey(pubkey []byte) ([]byte, error) {
	pubkeyPtr, pubkeyLen := sliceToPtr(pubkey)
	var compressedLen C.int
	var compressedPtr *C.uchar
	success := C.compress_pubkey(pubkeyPtr, pubkeyLen, &compressedPtr, &compressedLen)
	if !success {
		return nil, GeneralError
	}
	compressedPubkey := C.GoBytes(unsafe.Pointer(compressedPtr), compressedLen)
	return compressedPubkey, nil
}

func (self *PrivateKey) Destroy() {
	C.destroy_private_key(self.ptr)
}

func DeserializePublicKey(publicKeyBytes []byte) (*PublicKey, error) {
	err := validatePublicKey(publicKeyBytes)
	if err != nil {
		return nil, err
	}

	publicKeyPtr, publicKeyLen := sliceToPtr(publicKeyBytes)

	publicKey := &PublicKey{}
	success := C.deserialize_public_key(publicKeyPtr, publicKeyLen, &publicKey.ptr)
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

func (self *PublicKey) VerifySignature(message []byte, extraData []byte, signature *Signature, shouldUseCompositeHasher bool) error {
	var verified C.bool

	if signature == nil {
		return NilPointerError
	}

	messagePtr, messageLen := sliceToPtr(message)
	extraDataPtr, extraDataLen := sliceToPtr(extraData)

	success := C.verify_signature(self.ptr, messagePtr, messageLen, extraDataPtr, extraDataLen, signature.ptr, C.bool(shouldUseCompositeHasher), &verified)
	if !success {
		return GeneralError
	}
	if !verified {
		return NotVerifiedError
	}

	return nil
}

func (self *PublicKey) VerifyPoP(message []byte, signature *Signature) error {
	var verified C.bool

	if signature == nil {
		return NilPointerError
	}

	messagePtr, messageLen := sliceToPtr(message)
	success := C.verify_pop(self.ptr, messagePtr, messageLen, signature.ptr, &verified)
	if !success {
		return GeneralError
	}
	if !verified {
		return NotVerifiedError
	}

	return nil
}

func DeserializeSignature(signatureBytes []byte) (*Signature, error) {
	err := validateSignature(signatureBytes)
	if err != nil {
		return nil, err
	}

	signaturePtr, signatureLen := sliceToPtr(signatureBytes)

	signature := &Signature{}
	success := C.deserialize_signature(signaturePtr, signatureLen, &signature.ptr)
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
	if len(publicKeys) == 0 {
		return nil, EmptySliceError
	}

	publicKeysPtrs := []*C.struct_PublicKey{}
	for _, pk := range publicKeys {
		if pk == nil {
			return nil, NilPointerError
		}
		publicKeysPtrs = append(publicKeysPtrs, pk.ptr)
	}
	aggregatedPublicKey := &PublicKey{}
	success := C.aggregate_public_keys((**C.struct_PublicKey)(unsafe.Pointer(&publicKeysPtrs[0])), C.int(len(publicKeysPtrs)), &aggregatedPublicKey.ptr)
	if !success {
		return nil, GeneralError
	}

	return aggregatedPublicKey, nil
}

func AggregatePublicKeysSubtract(aggregatedPublicKey *PublicKey, publicKeys []*PublicKey) (*PublicKey, error) {
	if aggregatedPublicKey == nil {
		return nil, NilPointerError
	}

	if len(publicKeys) == 0 {
		return nil, EmptySliceError
	}

	publicKeysPtrs := []*C.struct_PublicKey{}
	for _, pk := range publicKeys {
		if pk == nil {
			return nil, NilPointerError
		}
		publicKeysPtrs = append(publicKeysPtrs, pk.ptr)
	}
	subtractedPublicKey := &PublicKey{}
	success := C.aggregate_public_keys_subtract(aggregatedPublicKey.ptr, (**C.struct_PublicKey)(unsafe.Pointer(&publicKeysPtrs[0])), C.int(len(publicKeysPtrs)), &subtractedPublicKey.ptr)
	if !success {
		return nil, GeneralError
	}

	return subtractedPublicKey, nil
}

func AggregateSignatures(signatures []*Signature) (*Signature, error) {
	if len(signatures) == 0 {
		return nil, EmptySliceError
	}

	signaturesPtrs := []*C.struct_Signature{}
	for _, sig := range signatures {
		if sig == nil {
			return nil, NilPointerError
		}
		signaturesPtrs = append(signaturesPtrs, sig.ptr)
	}
	aggregatedSignature := &Signature{}
	success := C.aggregate_signatures((**C.struct_Signature)(unsafe.Pointer(&signaturesPtrs[0])), C.int(len(signaturesPtrs)), &aggregatedSignature.ptr)
	if !success {
		return nil, GeneralError
	}

	return aggregatedSignature, nil
}

func encodeEpochToBytes(epochIndex uint16, maximumNonSigners uint32, addedPublicKeys []*PublicKey, shouldEncodeAggregatedPublicKey bool) ([]byte, error) {
	if len(addedPublicKeys) == 0 {
		return nil, EmptySliceError
	}

	publicKeysPtrs := []*C.struct_PublicKey{}
	for _, pk := range addedPublicKeys {
		if pk == nil {
			return nil, NilPointerError
		}
		publicKeysPtrs = append(publicKeysPtrs, pk.ptr)
	}
	var bytes *C.uchar
	var size C.int
	success := C.encode_epoch_block_to_bytes(C.ushort(epochIndex), C.uint(maximumNonSigners), (**C.struct_PublicKey)(unsafe.Pointer(&publicKeysPtrs[0])), C.int(len(publicKeysPtrs)), C.bool(shouldEncodeAggregatedPublicKey), &bytes, &size)
	if !success {
		return nil, GeneralError
	}
	defer C.free_vec(bytes, size)

	return C.GoBytes(unsafe.Pointer(bytes), size), nil
}

func EncodeEpochToBytes(epochIndex uint16, maximumNonSigners uint32, addedPublicKeys []*PublicKey) ([]byte, error) {
	return encodeEpochToBytes(epochIndex, maximumNonSigners, addedPublicKeys, false)
}

func EncodeEpochToBytesWithAggregatedKey(epochIndex uint16, maximumNonSigners uint32, addedPublicKeys []*PublicKey) ([]byte, error) {
	return encodeEpochToBytes(epochIndex, maximumNonSigners, addedPublicKeys, true)
}
