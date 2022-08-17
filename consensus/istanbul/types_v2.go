package istanbul

import (
	"io"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/rlp"
)

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
	var decodable PreprepareV2
	if err := s.Decode(&decodable); err != nil {
		return err
	}
	*rcc = PreprepareV2(decodable)
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
	var decodable PreparedCertificateV2
	if err := s.Decode(&decodable); err != nil {
		return err
	}
	*pc = PreparedCertificateV2(decodable)
	return nil
}

type RoundChangeRequest struct {
	View                  View
	PreparedCertificateV2 PreparedCertificateV2
}

func (rcr *RoundChangeRequest) HasPreparedCertificate() bool {
	return !rcr.PreparedCertificateV2.IsEmpty()
}

// EncodeRLP serializes rcr into the Ethereum RLP format.
func (rcr *RoundChangeRequest) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{rcr.View, rcr.PreparedCertificateV2})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (rcr *RoundChangeRequest) DecodeRLP(s *rlp.Stream) error {
	var decodable RoundChangeRequest
	if err := s.Decode(&decodable); err != nil {
		return err
	}
	*rcr = RoundChangeRequest(decodable)
	return nil
}

type RoundChangeV2 struct {
	Request          RoundChangeRequest
	RequestSignature []byte
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
	return rlp.Encode(w, []interface{}{rc.Request, rc.RequestSignature, rc.PreparedProposal})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (rc *RoundChangeV2) DecodeRLP(s *rlp.Stream) error {
	var decodable RoundChangeV2
	if err := s.Decode(&decodable); err != nil {
		return err
	}
	*rc = RoundChangeV2(decodable)
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
	var decodable RoundChangeCertificateV2
	if err := s.Decode(&decodable); err != nil {
		return err
	}
	*rcc = RoundChangeCertificateV2(decodable)
	return nil
}
