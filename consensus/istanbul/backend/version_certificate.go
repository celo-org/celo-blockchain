// Copyright 2021 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package backend

import (
	"crypto/ecdsa"
	"io"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	vet "github.com/celo-org/celo-blockchain/consensus/istanbul/backend/internal/enodes"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/rlp"
)

// Used as a salt when signing versionCertificate. This is to account for
// the unlikely case where a different signed struct with the same field types
// is used elsewhere and shared with other nodes. If that were to happen, a
// malicious node could try sending the other struct where this struct is used,
// or vice versa. This ensures that the signature is only valid for this struct.
var versionCertificateSalt = []byte("versionCertificate")

// versionCertificate is a signed message from a validator indicating the most
// recent version of its enode.
type versionCertificate vet.VersionCertificateEntry

type vcSignFn func(data []byte) ([]byte, error)

func newVersionCertificateFromEntry(entry *vet.VersionCertificateEntry) *versionCertificate {
	return &versionCertificate{
		Address:   entry.Address,
		PublicKey: entry.PublicKey,
		Version:   entry.Version,
		Signature: entry.Signature,
	}
}

func (vc *versionCertificate) Sign(signingFn vcSignFn) error {
	payloadToSign, err := vc.payloadToSign()
	if err != nil {
		return err
	}
	vc.Signature, err = signingFn(payloadToSign)
	if err != nil {
		return err
	}
	return nil
}

// RecoverPublicKeyAndAddress recovers the ECDSA public key and corresponding
// address from the Signature
func (vc *versionCertificate) RecoverPublicKeyAndAddress() error {
	payloadToSign, err := vc.payloadToSign()
	if err != nil {
		return err
	}
	payloadHash := crypto.Keccak256(payloadToSign)
	publicKey, err := crypto.SigToPub(payloadHash, vc.Signature)
	if err != nil {
		return err
	}
	address, err := crypto.PubkeyToAddress(*publicKey), nil
	if err != nil {
		return err
	}
	vc.PublicKey = publicKey
	vc.Address = address
	return nil
}

// EncodeRLP serializes versionCertificate into the Ethereum RLP format.
// Only the Version and Signature are encoded, as the public key and address
// can be recovered from the Signature using RecoverPublicKeyAndAddress
func (vc *versionCertificate) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{vc.Version, vc.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the versionCertificate fields from a RLP stream.
// Only the Version and Signature are encoded/decoded, as the public key and address
// can be recovered from the Signature using RecoverPublicKeyAndAddress
func (vc *versionCertificate) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Version   uint
		Signature []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	vc.Version, vc.Signature = msg.Version, msg.Signature
	return nil
}

func (vc *versionCertificate) Entry() *vet.VersionCertificateEntry {
	return &vet.VersionCertificateEntry{
		Address:   vc.Address,
		PublicKey: vc.PublicKey,
		Version:   vc.Version,
		Signature: vc.Signature,
	}
}

func (vc *versionCertificate) payloadToSign() ([]byte, error) {
	signedContent := []interface{}{versionCertificateSalt, vc.Version}
	payload, err := rlp.EncodeToBytes(signedContent)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func encodeVersionCertificatesMsg(versionCertificates []*versionCertificate) ([]byte, error) {
	payload, err := rlp.EncodeToBytes(versionCertificates)
	if err != nil {
		return nil, err
	}
	msg := &istanbul.Message{
		Code: istanbul.VersionCertificatesMsg,
		Msg:  payload,
	}
	msgPayload, err := msg.Payload()
	if err != nil {
		return nil, err
	}
	return msgPayload, nil
}

func generateVersionCertificate(address common.Address, publicKey *ecdsa.PublicKey, version uint, signFn vcSignFn) (*versionCertificate, error) {
	vc := &versionCertificate{
		Address:   address,
		PublicKey: publicKey,
		Version:   version,
	}
	err := vc.Sign(signFn)
	if err != nil {
		return nil, err
	}
	return vc, nil
}

func fromVCEntries(allEntries []*vet.VersionCertificateEntry) []*versionCertificate {
	allVersionCertificates := make([]*versionCertificate, len(allEntries))
	for i, entry := range allEntries {
		allVersionCertificates[i] = newVersionCertificateFromEntry(entry)
	}
	return allVersionCertificates
}
