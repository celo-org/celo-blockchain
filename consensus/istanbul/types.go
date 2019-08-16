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

package istanbul

import (
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// Proposal supports retrieving height and serialized block to be used during Istanbul consensus.
type Proposal interface {
	// Number retrieves the sequence number of this proposal.
	Number() *big.Int

	// Hash retrieves the hash of this proposal.
	Hash() common.Hash

	// ParentHash retrieves the hash of the parent of the proposal.
	ParentHash() common.Hash

	EncodeRLP(w io.Writer) error

	DecodeRLP(s *rlp.Stream) error
}

type Request struct {
	Proposal Proposal
}

// Aggregated certificate for whatever is the hash.
type QuorumCertificate struct {
	BlockHash   common.Hash
	Bitmap      *big.Int
	Certificate []byte
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes m into the Ethereum RLP format.
func (qc *QuorumCertificate) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{m.BlockHash, m.Bitmap, m.Certificate})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (qc *QuorumCertificate) DecodeRLP(s *rlp.Stream) error {
	var cert struct {
		BlockHash   common.Hash  
		Bitmap      *big.Int
		Certificate []byte
	}

	if err := s.Decode(&cert); err != nil {
		return err
	}
	qc.BlockHash, qc.Bitmap, qc.Certificate = cert.BlockHash, cert.Bitmap, cert.Certificate
	return nil
}

// Responds to MsgPropose/NewBlock with a partial signature if accepted to the next proposer
type Vote struct {
	Block      Proposal
	PartialSig []byte
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes m into the Ethereum RLP format.
func (v *Vote) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{v.Proposal, v.PartialSig})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (v *Vote) DecodeRLP(s *rlp.Stream) error {
	var vote struct {
		Proposal   Proposal
		PartialSig []byte
	}

	if err := s.Decode(&vote); err != nil {
		return err
	}
	v.Proposal, v.PartialSig = vote.Proposal, vote.PartialSig
	return nil
}

// Proposal message with the new block and a QC for the highest seen block
type Node struct {
	Block             Proposal
	QuorumCertificate QuorumCertificate
}


// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes m into the Ethereum RLP format.
func (n *Node) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{n.Proposal, n.QuorumCertificate})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (n *Node) DecodeRLP(s *rlp.Stream) error {
	var nn struct {
		Proposal          Proposal
		QuorumCertificate QuorumCertificate
	}

	if err := s.Decode(&proposal); err != nil {
		return err
	}
	n.Proposal, n.QuorumCertificate = nn.Proposal, nn.QuorumCertificate
	return nil
}


const (
	MsgPropose uint64 = iota
	MsgVote
	MsgNewView
)

// Messages must be signed & include current view number
type Message struct {
	Code          uint64
	Number        *big.Int
	Msg           []byte
	Address       common.Address
	Signature     []byte
}



// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes m into the Ethereum RLP format.
func (m *Message) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{m.Code, m.Msg, m.Address, m.Signature, m.CommittedSeal})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (m *Message) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Code          uint64
		Msg           []byte
		Address       common.Address
		Signature     []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	m.Code, m.Msg, m.Address, m.Signature = msg.Code, msg.Msg, msg.Address, msg.Signature
	return nil
}

// ==============================================
//
// define the functions that needs to be provided for core.

func (m *Message) FromPayload(b []byte, validateFn func([]byte, []byte) (common.Address, error)) error {
	// Decode message
	err := rlp.DecodeBytes(b, &m)
	if err != nil {
		return err
	}

	// Validate message (on a message without Signature)
	if validateFn != nil {
		var payload []byte
		payload, err = m.PayloadNoSig()
		if err != nil {
			return err
		}

		_, err = validateFn(payload, m.Signature)
	}
	// Still return the message even the err is not nil
	return err
}

func (m *Message) Payload() ([]byte, error) {
	return rlp.EncodeToBytes(m)
}

func (m *Message) PayloadNoSig() ([]byte, error) {
	return rlp.EncodeToBytes(&Message{
		Code:          m.Code,
		Msg:           m.Msg,
		Address:       m.Address,
		Signature:     []byte{},
	})
}

func (m *Message) Decode(val interface{}) error {
	return rlp.DecodeBytes(m.Msg, val)
}

func (m *Message) String() string {
	return fmt.Sprintf("{Code: %v, Address: %v}", m.Code, m.Address.String())
}
