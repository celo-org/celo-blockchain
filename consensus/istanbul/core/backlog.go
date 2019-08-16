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

	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

// TODO(Joshua) backlog is seriously wack rn.

// checkMessage checks the message state
// return errInvalidMessage if the message is invalid
// return errFutureMessage if the message number is larger than current number
// return errOldMessage if the message number is smaller than current number
// TODO:How does this make sense in hotstuff
func (c *core) checkMessage(number *big.Int) error {
	// TODO: fix with pointer problems
	// if number > c.current.Number() {
	// 	return errFutureMessage
	// }

	// if number < c.current.Number() {
	// 	return errOldMessage
	// }

	// For states(StatePreprepared, StatePrepared, StateCommitted),
	// can accept all message types if processing with same view
	return nil
}

func (c *core) storeBacklog(msg *istanbul.Message, src istanbul.Validator) {
	logger := c.logger.New("from", src)

	if msg.Address == c.Address() {
		logger.Warn("Backlog from self")
		return
	}

	logger.Trace("Store future message")

	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	backlog := c.backlogs[src]
	if backlog == nil {
		backlog = prque.New(nil)
	}
	switch msg.Code {
	case istanbul.MsgPropose:
		var p *istanbul.Node
		err := msg.Decode(&p)
		if err == nil {
			backlog.Push(msg, msg.Number.Int64())
		}
	default:
		var p *istanbul.Vote
		err := msg.Decode(&p)
		if err == nil {
			backlog.Push(msg, msg.Number.Int64())
		}
	}
	c.backlogs[src] = backlog
}

func (c *core) processBacklog() {
	c.backlogsMu.Lock()
	defer c.backlogsMu.Unlock()

	for src, backlog := range c.backlogs {
		if backlog == nil {
			continue
		}

		logger := c.logger.New("from", src)
		isFuture := false

		// We stop processing if
		//   1. backlog is empty
		//   2. The first message in queue is a future message
		for !(backlog.Empty() || isFuture) {
			m, prio := backlog.Pop()
			msg := m.(*istanbul.Message)
			switch msg.Code {
			case istanbul.MsgPropose:
				var m *istanbul.Node
				msg.Decode(&m)
			default:
				var m *istanbul.Vote
				msg.Decode(&m)
				// TODO
			}
			// Push back if it's a future message
			err := c.checkMessage(msg.Number)
			if err != nil {
				if err == errFutureMessage {
					logger.Trace("Stop processing backlog", "message", msg)
					backlog.Push(msg, prio)
					isFuture = true
					break
				}
				logger.Trace("Skip the backlog event", "message", msg, "err", err)
				continue
			}
			logger.Trace("Post backlog event", "message", msg)

			go c.sendEvent(backlogEvent{
				src: src,
				msg: msg,
			})
		}
	}
}
