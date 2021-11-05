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
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

func (c *Core) handleRequest(request *istanbul.Request) error {
	logger := c.newLogger("func", "handleRequest")

	err := c.checkRequestMsg(request)
	if err == errInvalidMessage {
		logger.Warn("invalid request")
		return err
	} else if err != nil {
		logger.Warn("unexpected request", "err", err, "number", request.Proposal.Number(), "hash", request.Proposal.Hash())
		return err
	}

	logger.Trace("handleRequest", "number", request.Proposal.Number(), "hash", request.Proposal.Hash())

	if err = c.current.SetPendingRequest(request); err != nil {
		return err
	}

	// Must go through startNewRound to send proposals for round > 0 to ensure a round change certificate is generated.
	if c.current.State() == StateAcceptRequest && c.current.Round().Cmp(common.Big0) == 0 {
		c.sendPreprepare(request, istanbul.RoundChangeCertificate{})
	}
	return nil
}

// check request state
// return errInvalidMessage if the message is invalid
// return errFutureMessage if the sequence of proposal is larger than current sequence
// return errOldMessage if the sequence of proposal is smaller than current sequence
func (c *Core) checkRequestMsg(request *istanbul.Request) error {
	if request == nil || request.Proposal == nil {
		return errInvalidMessage
	}

	if c := c.current.Sequence().Cmp(request.Proposal.Number()); c > 0 {
		return errOldMessage
	} else if c < 0 {
		return errFutureMessage
	} else {
		return nil
	}
}

var (
	maxNumberForRequestsQueue = big.NewInt(2 << (63 - 2))
)

func (c *Core) storeRequestMsg(request *istanbul.Request) {
	logger := c.newLogger("func", "storeRequestMsg")

	if request.Proposal.Number().Cmp(maxNumberForRequestsQueue) >= 0 {
		logger.Debug("Dropping future request", "number", request.Proposal.Number(), "hash", request.Proposal.Hash())
		return
	}

	logger.Trace("Store future request", "number", request.Proposal.Number(), "hash", request.Proposal.Hash())

	c.pendingRequestsMu.Lock()
	defer c.pendingRequestsMu.Unlock()

	c.pendingRequests.Push(request, -request.Proposal.Number().Int64())
}

func (c *Core) processPendingRequests() {
	c.pendingRequestsMu.Lock()
	defer c.pendingRequestsMu.Unlock()

	for !(c.pendingRequests.Empty()) {
		m, prio := c.pendingRequests.Pop()
		r, ok := m.(*istanbul.Request)
		if !ok {
			c.logger.Warn("Malformed request, skip", "m", m)
			continue
		}

		// Push back if it's a future message
		err := c.checkRequestMsg(r)
		if err == nil {
			c.logger.Trace("Post pending request", "number", r.Proposal.Number(), "hash", r.Proposal.Hash())

			go c.sendEvent(istanbul.RequestEvent{
				Proposal: r.Proposal,
			})
		} else if err == errFutureMessage {
			c.logger.Trace("Stop processing request", "number", r.Proposal.Number(), "hash", r.Proposal.Hash())
			c.pendingRequests.Push(m, prio)
			break
		} else if err != nil {
			c.logger.Trace("Skip the pending request", "number", r.Proposal.Number(), "hash", r.Proposal.Hash(), "err", err)
		}

	}
}
