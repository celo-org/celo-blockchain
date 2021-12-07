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

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

// Verify a prepared certificate and return the view that all of its messages pertain to.
func (c *core) verifyPreparedCertificate(preparedCertificate istanbul.PreparedCertificate, targetRound uint64) (*istanbul.View, error) {
	logger := c.newLogger("func", "verifyPreparedCertificate", "proposal_number", preparedCertificate.Proposal.Number(), "proposal_hash", preparedCertificate.Proposal.Hash().String())

	// Validate the attached proposal
	if _, err := c.verifyProposal(preparedCertificate.Proposal); err != nil {
		return nil, errInvalidPreparedCertificateProposal
	}

	if len(preparedCertificate.PrepareOrCommitMessages) > c.current.ValidatorSet().Size() || len(preparedCertificate.PrepareOrCommitMessages) < c.current.ValidatorSet().MinQuorumSize() {
		return nil, errInvalidPreparedCertificateNumMsgs
	}

	seen := make(map[common.Address]bool)

	var view *istanbul.View
	for _, message := range preparedCertificate.PrepareOrCommitMessages {
		data, err := message.PayloadNoSig()
		if err != nil {
			return nil, err
		}
		// Verify message signed by a validator
		signer, err := c.validateFn(data, message.Signature)
		if err != nil {
			return nil, err
		}

		if signer != message.Address {
			return nil, errInvalidPreparedCertificateMsgSignature
		}

		// Check for duplicate messages
		if seen[signer] {
			return nil, errInvalidPreparedCertificateDuplicate
		}
		seen[signer] = true

		// Check that the message is a PREPARE or COMMIT message
		if message.Code != istanbul.MsgPrepare && message.Code != istanbul.MsgCommit {
			return nil, errInvalidPreparedCertificateMsgCode
		}

		// Assume prepare but overwrite if commit
		subject := message.Prepare()
		if message.Code == istanbul.MsgCommit {
			commit := message.Commit()
			// Verify the committedSeal
			_, src := c.current.ValidatorSet().GetByAddress(signer)
			err = c.verifyCommittedSeal(commit, src)
			if err != nil {
				logger.Error("Commit seal did not contain signature from message signer.", "err", err)
				return nil, err
			}

			newValSet, err := c.backend.NextBlockValidators(preparedCertificate.Proposal)
			if err != nil {
				return nil, err
			}
			err = c.verifyEpochValidatorSetSeal(commit, preparedCertificate.Proposal.Number().Uint64(), newValSet, src)
			if err != nil {
				logger.Error("Epoch validator set seal seal did not contain signature from message signer.", "err", err)
				return nil, err
			}

			subject = commit.Subject
		}

		msgLogger := logger.New("msg_round", subject.View.Round, "msg_seq", subject.View.Sequence, "msg_digest", subject.Digest.String())
		msgLogger.Trace("Decoded message in prepared certificate", "code", message.Code)

		// Verify message for the proper sequence.
		if subject.View.Sequence.Cmp(c.current.Sequence()) != 0 {
			return nil, errInvalidPreparedCertificateMsgView
		}

		// Verify message for the proper proposal.
		if subject.Digest != preparedCertificate.Proposal.Hash() {
			return nil, errInvalidPreparedCertificateDigestMismatch
		}

		// Verify that the view is the same for all of the messages
		if view == nil {
			// Check that the prepared certificate round is not newer than the
			// target round. If it is then there is no point switching to the
			// target round since it is already old.
			if subject.View.Round.Uint64() > targetRound {
				return nil, errInvalidRoundChangeViewMismatch
			}
			view = subject.View
		} else {
			if view.Cmp(subject.View) != 0 {
				return nil, errInvalidPreparedCertificateInconsistentViews
			}
		}
	}
	if view == nil {
		return nil, errInvalidRoundChangeViewMismatch
	}
	return view, nil
}

// Extract the view from a PreparedCertificate that has already been verified.
func (c *core) getViewFromVerifiedPreparedCertificate(preparedCertificate istanbul.PreparedCertificate) (*istanbul.View, error) {
	if len(preparedCertificate.PrepareOrCommitMessages) < c.current.ValidatorSet().MinQuorumSize() {
		return nil, errInvalidPreparedCertificateNumMsgs
	}

	message := preparedCertificate.PrepareOrCommitMessages[0]

	// Assume prepare but overwrite if commit
	subject := message.Prepare()
	if message.Code == istanbul.MsgCommit {
		subject = message.Commit().Subject
	}
	return subject.View, nil
}
