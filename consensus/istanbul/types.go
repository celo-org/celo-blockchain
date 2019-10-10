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

	EncodeRLP(w io.Writer) error

	DecodeRLP(s *rlp.Stream) error
}

type Request struct {
	Proposal Proposal
}

// View includes a round number and a sequence number.
// Sequence is the block number we'd like to commit.
// Each round has a number and is composed by 3 steps: preprepare, prepare and commit.
//
// If the given block is not accepted by validators, a round change will occur
// and the validators start a new round with round+1.
type View struct {
	Round    *big.Int
	Sequence *big.Int
}

// EncodeRLP serializes b into the Ethereum RLP format.
func (v *View) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{v.Round, v.Sequence})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (v *View) DecodeRLP(s *rlp.Stream) error {
	var view struct {
		Round    *big.Int
		Sequence *big.Int
	}

	if err := s.Decode(&view); err != nil {
		return err
	}
	v.Round, v.Sequence = view.Round, view.Sequence
	return nil
}

func (v *View) String() string {
	return fmt.Sprintf("{Round: %d, Sequence: %d}", v.Round.Uint64(), v.Sequence.Uint64())
}

// Cmp compares v and y and returns:
//   -1 if v <  y
//    0 if v == y
//   +1 if v >  y
func (v *View) Cmp(y *View) int {
	if v.Sequence.Cmp(y.Sequence) != 0 {
		return v.Sequence.Cmp(y.Sequence)
	}
	if v.Round.Cmp(y.Round) != 0 {
		return v.Round.Cmp(y.Round)
	}
	return 0
}

type RoundChangeCertificate struct {
	RoundChangeMessages []Message
}

func (b *RoundChangeCertificate) IsEmpty() bool {
	return len(b.RoundChangeMessages) == 0
}

// EncodeRLP serializes b into the Ethereum RLP format.
func (b *RoundChangeCertificate) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{b.RoundChangeMessages})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (b *RoundChangeCertificate) DecodeRLP(s *rlp.Stream) error {
	var roundChangeCertificate struct {
		RoundChangeMessages []Message
	}

	if err := s.Decode(&roundChangeCertificate); err != nil {
		return err
	}
	b.RoundChangeMessages = roundChangeCertificate.RoundChangeMessages

	return nil
}

type Preprepare struct {
	View                   *View
	Proposal               Proposal
	RoundChangeCertificate RoundChangeCertificate
}

func (b *Preprepare) HasRoundChangeCertificate() bool {
	return !b.RoundChangeCertificate.IsEmpty()
}

// EncodeRLP serializes b into the Ethereum RLP format.
func (b *Preprepare) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{b.View, b.Proposal, &b.RoundChangeCertificate})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (b *Preprepare) DecodeRLP(s *rlp.Stream) error {
	var preprepare struct {
		View                   *View
		Proposal               *types.Block
		RoundChangeCertificate RoundChangeCertificate
	}

	if err := s.Decode(&preprepare); err != nil {
		return err
	}
	b.View, b.Proposal, b.RoundChangeCertificate = preprepare.View, preprepare.Proposal, preprepare.RoundChangeCertificate

	return nil
}

type PreparedCertificate struct {
	Proposal                Proposal
	PrepareOrCommitMessages []Message
}

func EmptyPreparedCertificate() PreparedCertificate {
	emptyHeader := &types.Header{
		Difficulty: big.NewInt(0),
		Number:     big.NewInt(0),
		GasLimit:   0,
		GasUsed:    0,
		Time:       big.NewInt(0),
	}
	block := &types.Block{}
	block = block.WithRandomness(&types.EmptyRandomness)

	return PreparedCertificate{
		Proposal:                block.WithSeal(emptyHeader),
		PrepareOrCommitMessages: []Message{},
	}
}

func (b *PreparedCertificate) IsEmpty() bool {
	return len(b.PrepareOrCommitMessages) == 0
}

func (b *PreparedCertificate) View() *View {
	if b.IsEmpty() {
		return nil
	}
	msg := b.PrepareOrCommitMessages[0]
	var s *Subject
	err := msg.Decode(&s)
	if err != nil {
		return nil
	}
	return s.View
}

// EncodeRLP serializes b into the Ethereum RLP format.
func (b *PreparedCertificate) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{b.Proposal, b.PrepareOrCommitMessages})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (b *PreparedCertificate) DecodeRLP(s *rlp.Stream) error {
	var preparedCertificate struct {
		Proposal                *types.Block
		PrepareOrCommitMessages []Message
	}

	if err := s.Decode(&preparedCertificate); err != nil {
		return err
	}

	b.Proposal, b.PrepareOrCommitMessages = preparedCertificate.Proposal, preparedCertificate.PrepareOrCommitMessages
	return nil
}

type RoundChange struct {
	View                *View
	PreparedCertificate PreparedCertificate
}

func (b *RoundChange) HasPreparedCertificate() bool {
	return !b.PreparedCertificate.IsEmpty()
}

// EncodeRLP serializes b into the Ethereum RLP format.
func (b *RoundChange) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{b.View, &b.PreparedCertificate})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (b *RoundChange) DecodeRLP(s *rlp.Stream) error {
	var roundChange struct {
		View                *View
		PreparedCertificate PreparedCertificate
	}

	if err := s.Decode(&roundChange); err != nil {
		return err
	}
	b.View, b.PreparedCertificate = roundChange.View, roundChange.PreparedCertificate
	return nil
}

type Subject struct {
	View   *View
	Digest common.Hash
}

// EncodeRLP serializes b into the Ethereum RLP format.
func (b *Subject) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{b.View, b.Digest})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (b *Subject) DecodeRLP(s *rlp.Stream) error {
	var subject struct {
		View   *View
		Digest common.Hash
	}

	if err := s.Decode(&subject); err != nil {
		return err
	}
	b.View, b.Digest = subject.View, subject.Digest
	return nil
}

func (b *Subject) String() string {
	return fmt.Sprintf("{View: %v, Digest: %v}", b.View, b.Digest.String())
}

const (
	MsgPreprepare uint64 = iota
	MsgPrepare
	MsgCommit
	MsgRoundChange
)

type Message struct {
	Code          uint64
	Msg           []byte
	Address       common.Address
	Signature     []byte
	CommittedSeal []byte
	DestAddresses []common.Address // This is non nil for consensus messages sent from a proxied validator
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes m into the Ethereum RLP format.
func (m *Message) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{m.Code, m.Msg, m.Address, m.Signature, m.CommittedSeal, m.DestAddresses})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (m *Message) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Code          uint64
		Msg           []byte
		Address       common.Address
		Signature     []byte
		CommittedSeal []byte
		DestAddresses []common.Address
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	m.Code, m.Msg, m.Address, m.Signature, m.CommittedSeal, m.DestAddresses = msg.Code, msg.Msg, msg.Address, msg.Signature, msg.CommittedSeal, m.DestAddresses
	return nil
}

// ==============================================
//
// define the functions that needs to be provided for core.

func (m *Message) Sign(signingFn func(data []byte) ([]byte, error)) error {
	// Construct and encode a message with no signature
	payloadNoSig, err := m.PayloadNoSigAndDestAddrs()
	if err != nil {
		return err
	}
	m.Signature, err = signingFn(payloadNoSig)
	return err
}

func (m *Message) FromPayload(b []byte, validateFn func([]byte, []byte) (common.Address, error)) error {
	// Decode Message
	err := rlp.DecodeBytes(b, &m)
	if err != nil {
		return err
	}

	// Validate message (on a message without Signature)
	if validateFn != nil {
		var payload []byte
		payload, err = m.PayloadNoSigAndDestAddrs()
		if err != nil {
			return err
		}

		signed_val_addr, err := validateFn(payload, m.Signature)
		if err != nil {
			return err
		}
		if signed_val_addr != m.Address {
			return ErrInvalidSigner
		}
	}
	return nil
}

func (m *Message) Payload() ([]byte, error) {
	return rlp.EncodeToBytes(m)
}

func (m *Message) PayloadNoSigAndDestAddrs() ([]byte, error) {
	return rlp.EncodeToBytes(&Message{
		Code:          m.Code,
		Msg:           m.Msg,
		Address:       m.Address,
		Signature:     []byte{},
		CommittedSeal: m.CommittedSeal,
		// Don't include the DestAddresses in the payload to sign, since the sentry will need to
		// set it to an empty array when sending it off to other validators/sentries
		DestAddresses: []common.Address{},
	})
}

func (m *Message) Decode(val interface{}) error {
	return rlp.DecodeBytes(m.Msg, val)
}

func (m *Message) String() string {
	return fmt.Sprintf("{Code: %v, Address: %v}", m.Code, m.Address.String())
}
