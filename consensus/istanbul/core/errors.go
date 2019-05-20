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

package core

import "errors"

var (
	// errInconsistentSubject is returned when received subject is different from
	// current subject.
	errInconsistentSubject = errors.New("inconsistent subjects")
	// errNotFromProposer is returned when received message is supposed to be from
	// proposer.
	errNotFromProposer = errors.New("message does not come from proposer")
	// errIgnored is returned when a message was ignored.
	errIgnored = errors.New("message is ignored")
	// errFutureMessage is returned when current view is earlier than the
	// view of the received message.
	errFutureMessage = errors.New("future message")
	// errOldMessage is returned when the received message's view is earlier
	// than current view.
	errOldMessage = errors.New("old message")
	// errInvalidMessage is returned when the message is malformed.
	errInvalidMessage = errors.New("invalid message")
	// errFailedDecodePreprepare is returned when the PRE-PREPARE message is malformed.
	errFailedDecodePreprepare = errors.New("failed to decode PRE-PREPARE")
	// errFailedDecodePrepare is returned when the PREPARE message is malformed.
	errFailedDecodePrepare = errors.New("failed to decode PREPARE")
	// errFailedDecodeCommit is returned when the COMMIT message is malformed.
	errFailedDecodeCommit = errors.New("failed to decode COMMIT")

	// errInvalidPreparedCertificateProposal is returned when the PREPARED certificate has an invalid proposal.
	errInvalidPreparedCertificateProposal = errors.New("invalid proposal in PREPARED certificate")
	// errInvalidPreparedCertificateNumMsgs is returned when the PREPARED certificate has an incorrect number of PREPARE messages.
	errInvalidPreparedCertificateNumMsgs = errors.New("invalid number of PREPARE messages in certificate")
	// errInvalidPreparedCertificateMsgSignature is returned when the PREPARED certificate has a PREPARE message with an invalid signature.
	errInvalidPreparedCertificateMsgSignature = errors.New("invalid signature in PREPARED certificate")
	// errInvalidPreparedCertificateDuplicate is returned when the PREPARED certificate has multiple PREPARE messages from the same validator.
	errInvalidPreparedCertificateDuplicate = errors.New("duplicate message in PREPARED certificate")
	// errInvalidPreparedCertificateMsgCode is returned when the PREPARED certificate contains a message with the wrong code.
	errInvalidPreparedCertificateMsgCode = errors.New("non-PREPARE message in PREPARED certificate")
	// errInvalidPreparedCertificateMsgDecode is returned when the PREPARED certificate contains a message that can't be decoded.
	errInvalidPreparedCertificateMsgDecode = errors.New("failed to decode message in PREPARED certificate")
	// errInvalidPreparedCertificateMsgView is returned when the PREPARED certificate contains a message for the wrong view
	errInvalidPreparedCertificateMsgView = errors.New("message in PREPARED certificate for wrong view")
	// errInvalidPreparedCertificateDigestMismatch is returned when the PREPARED certificate proposal doesn't match one of the PREPARE messages.
	errInvalidPreparedCertificateDigestMismatch = errors.New("message in PREPARED certificate for different digest than proposal")

	// errInvalidRoundChangeCertificateNumMsgs is returned when the ROUND CHANGE certificate has an incorrect number of ROUND CHANGE messages.
	errInvalidRoundChangeCertificateNumMsgs = errors.New("invalid number of ROUND CHANGE messages in certificate")
	// errInvalidRoundChangeCertificateMsgSignature is returned when the ROUND CHANGE certificate has a ROUND CHANGE message with an invalid signature.
	errInvalidRoundChangeCertificateMsgSignature = errors.New("invalid signature in ROUND CHANGE certificate")
	// errInvalidRoundChangeCertificateDuplicate is returned when the ROUND CHANGE certificate has multiple ROUND CHANGE messages from the same validator.
	errInvalidRoundChangeCertificateDuplicate = errors.New("duplicate message in ROUND CHANGE certificate")
	// errInvalidRoundChangeCertificateMsgCode is returned when the ROUND CHANGE certificate contains a message with the wrong code.
	errInvalidRoundChangeCertificateMsgCode = errors.New("non-ROUND CHANGE message in ROUND CHANGE certificate")
	// errInvalidRoundChangeCertificateMsgDecode is returned when the ROUND CHANGE certificate contains a message that can't be decoded.
	errInvalidRoundChangeCertificateMsgDecode = errors.New("failed to decode message in ROUND CHANGE certificate")
	// errInvalidRoundChangeCertificateMsgView is returned when the ROUND CHANGE certificate contains a message for the wrong view
	errInvalidRoundChangeCertificateMsgView = errors.New("message in ROUND CHANGE certificate for wrong view")

	// errMissingRoundChangeCertificate is returned when ROUND CHANGE certificate is missing from a PREPREPARE for round > 0.
	errMissingRoundChangeCertificate = errors.New("missing ROUND CHANGE certificate in PREPREPARE")
	// errFailedCreatePreparedCertificate is returned when there aren't enough PREPARE messages to create a PREPARED certificate.
	errFailedCreatePreparedCertificate = errors.New("failed to create PREPARED certficate")
	// errFailedCreateRoundChangeCertificate is returned when there aren't enough ROUND CHANGE messages to create a ROUND CHANGE certificate.
	errFailedCreateRoundChangeCertificate = errors.New("failed to create ROUND CHANGE certficate")
	// errInvalidProposal is returned when a PREPARED certificate exists for proposal A in the ROUND CHANGE certificate for a PREPREPARE with proposal B.
	errInvalidProposal = errors.New("invalid proposal in PREPREPARE")
)
