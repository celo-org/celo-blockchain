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

	"github.com/ethereum/go-ethereum/rlp"
)

// PlumoProofEpochs holds a proof's epoch range, which is then used as a key for lookups
type PlumoProofEpochs struct {
	FirstEpoch uint
	LastEpoch  uint
}

// EncodeRLP serializes p into the Ethereum RLP format.
func (p *PlumoProofEpochs) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		p.FirstEpoch,
		p.LastEpoch,
	})
}

// DecodeRLP implements rlp.Decoder, and loads the plumo proof epoch fields from a RLP stream.
func (p *PlumoProofEpochs) DecodeRLP(s *rlp.Stream) error {
	var plumoProofEpochs struct {
		FirstEpoch uint
		LastEpoch  uint
	}
	if err := s.Decode(&plumoProofEpochs); err != nil {
		return err
	}
	p.FirstEpoch, p.LastEpoch = plumoProofEpochs.FirstEpoch, plumoProofEpochs.LastEpoch
	return nil
}

func (p *PlumoProofEpochs) String() string {
	return fmt.Sprintf("{firstEpoch: %d, lastEpoch: %d}", p.FirstEpoch, p.LastEpoch)
}

// PlumoProof encapsulates a serialized plumo proof and the epochs it operates over
type PlumoProof struct {
	Proof  []byte
	Epochs PlumoProofEpochs
}

// EncodeRLP serializes p into the Ethereum RLP format.
func (p *PlumoProof) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		p.Proof,
		&p.Epochs,
	})
}

// DecodeRLP implements rlp.Decoder, and loads the plumo proof fields from a RLP stream.
func (p *PlumoProof) DecodeRLP(s *rlp.Stream) error {
	var plumoProof struct {
		Proof  []byte
		Epochs PlumoProofEpochs
	}
	if err := s.Decode(&plumoProof); err != nil {
		return err
	}
	p.Proof, p.Epochs = plumoProof.Proof, plumoProof.Epochs
	return nil
}

func (p *PlumoProof) String() string {
	return fmt.Sprintf("{epochs: %s, proof: %x}", p.Epochs.String(), p.Proof)
}
