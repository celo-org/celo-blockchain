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
	"fmt"
	"io"

	"github.com/celo-org/celo-blockchain/rlp"
)

// PlumoProofMetadata holds a proof's epoch range and proof version number, which is then used as a key for lookups
// TODO: do these need to be uints for encoding? lots of casting being done
type PlumoProofMetadata struct {
	FirstEpoch    uint
	LastEpoch     uint
	VersionNumber uint
}

// EncodeRLP serializes p into the Ethereum RLP format.
func (p *PlumoProofMetadata) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		p.FirstEpoch,
		p.LastEpoch,
		p.VersionNumber,
	})
}

// DecodeRLP implements rlp.Decoder, and loads the plumo proof epoch fields from a RLP stream.
func (p *PlumoProofMetadata) DecodeRLP(s *rlp.Stream) error {
	var plumoProofMetadata struct {
		FirstEpoch    uint
		LastEpoch     uint
		VersionNumber uint
	}
	if err := s.Decode(&plumoProofMetadata); err != nil {
		return err
	}
	p.FirstEpoch, p.LastEpoch, p.VersionNumber = plumoProofMetadata.FirstEpoch, plumoProofMetadata.LastEpoch, plumoProofMetadata.VersionNumber
	return nil
}

func (p *PlumoProofMetadata) String() string {
	return fmt.Sprintf("{firstEpoch: %d, lastEpoch: %d, versionNumber: %d}", p.FirstEpoch, p.LastEpoch, p.VersionNumber)
}

// PlumoProof encapsulates a serialized plumo proof and the epochs it operates over
type PlumoProof struct {
	Proof    []byte
	Metadata PlumoProofMetadata
}

// EncodeRLP serializes p into the Ethereum RLP format.
func (p *PlumoProof) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		p.Proof,
		&p.Metadata,
	})
}

// DecodeRLP implements rlp.Decoder, and loads the plumo proof fields from a RLP stream.
func (p *PlumoProof) DecodeRLP(s *rlp.Stream) error {
	var plumoProof struct {
		Proof    []byte
		Metadata PlumoProofMetadata
	}
	if err := s.Decode(&plumoProof); err != nil {
		return err
	}
	p.Proof, p.Metadata = plumoProof.Proof, plumoProof.Metadata
	return nil
}

func (p *PlumoProof) String() string {
	return fmt.Sprintf("{metadata: %s, proof: %x}", p.Metadata.String(), p.Proof)
}

type PlumoProofs []*PlumoProof
