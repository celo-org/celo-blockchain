// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package types

import (
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/rlp"
)

var (
	IstanbulExtraVanity       = 32                       // Fixed number of extra-data bytes reserved for validator vanity
	IstanbulExtraBlsSignature = blscrypto.SIGNATUREBYTES // Fixed number of extra-data bytes reserved for validator seal on the current block
	IstanbulExtraSeal         = 65                       // Fixed number of extra-data bytes reserved for validator seal

	// ErrInvalidIstanbulHeaderExtra is returned if the length of extra-data is less than 32 bytes
	ErrInvalidIstanbulHeaderExtra = errors.New("invalid istanbul header extra-data")
	EmptyBlockSeal                = []byte{}
)

type IstanbulAggregatedSeal struct {
	// Bitmap is a bitmap having an active bit for each validator that signed this block
	Bitmap *big.Int
	// Signature is an aggregated BLS signature resulting from signatures by each validator that signed this block
	Signature []byte
	// Round is the round in which the signature was created.
	Round *big.Int
}

type IstanbulEpochValidatorSetSeal struct {
	// Bitmap is a bitmap having an active bit for each validator that signed this epoch data
	Bitmap *big.Int

	// Signature is an aggregated BLS signature resulting from signatures by each validator that signed this block
	Signature []byte
}

// EncodeRLP serializes ist into the Ethereum RLP format.
func (ist *IstanbulAggregatedSeal) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		ist.Bitmap,
		ist.Signature,
		ist.Round,
	})
}

// DecodeRLP implements rlp.Decoder, and load the istanbul fields from a RLP stream.
func (ist *IstanbulAggregatedSeal) DecodeRLP(s *rlp.Stream) error {
	var istanbulAggregatedSeal struct {
		Bitmap    *big.Int
		Signature []byte
		Round     *big.Int
	}
	if err := s.Decode(&istanbulAggregatedSeal); err != nil {
		return err
	}
	ist.Bitmap, ist.Signature, ist.Round = istanbulAggregatedSeal.Bitmap, istanbulAggregatedSeal.Signature, istanbulAggregatedSeal.Round
	return nil
}

func (ist *IstanbulAggregatedSeal) String() string {
	return fmt.Sprintf("{round: %s, bitmap: %s, signature: %x}", ist.Round.String(), ist.Bitmap.Text(2), ist.Signature)
}

type IstanbulExtra struct {
	// AddedValidators are the validators that have been added in the block
	AddedValidators []common.Address
	// AddedValidatorsPublicKeys are the BLS public keys for the validators added in the block
	AddedValidatorsPublicKeys []blscrypto.SerializedPublicKey
	// RemovedValidators is a bitmap having an active bit for each removed validator in the block
	RemovedValidators *big.Int
	// Seal is an ECDSA signature by the proposer
	Seal []byte
	// AggregatedSeal contains the aggregated BLS signature created via IBFT consensus.
	AggregatedSeal IstanbulAggregatedSeal
	// ParentAggregatedSeal contains and aggregated BLS signature for the previous block.
	ParentAggregatedSeal IstanbulAggregatedSeal
}

// EncodeRLP serializes ist into the Ethereum RLP format.
func (ist *IstanbulExtra) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		ist.AddedValidators,
		ist.AddedValidatorsPublicKeys,
		ist.RemovedValidators,
		ist.Seal,
		&ist.AggregatedSeal,
		&ist.ParentAggregatedSeal,
	})
}

// DecodeRLP implements rlp.Decoder, and load the istanbul fields from a RLP stream.
func (ist *IstanbulExtra) DecodeRLP(s *rlp.Stream) error {
	var istanbulExtra struct {
		AddedValidators           []common.Address
		AddedValidatorsPublicKeys []blscrypto.SerializedPublicKey
		RemovedValidators         *big.Int
		Seal                      []byte
		AggregatedSeal            IstanbulAggregatedSeal
		ParentAggregatedSeal      IstanbulAggregatedSeal
	}
	if err := s.Decode(&istanbulExtra); err != nil {
		return err
	}
	ist.AddedValidators, ist.AddedValidatorsPublicKeys, ist.RemovedValidators, ist.Seal, ist.AggregatedSeal, ist.ParentAggregatedSeal = istanbulExtra.AddedValidators, istanbulExtra.AddedValidatorsPublicKeys, istanbulExtra.RemovedValidators, istanbulExtra.Seal, istanbulExtra.AggregatedSeal, istanbulExtra.ParentAggregatedSeal
	return nil
}

// ExtractIstanbulExtra extracts all values of the IstanbulExtra from the header. It returns an
// error if the length of the given extra-data is less than 32 bytes or the extra-data can not
// be decoded.
func extractIstanbulExtra(h *Header) (*IstanbulExtra, error) {
	if len(h.Extra) < IstanbulExtraVanity {
		return nil, ErrInvalidIstanbulHeaderExtra
	}

	var istanbulExtra *IstanbulExtra
	err := rlp.DecodeBytes(h.Extra[IstanbulExtraVanity:], &istanbulExtra)
	if err != nil {
		return nil, err
	}
	return istanbulExtra, nil
}

// IstanbulFilteredHeader returns a filtered header which some information (like seal, aggregated signature)
// are clean to fulfill the Istanbul hash rules. It returns nil if the extra-data cannot be
// decoded/encoded by rlp.
func IstanbulFilteredHeader(h *Header, keepSeal bool) *Header {
	newHeader := CopyHeader(h)
	istanbulExtra, err := extractIstanbulExtra(newHeader)
	if err != nil {
		return nil
	}

	if !keepSeal {
		istanbulExtra.Seal = []byte{}
	}
	istanbulExtra.AggregatedSeal = IstanbulAggregatedSeal{}

	payload, err := rlp.EncodeToBytes(&istanbulExtra)
	if err != nil {
		return nil
	}

	newHeader.Extra = append(newHeader.Extra[:IstanbulExtraVanity], payload...)

	return newHeader
}
