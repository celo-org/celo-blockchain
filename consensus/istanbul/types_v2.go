package istanbul

import (
	"io"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/rlp"
)

type PayloadNoSig interface {
	PayloadNoSig() ([]byte, error)
}

type ValidateFn func([]byte, []byte) (common.Address, error)

func GetSignerFromSignature(p PayloadNoSig, signature []byte, validateFn ValidateFn) (common.Address, error) {
	data, err := p.PayloadNoSig()
	if err != nil {
		return common.Address{}, err
	}

	signer, err := validateFn(data, signature)
	if err != nil {
		return common.Address{}, err
	}
	return signer, nil
}

func CheckSignedBy(p PayloadNoSig, signature []byte, signer common.Address, wrongSignatureError error, validateFn ValidateFn) error {
	extractedSigner, err := GetSignerFromSignature(p, signature, validateFn)
	if err != nil {
		return err
	}

	if extractedSigner != signer {
		return wrongSignatureError
	}
	return nil
}

// ## PreprepareV2 ##############################################################

// NewPreprepareV2Message constructs a Message instance with the given sender and
// prePrepare. Both the prePrepare instance and the serialized bytes of
// prePrepare are part of the returned Message.
func NewPreprepareV2Message(prePrepareV2 *PreprepareV2, sender common.Address) *Message {
	message := &Message{
		Address:      sender,
		Code:         MsgPreprepare,
		prePrepareV2: prePrepareV2,
	}
	setMessageBytes(message, prePrepareV2)
	return message
}

type PreprepareV2 struct {
	View                     *View
	Proposal                 Proposal
	RoundChangeCertificateV2 RoundChangeCertificateV2
}

// EncodeRLP serializes pp into the Ethereum RLP format.
func (pp *PreprepareV2) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{pp.View, pp.Proposal, pp.RoundChangeCertificateV2})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (rcc *PreprepareV2) DecodeRLP(s *rlp.Stream) error {
	type decodable PreprepareV2
	var d decodable
	if err := s.Decode(&d); err != nil {
		return err
	}
	*rcc = PreprepareV2(d)
	return nil
}

type PreparedCertificateV2 struct {
	ProposalHash            common.Hash
	PrepareOrCommitMessages []Message
}

func (pc *PreparedCertificateV2) IsEmpty() bool {
	return len(pc.PrepareOrCommitMessages) == 0
}

// EncodeRLP serializes pc into the Ethereum RLP format.
func (pc *PreparedCertificateV2) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{pc.ProposalHash, pc.PrepareOrCommitMessages})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (pc *PreparedCertificateV2) DecodeRLP(s *rlp.Stream) error {
	type decodable PreparedCertificateV2
	var d decodable
	if err := s.Decode(&d); err != nil {
		return err
	}
	*pc = PreparedCertificateV2(d)
	return nil
}

type RoundChangeRequest struct {
	Address               common.Address
	View                  View
	PreparedCertificateV2 PreparedCertificateV2
	Signature             []byte
}

func (rcr *RoundChangeRequest) HasPreparedCertificate() bool {
	return !rcr.PreparedCertificateV2.IsEmpty()
}

// EncodeRLP serializes rcr into the Ethereum RLP format.
func (rcr *RoundChangeRequest) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{rcr.Address, rcr.View, rcr.PreparedCertificateV2, rcr.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (rcr *RoundChangeRequest) DecodeRLP(s *rlp.Stream) error {
	type decodable RoundChangeRequest
	var d decodable
	if err := s.Decode(&d); err != nil {
		return err
	}
	*rcr = RoundChangeRequest(d)
	return nil
}

func (rcr *RoundChangeRequest) PayloadNoSig() ([]byte, error) {
	return rlp.EncodeToBytes(&RoundChangeRequest{
		Address:               rcr.Address,
		View:                  rcr.View,
		PreparedCertificateV2: rcr.PreparedCertificateV2,
		Signature:             []byte{},
	})
}

func (rcr *RoundChangeRequest) Sign(signingFn func(data []byte) ([]byte, error)) error {
	// Construct and encode a round change request with no signature
	payloadNoSig, err := rcr.PayloadNoSig()
	if err != nil {
		return err
	}
	rcr.Signature, err = signingFn(payloadNoSig)
	return err
}

// NewRoundChangeV2Message constructs a Message instance with the given sender and
// roundChangeV2. Both the roundChangeV2 instance and the serialized bytes of
// roundChange are part of the returned Message.
func NewRoundChangeV2Message(roundChangeV2 *RoundChangeV2, sender common.Address) *Message {
	message := &Message{
		Address:       sender,
		Code:          MsgRoundChangeV2,
		roundChangeV2: roundChangeV2,
	}
	setMessageBytes(message, roundChangeV2)
	return message
}

type RoundChangeV2 struct {
	Request          RoundChangeRequest
	PreparedProposal Proposal
}

func (rc *RoundChangeV2) HasPreparedCertificate() bool {
	return rc.Request.HasPreparedCertificate()
}

func (rc *RoundChangeV2) ProposalMatch() bool {
	if !rc.HasPreparedCertificate() {
		return rc.PreparedProposal == nil && len(rc.Request.PreparedCertificateV2.ProposalHash) == 0
	}
	return rc.PreparedProposal.Hash() == rc.Request.PreparedCertificateV2.ProposalHash
}

// EncodeRLP serializes rc into the Ethereum RLP format.
func (rc *RoundChangeV2) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{rc.Request, rc.PreparedProposal})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (rc *RoundChangeV2) DecodeRLP(s *rlp.Stream) error {
	type decodable RoundChangeV2
	var d decodable
	if err := s.Decode(&d); err != nil {
		return err
	}
	*rc = RoundChangeV2(d)
	return nil
}

type RoundChangeCertificateV2 struct {
	Requests []RoundChangeRequest
}

func (rcc *RoundChangeCertificateV2) HighestRoundPreparedCertificate() *PreparedCertificateV2 {
	var hrpc *PreparedCertificateV2
	var maxRound int64 = -1
	for _, req := range rcc.Requests {
		if !req.HasPreparedCertificate() {
			continue
		}
		if req.View.Round == nil {
			continue
		}
		round := req.View.Round.Int64()
		if hrpc == nil || round > maxRound {
			hrpc = &req.PreparedCertificateV2
			maxRound = round
		}
	}
	return hrpc
}

// EncodeRLP serializes rcc into the Ethereum RLP format.
func (rcc *RoundChangeCertificateV2) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{rcc.Requests})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (rcc *RoundChangeCertificateV2) DecodeRLP(s *rlp.Stream) error {
	type decodable RoundChangeCertificateV2
	var d decodable
	if err := s.Decode(&d); err != nil {
		return err
	}
	*rcc = RoundChangeCertificateV2(d)
	return nil
}
