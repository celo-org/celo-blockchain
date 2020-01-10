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

// UptimeEntry contains the uptime score of a validator during an epoch as well as the
// last block they signed on
type UptimeEntry struct {
	ScoreTally      uint64
	LastSignedBlock uint64
}

func (u *UptimeEntry) String() string {
	return fmt.Sprintf("UptimeEntry { scoreTally: %v, lastBlock: %v}", u.ScoreTally, u.LastSignedBlock)
}

// Uptime contains the latest block for which uptime metrics were accounted for. It also contains
// an array of Entries where the `i`th entry represents the uptime statistics of the `i`th validator
// in the validator set for that epoch
type Uptime struct {
	LatestBlock uint64
	Entries     []UptimeEntry
}

// Proposal supports retrieving height and serialized block to be used during Istanbul consensus.
type Proposal interface {
	// Number retrieves the sequence number of this proposal.
	Number() *big.Int

	Header() *types.Header

	// Hash retrieves the hash of this block
	Hash() common.Hash

	// ParentHash retrieves the hash of this block's parent
	ParentHash() common.Hash

	EncodeRLP(w io.Writer) error

	DecodeRLP(s *rlp.Stream) error
}

// ## Request ##############################################################

type Request struct {
	Proposal Proposal
}

// EncodeRLP serializes b into the Ethereum RLP format.
func (b *Request) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{b.Proposal})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (b *Request) DecodeRLP(s *rlp.Stream) error {
	var request struct {
		Proposal *types.Block
	}

	if err := s.Decode(&request); err != nil {
		return err
	}

	b.Proposal = request.Proposal
	return nil
}

// ## View ##############################################################

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

func (v *View) String() string {
	if v.Round == nil || v.Sequence == nil {
		return "Invalid"
	}
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

// ## RoundChangeCertificate ##############################################################

type RoundChangeCertificate struct {
	RoundChangeMessages []Message
}

func (b *RoundChangeCertificate) IsEmpty() bool {
	return len(b.RoundChangeMessages) == 0
}

// ## Preprepare ##############################################################

type Preprepare struct {
	View                   *View
	Proposal               Proposal
	RoundChangeCertificate RoundChangeCertificate
}

type PreprepareData struct {
	View                   *View
	Proposal               *types.Block
	RoundChangeCertificate RoundChangeCertificate
}

type PreprepareSummary struct {
	View                          *View            `json:"view"`
	ProposalHash                  common.Hash      `json:"proposalHash"`
	RoundChangeCertificateSenders []common.Address `json:"roundChangeCertificateSenders"`
}

func (pp *Preprepare) HasRoundChangeCertificate() bool {
	return !pp.RoundChangeCertificate.IsEmpty()
}

func (pp *Preprepare) AsData() *PreprepareData {
	return &PreprepareData{
		View:                   pp.View,
		Proposal:               pp.Proposal.(*types.Block),
		RoundChangeCertificate: pp.RoundChangeCertificate,
	}
}

func (pp *Preprepare) Summary() *PreprepareSummary {
	return &PreprepareSummary{
		View:                          pp.View,
		ProposalHash:                  pp.Proposal.Hash(),
		RoundChangeCertificateSenders: MapMessagesToSenders(pp.RoundChangeCertificate.RoundChangeMessages),
	}
}

// RLP Encoding ---------------------------------------------------------------

// EncodeRLP serializes b into the Ethereum RLP format.
func (pp *Preprepare) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, pp.AsData())
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (pp *Preprepare) DecodeRLP(s *rlp.Stream) error {
	var data PreprepareData
	if err := s.Decode(&data); err != nil {
		return err
	}
	pp.View, pp.Proposal, pp.RoundChangeCertificate = data.View, data.Proposal, data.RoundChangeCertificate
	return nil
}

// ## PreparedCertificate #####################################################

type PreparedCertificate struct {
	Proposal                Proposal
	PrepareOrCommitMessages []Message
}

type PreparedCertificateData struct {
	Proposal                *types.Block
	PrepareOrCommitMessages []Message
}

type PreparedCertificateSummary struct {
	ProposalHash   common.Hash      `json:"proposalHash"`
	PrepareSenders []common.Address `json:"prepareSenders"`
	CommitSenders  []common.Address `json:"commitSenders"`
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

func (pc *PreparedCertificate) IsEmpty() bool {
	return len(pc.PrepareOrCommitMessages) == 0
}

func (pc *PreparedCertificate) AsData() *PreparedCertificateData {
	return &PreparedCertificateData{
		Proposal:                pc.Proposal.(*types.Block),
		PrepareOrCommitMessages: pc.PrepareOrCommitMessages,
	}
}

func (pc *PreparedCertificate) Summary() *PreparedCertificateSummary {
	var prepareSenders, commitSenders []common.Address
	for _, msg := range pc.PrepareOrCommitMessages {
		if msg.Code == MsgPrepare {
			prepareSenders = append(prepareSenders, msg.Address)
		} else {
			commitSenders = append(commitSenders, msg.Address)
		}
	}

	return &PreparedCertificateSummary{
		ProposalHash:   pc.Proposal.Hash(),
		PrepareSenders: prepareSenders,
		CommitSenders:  commitSenders,
	}
}

// RLP Encoding ---------------------------------------------------------------

// EncodeRLP serializes b into the Ethereum RLP format.
func (pc *PreparedCertificate) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, pc.AsData())
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (pc *PreparedCertificate) DecodeRLP(s *rlp.Stream) error {
	var data PreparedCertificateData
	if err := s.Decode(&data); err != nil {
		return err
	}
	pc.PrepareOrCommitMessages, pc.Proposal = data.PrepareOrCommitMessages, data.Proposal
	return nil

}

// ## RoundChange #############################################################

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

// ## Subject #################################################################

type Subject struct {
	View   *View
	Digest common.Hash
}

func (s *Subject) String() string {
	return fmt.Sprintf("{View: %v, Digest: %v}", s.View, s.Digest.String())
}

// ## CommittedSubject #################################################################

type CommittedSubject struct {
	Subject       *Subject
	CommittedSeal []byte
}

// ## ForwardMessage #################################################################

type ForwardMessage struct {
	Msg           []byte
	DestAddresses []common.Address
}

// ## Message #################################################################

const (
	MsgPreprepare uint64 = iota
	MsgPrepare
	MsgCommit
	MsgRoundChange
)

type Message struct {
	Code      uint64
	Msg       []byte
	Address   common.Address // The sender address
	Signature []byte         // Signature of the Message using the private key associated with the "Address" field
}

// define the functions that needs to be provided for core.

func (m *Message) Sign(signingFn func(data []byte) ([]byte, error)) error {
	// Construct and encode a message with no signature
	payloadNoSig, err := m.PayloadNoSig()
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
		payload, err = m.PayloadNoSig()
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

func (m *Message) PayloadNoSig() ([]byte, error) {
	return rlp.EncodeToBytes(&Message{
		Code:      m.Code,
		Msg:       m.Msg,
		Address:   m.Address,
		Signature: []byte{},
	})
}

func (m *Message) Decode(val interface{}) error {
	return rlp.DecodeBytes(m.Msg, val)
}

func (m *Message) String() string {
	return fmt.Sprintf("{Code: %v, Address: %v}", m.Code, m.Address.String())
}

// MapMessagesToSenders map a list of Messages to the list of the sender addresses
func MapMessagesToSenders(messages []Message) []common.Address {
	returnList := make([]common.Address, len(messages))

	for i, ms := range messages {
		returnList[i] = ms.Address
	}

	return returnList
}
