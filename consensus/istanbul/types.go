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
	"crypto/ecdsa"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/celo-org/celo-blockchain/accounts"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/crypto"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/rlp"
)

// Decrypt is a decrypt callback function to request an ECIES ciphertext to be
// decrypted
type DecryptFn func(accounts.Account, []byte, []byte, []byte) ([]byte, error)

// SignerFn is a signer callback function to request a header to be signed by a
// backing account.
type SignerFn func(accounts.Account, string, []byte) ([]byte, error)

// BLSSignerFn is a signer callback function to request a message and extra data to be signed by a
// backing account using BLS with a direct or composite hasher
type BLSSignerFn func(accounts.Account, []byte, []byte, bool, bool) (blscrypto.SerializedSignature, error)

// HashSignerFn is a signer callback function to request a hash to be signed by a
// backing account.
type HashSignerFn func(accounts.Account, []byte) ([]byte, error)

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
// To considerably reduce the bandwith used by the RoundChangeCertificate type (which often
// contains repeated Proposal from different RoundChange messages), we break it apart during
// RLP encoding and then build it back during decoding. Proposals are sent just once, and
// Messages referencing them will use their Hash instead.

type RoundChangeCertificate struct {
	RoundChangeMessages []Message
}

func (b *RoundChangeCertificate) IsEmpty() bool {
	return len(b.RoundChangeMessages) == 0
}

// EncodeRLP serializes b into the Ethereum RLP format.
func (c *RoundChangeCertificate) EncodeRLP(w io.Writer) error {
	proposals := c.distinctProposals()
	messages := c.indexedMessages()
	return rlp.Encode(w, []interface{}{proposals, messages})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (c *RoundChangeCertificate) DecodeRLP(s *rlp.Stream) error {
	var decodestr struct {
		Proposals       []Proposal
		IndexedMessages []IndexedRoundChangeMessage
	}

	if err := s.Decode(&decodestr); err != nil {
		return err
	}
	return c.setValues(decodestr.Proposals, decodestr.IndexedMessages)
}

func getRoundChange(message *Message) RoundChange {
	// implement
	return nil
}

// setValues recreates the RoundChange messages from the props (Proposal set/index) and the
// list of IndexedRoundChangeMessage, which is supposed to be the same as the RoundChange
// Messages but with the proposals just referenced to the Proposals set.
func (c *RoundChangeCertificate) setValues(props []Proposal, iMess []IndexedRoundChangeMessage) error {
	// create a Proposal index from the list
	propIndex := make(map[common.Hash]Proposal)
	for _, prop := range props {
		propIndex[prop.Hash()] = prop
	}
	// Recreate Messages one by one
	mess := make([]Message, len(iMess))
	for i, im := range iMess {
		mess[i] = Message{
			Code:      im.Message.Code,
			Address:   im.Message.Address,
			Signature: im.Message.Signature,
		}

		// Add the proposal to the message if it had one
		roundChange := getRoundChange(&im.Message)
		if proposal, ok := propIndex[im.ProposalHash]; ok {
			roundChange.PreparedCertificate.Proposal = proposal
		}

		setMessageBytes(&mess[i], roundChange)
	}
	c.RoundChangeMessages = mess
	return nil
}

func (c *RoundChangeCertificate) distinctProposals() []Proposal {
	r := make(map[common.Hash]Proposal)
	for _, message := range c.RoundChangeMessages {
		rcm := message.RoundChange()
		if rcm.HasPreparedCertificate() {
			prop := rcm.PreparedCertificate.Proposal
			if prop != nil {
				r[prop.Hash()] = prop
			}
		}
	}
	// Iterate values. RLP does not support maps
	v := make([]Proposal, 0)
	for _, p := range r {
		v = append(v, p)
	}
	return v
}

type IndexedRoundChangeMessage struct {
	ProposalHash common.Hash
	Message      Message // PreparedCertificate.Proposal = nil if any
}

func NewIndexedRoundChangeMessage(message *Message) *IndexedRoundChangeMessage {
	roundChange := getRoundChange(message)
	pc := roundChange.PreparedCertificate

	// Assume message.Code = MsgRoundChange
	indexedMsg := IndexedRoundChangeMessage{
		Message: Message{
			Code:      message.Code,
			Address:   message.Address,
			Signature: message.Signature,
		},
	}

	if pc.Proposal != nil {
		indexedMsg.ProposalHash = pc.Proposal.Hash()
	}

	setMessageBytes(&indexedMsg.Message,
		RoundChange{
			View: roundChange.View,
			PreparedCertificate: PreparedCertificate{
				Proposal:                nil, // Removed
				PrepareOrCommitMessages: pc.PrepareOrCommitMessages,
			},
		})

	return &indexedMsg
}

func (c *RoundChangeCertificate) indexedMessages() []*IndexedRoundChangeMessage {
	r := make([]*IndexedRoundChangeMessage, len(c.RoundChangeMessages))
	for i, message := range c.RoundChangeMessages {
		r[i] = NewIndexedRoundChangeMessage(&message)
	}
	return r
}

// ## Preprepare ##############################################################

// NewPreprepareMessage constructs a Message instance with the given sender and
// prePrepare. Both the prePrepare instance and the serialized bytes of
// prePrepare are part of the returned Message.
func NewPreprepareMessage(prePrepare *Preprepare, sender common.Address) *Message {
	message := &Message{
		Address:    sender,
		Code:       MsgPreprepare,
		prePrepare: prePrepare,
	}
	setMessageBytes(message, prePrepare)
	return message
}

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
		Number:  big.NewInt(0),
		GasUsed: 0,
		Time:    0,
	}
	block := &types.Block{}
	block = block.WithRandomness(&types.EmptyRandomness)
	block = block.WithEpochSnarkData(&types.EmptyEpochSnarkData)

	return PreparedCertificate{
		Proposal:                block.WithHeader(emptyHeader),
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

// NewRoundChangeMessage constructs a Message instance with the given sender and
// roundChange. Both the roundChange instance and the serialized bytes of
// roundChange are part of the returned Message.
func NewRoundChangeMessage(roundChange *RoundChange, sender common.Address) *Message {
	message := &Message{
		Address:     sender,
		Code:        MsgRoundChange,
		roundChange: roundChange,
	}
	setMessageBytes(message, roundChange)
	return message
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

// ## Subject #################################################################

// NewPrepareMessage constructs a Message instance with the given sender and
// subject. Both the subject instance and the serialized bytes of subject are
// part of the returned Message.
func NewPrepareMessage(subject *Subject, sender common.Address) *Message {
	message := &Message{
		Address: sender,
		Code:    MsgPrepare,
		prepare: subject,
	}
	setMessageBytes(message, subject)
	return message
}

type Subject struct {
	View   *View
	Digest common.Hash
}

func (s *Subject) String() string {
	return fmt.Sprintf("{View: %v, Digest: %v}", s.View, s.Digest.String())
}

// ## CommittedSubject #################################################################

// NewCommitMessage constructs a Message instance with the given sender and
// commit. Both the commit instance and the serialized bytes of commit are
// part of the returned Message.
func NewCommitMessage(commit *CommittedSubject, sender common.Address) *Message {
	message := &Message{
		Address:          sender,
		Code:             MsgCommit,
		committedSubject: commit,
	}
	setMessageBytes(message, commit)
	return message
}

type CommittedSubject struct {
	Subject               *Subject
	CommittedSeal         []byte
	EpochValidatorSetSeal []byte
}

// ## ForwardMessage #################################################################

// NewForwardMessage constructs a Message instance with the given sender and
// forwardMessage. Both the forwardMessage instance and the serialized bytes of
// fowardMessage are part of the returned Message.
func NewForwardMessage(fowardMessage *ForwardMessage, sender common.Address) *Message {
	message := &Message{
		Address:        sender,
		Code:           FwdMsg,
		forwardMessage: fowardMessage,
	}
	setMessageBytes(message, fowardMessage)
	return message
}

type ForwardMessage struct {
	Code          uint64
	Msg           []byte
	DestAddresses []common.Address
}

// ===============================================================
//
// define the IstanbulQueryEnode message format, the QueryEnodeMsgCache entries, the queryEnode send function (both the gossip version and the "retrieve from cache" version), and the announce get function

// NewQueryEnodeMessage constructs a Message instance with the given sender and
// queryEnode. Both the queryEnode instance and the serialized bytes of
// queryEnode are part of the returned Message.
func NewQueryEnodeMessage(queryEnode *QueryEnodeData, sender common.Address) *Message {
	message := &Message{
		Address:    sender,
		Code:       QueryEnodeMsg,
		queryEnode: queryEnode,
	}
	setMessageBytes(message, queryEnode)
	return message
}

type EncryptedEnodeURL struct {
	DestAddress       common.Address
	EncryptedEnodeURL []byte
}

func (ee *EncryptedEnodeURL) String() string {
	return fmt.Sprintf("{DestAddress: %s, EncryptedEnodeURL length: %d}", ee.DestAddress.String(), len(ee.EncryptedEnodeURL))
}

type QueryEnodeData struct {
	EncryptedEnodeURLs []*EncryptedEnodeURL
	Version            uint
	// The timestamp of the node when the message is generated.
	// This results in a new hash for a newly generated message so it gets regossiped by other nodes
	Timestamp uint
}

func (qed *QueryEnodeData) String() string {
	return fmt.Sprintf("{Version: %v, Timestamp: %v, EncryptedEnodeURLs: %v}", qed.Version, qed.Timestamp, qed.EncryptedEnodeURLs)
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes ar into the Ethereum RLP format.
func (ee *EncryptedEnodeURL) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ee.DestAddress, ee.EncryptedEnodeURL})
}

// DecodeRLP implements rlp.Decoder, and load the ar fields from a RLP stream.
func (ee *EncryptedEnodeURL) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		DestAddress       common.Address
		EncryptedEnodeURL []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ee.DestAddress, ee.EncryptedEnodeURL = msg.DestAddress, msg.EncryptedEnodeURL
	return nil
}

// EncodeRLP serializes ad into the Ethereum RLP format.
func (qed *QueryEnodeData) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{qed.EncryptedEnodeURLs, qed.Version, qed.Timestamp})
}

// DecodeRLP implements rlp.Decoder, and load the ad fields from a RLP stream.
func (qed *QueryEnodeData) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		EncryptedEnodeURLs []*EncryptedEnodeURL
		Version            uint
		Timestamp          uint
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	qed.EncryptedEnodeURLs, qed.Version, qed.Timestamp = msg.EncryptedEnodeURLs, msg.Version, msg.Timestamp
	return nil
}

// ## Consensus Message codes ##########################################################

const (
	MsgPreprepare uint64 = iota
	MsgPrepare
	MsgCommit
	MsgRoundChange
)

// Message is a wrapper used for all istanbul communication. It encapsulates
// the sender's address, a code that indicates the type of the wrapped message
// and a signature. Message instances also hold a deserialised instance of the
// inner message which can be retrieved by calling the corresponding function
// (Commit(), Preprepare() ... etc).
//
// Messages should be initialised either through the use of one of the
// NewXXXMessage constructors or by calling FromPayload on an empty Message
// instance, these mechanisms ensure that the produced Message instances will
// contain the deserialised inner message instance and the serialised bytes of
// the inner message.
type Message struct {
	Code      uint64
	Msg       []byte         // The serialised bytes of the innner message.
	Address   common.Address // The sender address
	Signature []byte         // Signature of the Message using the private key associated with the "Address" field

	// The below fields are the potential inner message instances only one
	// should be set for a message instance. These fields are not rlp
	// serializable since they are private. They are set when calling
	// Message.FromPayload, or at message construction time.
	committedSubject    *CommittedSubject
	prePrepare          *Preprepare
	prepare             *Subject
	roundChange         *RoundChange
	queryEnode          *QueryEnodeData
	forwardMessage      *ForwardMessage
	enodeCertificate    *EnodeCertificate
	versionCertificates []*VersionCertificate
	valEnodeShareData   *ValEnodesShareData
}

// setMessageBytes sets the Msg field of msg to the rlp serialised bytes of
// innerMessage. If innerMessage fails serialisation then this function
// panics. This is intended for use by NewXXXMessage constructors only.
func setMessageBytes(msg *Message, innerMessage interface{}) {
	bytes, err := rlp.EncodeToBytes(innerMessage)
	if err != nil {
		panic(fmt.Sprintf("attempt to serialise inner message of type %T failed", innerMessage))
	}
	msg.Msg = bytes
}

func (m *Message) Sign(signingFn func(data []byte) ([]byte, error)) error {
	// Construct and encode a message with no signature
	payloadNoSig, err := m.PayloadNoSig()
	if err != nil {
		return err
	}
	m.Signature, err = signingFn(payloadNoSig)
	return err
}

func (m *Message) DecodeRLP(stream *rlp.Stream) error {
	type decodable Message
	var d decodable
	err := stream.Decode(&d)
	if err != nil {
		return err
	}
	*m = Message(d)

	if len(m.Msg) == 0 && len(m.Signature) == 0 {
		// Empty validator handshake message
		return nil
	}

	switch m.Code {
	case MsgPreprepare:
		var p *Preprepare
		err = m.decode(&p)
		if err != nil {
			return err
		}
		m.prePrepare = p
	case MsgPrepare:
		var p *Subject
		err = m.decode(&p)
		m.prepare = p
	case MsgCommit:
		var cs *CommittedSubject
		err = m.decode(&cs)
		m.committedSubject = cs
	case MsgRoundChange:
		var p *RoundChange
		err = m.decode(&p)
		if err != nil {
			return err
		}
		m.roundChange = p
	case QueryEnodeMsg:
		var q *QueryEnodeData
		err = m.decode(&q)
		m.queryEnode = q
	case FwdMsg:
		var f *ForwardMessage
		err = m.decode(&f)
		m.forwardMessage = f
	case EnodeCertificateMsg:
		var e *EnodeCertificate
		err = m.decode(&e)
		m.enodeCertificate = e
	case VersionCertificatesMsg:
		var v []*VersionCertificate
		err = m.decode(&v)
		m.versionCertificates = v
	case ValEnodesShareMsg:
		var v *ValEnodesShareData
		err = m.decode(&v)
		m.valEnodeShareData = v
	default:
		err = fmt.Errorf("unrecognised message code %d", m.Code)
	}
	return err

}

// FromPayload decodes b into a Message instance it will set one of the private
// fields committedSubject, prePrepare, prepare or roundChange depending on the
// type of the message.
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

func (m *Message) decode(val interface{}) error {
	return rlp.DecodeBytes(m.Msg, val)
}

func (m *Message) String() string {
	return fmt.Sprintf("{Code: %v, Address: %v}", m.Code, m.Address.String())
}

// Commit returns the committed subject if this is a commit message.
func (m *Message) Commit() *CommittedSubject {
	return m.committedSubject
}

// Preprepare returns preprepare if this is a preprepare message.
func (m *Message) Preprepare() *Preprepare {
	return m.prePrepare
}

// Prepare returns prepare if this is a prepare message.
func (m *Message) Prepare() *Subject {
	return m.prepare
}

// Prepare returns round change if this is a round change message.
func (m *Message) RoundChange() *RoundChange {
	return m.roundChange
}

// QueryEnode returns query enode data if this is a query enode message.
func (m *Message) QueryEnodeMsg() *QueryEnodeData {
	return m.queryEnode
}

// ForwardMessage returns forward message if this is a forward message.
func (m *Message) ForwardMessage() *ForwardMessage {
	return m.forwardMessage
}

// EnodeCertificate returns the enode certificate if this is an enode
// certificate message
func (m *Message) EnodeCertificate() *EnodeCertificate {
	return m.enodeCertificate
}

// VersionCertificates returns the version certificate entries if this is a
// version certificates message.
func (m *Message) VersionCertificates() []*VersionCertificate {
	return m.versionCertificates
}

// ValEnodesShareData returns val enode share data if this is a val enodes share message.
func (m *Message) ValEnodesShareData() *ValEnodesShareData {
	return m.valEnodeShareData
}

func (m *Message) Copy() *Message {
	return &Message{
		Code:      m.Code,
		Msg:       append(m.Msg[:0:0], m.Msg...),
		Address:   m.Address,
		Signature: append(m.Signature[:0:0], m.Signature...),
	}
}

// MapMessagesToSenders map a list of Messages to the list of the sender addresses
func MapMessagesToSenders(messages []Message) []common.Address {
	returnList := make([]common.Address, len(messages))

	for i, ms := range messages {
		returnList[i] = ms.Address
	}

	return returnList
}

// ## EnodeCertificate ######################################################################

// NewValEnodesShareMessage constructs a Message instance with the given sender
// and enodeCertificate. Both the enodeCertificate instance and the serialized
// bytes of enodeCertificate are part of the returned Message.
func NewEnodeCeritifcateMessage(enodeCertificate *EnodeCertificate, sender common.Address) *Message {
	message := &Message{
		Address:          sender,
		Code:             EnodeCertificateMsg,
		enodeCertificate: enodeCertificate,
	}
	setMessageBytes(message, enodeCertificate)
	return message
}

type EnodeCertificate struct {
	EnodeURL string
	Version  uint
}

// EncodeRLP serializes ec into the Ethereum RLP format.
func (ec *EnodeCertificate) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ec.EnodeURL, ec.Version})
}

// DecodeRLP implements rlp.Decoder, and load the ec fields from a RLP stream.
func (ec *EnodeCertificate) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		EnodeURL string
		Version  uint
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ec.EnodeURL, ec.Version = msg.EnodeURL, msg.Version
	return nil
}

// ## EnodeCertMsg ######################################################################
type EnodeCertMsg struct {
	Msg           *Message
	DestAddresses []common.Address
}

// ## AddressEntry ######################################################################
// AddressEntry is an entry for the valEnodeTable.
type AddressEntry struct {
	Address                      common.Address
	PublicKey                    *ecdsa.PublicKey
	Node                         *enode.Node
	Version                      uint
	HighestKnownVersion          uint
	NumQueryAttemptsForHKVersion uint
	LastQueryTimestamp           *time.Time
}

func (ae *AddressEntry) String() string {
	var nodeString string
	if ae.Node != nil {
		nodeString = ae.Node.String()
	}
	return fmt.Sprintf("{address: %v, enodeURL: %v, version: %v, highestKnownVersion: %v, numQueryAttempsForHKVersion: %v, LastQueryTimestamp: %v}", ae.Address.String(), nodeString, ae.Version, ae.HighestKnownVersion, ae.NumQueryAttemptsForHKVersion, ae.LastQueryTimestamp)
}

// Implement RLP Encode/Decode interface
type AddressEntryRLP struct {
	Address                      common.Address
	CompressedPublicKey          []byte
	EnodeURL                     string
	Version                      uint
	HighestKnownVersion          uint
	NumQueryAttemptsForHKVersion uint
	LastQueryTimestamp           []byte
}

// EncodeRLP serializes AddressEntry into the Ethereum RLP format.
func (ae *AddressEntry) EncodeRLP(w io.Writer) error {
	var nodeString string
	if ae.Node != nil {
		nodeString = ae.Node.String()
	}
	var publicKeyBytes []byte
	if ae.PublicKey != nil {
		publicKeyBytes = crypto.CompressPubkey(ae.PublicKey)
	}
	var lastQueryTimestampBytes []byte
	if ae.LastQueryTimestamp != nil {
		var err error
		lastQueryTimestampBytes, err = ae.LastQueryTimestamp.MarshalBinary()
		if err != nil {
			return err
		}
	}

	return rlp.Encode(w, AddressEntryRLP{Address: ae.Address,
		CompressedPublicKey:          publicKeyBytes,
		EnodeURL:                     nodeString,
		Version:                      ae.Version,
		HighestKnownVersion:          ae.HighestKnownVersion,
		NumQueryAttemptsForHKVersion: ae.NumQueryAttemptsForHKVersion,
		LastQueryTimestamp:           lastQueryTimestampBytes})
}

// DecodeRLP implements rlp.Decoder, and load the AddressEntry fields from a RLP stream.
func (ae *AddressEntry) DecodeRLP(s *rlp.Stream) error {
	var entry AddressEntryRLP
	var err error
	if err := s.Decode(&entry); err != nil {
		return err
	}
	var node *enode.Node
	if len(entry.EnodeURL) > 0 {
		node, err = enode.ParseV4(entry.EnodeURL)
		if err != nil {
			return err
		}
	}
	var publicKey *ecdsa.PublicKey
	if len(entry.CompressedPublicKey) > 0 {
		publicKey, err = crypto.DecompressPubkey(entry.CompressedPublicKey)
		if err != nil {
			return err
		}
	}
	lastQueryTimestamp := &time.Time{}
	if len(entry.LastQueryTimestamp) > 0 {
		err := lastQueryTimestamp.UnmarshalBinary(entry.LastQueryTimestamp)
		if err != nil {
			return err
		}
	}

	*ae = AddressEntry{Address: entry.Address,
		PublicKey:                    publicKey,
		Node:                         node,
		Version:                      entry.Version,
		HighestKnownVersion:          entry.HighestKnownVersion,
		NumQueryAttemptsForHKVersion: entry.NumQueryAttemptsForHKVersion,
		LastQueryTimestamp:           lastQueryTimestamp}
	return nil
}

// GetNode returns the address entry's node
func (ae *AddressEntry) GetNode() *enode.Node {
	return ae.Node
}

// GetVersion returns the addess entry's version
func (ae *AddressEntry) GetVersion() uint {
	return ae.Version
}

// GetAddess returns the addess entry's address
func (ae *AddressEntry) GetAddress() common.Address {
	return ae.Address
}

// ## VersionCertificate ######################################################################

// NewVersionCeritifcatesMessage constructs a Message instance with the given sender
// and versionCertificates. Both the versionCertificates instance and the serialized
// bytes of versionCertificates are part of the returned Message.
func NewVersionCeritifcatesMessage(versionCertificates []*VersionCertificate, sender common.Address) *Message {
	message := &Message{
		Address:             sender,
		Code:                VersionCertificatesMsg,
		versionCertificates: versionCertificates,
	}
	setMessageBytes(message, versionCertificates)
	return message
}

// VersionCertificate is an entry in the VersionCertificateDB.
// It's a signed message from a registered or active validator indicating
// the most recent version of its enode.
type VersionCertificate struct {
	Version   uint
	Signature []byte
	address   common.Address
	pubKey    *ecdsa.PublicKey
}

// NewVersionCeritifcate constructs a VersionCertificate instance with the
// given version.  It uses the signingFn to generate a version signature and
// then builds a version certificate from the version and its signature.
func NewVersionCertificate(version uint, signingFn func([]byte) ([]byte, error)) (*VersionCertificate, error) {
	vc := &VersionCertificate{Version: version}
	payloadToSign, err := vc.signaturePayload()
	if err != nil {
		return nil, err
	}
	vc.Signature, err = signingFn(payloadToSign)
	if err != nil {
		return nil, err
	}
	err = vc.recoverAddressAndPubKey()
	if err != nil {
		return nil, err
	}

	return vc, nil
}

// NewVersionCeritifcateFrom fields constructs a VersionCertificate instance
// with the given fields, using them to build a VersionCertificate instance.
// No validation is done on the provided fields. It
// is assumed that the fields are valid, meaning that the signature was
// generated using the given version and the private part of the given public
// key, and also that the address corresponds to the given public key.
func NewVersionCertificateFromFields(version uint, signature []byte, address common.Address, key *ecdsa.PublicKey) *VersionCertificate {
	return &VersionCertificate{
		Version:   version,
		Signature: signature,
		address:   address,
		pubKey:    key,
	}
}

// Used as a salt when signing versionCertificate. This is to account for
// the unlikely case where a different signed struct with the same field types
// is used elsewhere and shared with other nodes. If that were to happen, a
// malicious node could try sending the other struct where this struct is used,
// or vice versa. This ensures that the signature is only valid for this struct.
var versionCertificateSalt = []byte("versionCertificate")

func (vc *VersionCertificate) signaturePayload() ([]byte, error) {
	return rlp.EncodeToBytes([]interface{}{versionCertificateSalt, vc.Version})
}

func (vc *VersionCertificate) Address() common.Address {
	return vc.address
}

func (vc *VersionCertificate) PublicKey() *ecdsa.PublicKey {
	return vc.pubKey
}

func (vc *VersionCertificate) String() string {
	return fmt.Sprintf("%d", vc.Version)
}

func (vc *VersionCertificate) DecodeRLP(s *rlp.Stream) error {
	// Create separate type to avoid stack overflow when calling Decode
	type decodable VersionCertificate
	var d decodable
	if err := s.Decode(&d); err != nil {
		return err
	}
	// copy struct data
	*vc = VersionCertificate(d)

	return vc.recoverAddressAndPubKey()
}

func (vc *VersionCertificate) recoverAddressAndPubKey() error {
	payloadToSign, err := vc.signaturePayload()
	if err != nil {
		return err
	}
	payloadHash := crypto.Keccak256(payloadToSign)
	vc.pubKey, err = crypto.SigToPub(payloadHash, vc.Signature)
	if err != nil {
		return err
	}
	vc.address = crypto.PubkeyToAddress(*vc.pubKey)
	return nil
}

// ## SharedValidatorEnode ######################################################################

// NewValEnodesShareMessage constructs a Message instance with the given sender
// and valEnodeShareData. Both the valEnodeShareData instance and the
// serialized bytes of valEnodeShareData are part of the returned Message.
func NewValEnodesShareMessage(valEnodeShareData *ValEnodesShareData, sender common.Address) *Message {
	message := &Message{
		Address:           sender,
		Code:              ValEnodesShareMsg,
		valEnodeShareData: valEnodeShareData,
	}
	setMessageBytes(message, valEnodeShareData)
	return message
}

type SharedValidatorEnode struct {
	Address  common.Address
	EnodeURL string
	Version  uint
}

type ValEnodesShareData struct {
	ValEnodes []SharedValidatorEnode
}

func (sve *SharedValidatorEnode) String() string {
	return fmt.Sprintf("{Address: %s, EnodeURL: %v, Version: %v}", sve.Address.Hex(), sve.EnodeURL, sve.Version)
}

func (sd *ValEnodesShareData) String() string {
	outputStr := "{ValEnodes:"
	for _, valEnode := range sd.ValEnodes {
		outputStr = fmt.Sprintf("%s %s", outputStr, valEnode.String())
	}
	return fmt.Sprintf("%s}", outputStr)
}

// EncodeRLP serializes sd into the Ethereum RLP format.
func (sd *ValEnodesShareData) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{sd.ValEnodes})
}

// DecodeRLP implements rlp.Decoder, and load the sd fields from a RLP stream.
func (sd *ValEnodesShareData) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		ValEnodes []SharedValidatorEnode
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	sd.ValEnodes = msg.ValEnodes
	return nil
}
