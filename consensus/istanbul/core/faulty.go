// Copyright 2017 AMIS Technologies
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
	"math/rand"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

func (c *core) random() bool {
	return c.config.FaultyMode == istanbul.Random.Uint64() && rand.Intn(2) == 1
}

func (c *core) notBroadcast() bool {
	return c.config.FaultyMode == istanbul.NotBroadcast.Uint64() || c.random()
}

func (c *core) sendWrongMsg() bool {
	return c.config.FaultyMode == istanbul.SendWrongMsg.Uint64() || c.random()
}

func (c *core) modifySig() bool {
	return c.config.FaultyMode == istanbul.ModifySig.Uint64() || c.random()
}

func (c *core) alwaysPropose() bool {
	return c.config.FaultyMode == istanbul.AlwaysPropose.Uint64() || c.random()
}

func (c *core) alwaysRoundChange() bool {
	return c.config.FaultyMode == istanbul.AlwaysRoundChange.Uint64() || c.random()
}

func (c *core) sendExtraMessages() bool {
	return c.config.FaultyMode == istanbul.SendExtraMessages.Uint64() || c.random()
}

func (c *core) sendExtraFutureMessages() bool {
	return c.config.FaultyMode == istanbul.SendExtraFutureMessages.Uint64() || c.random()
}

func (c *core) neverCommit() bool {
	return c.config.FaultyMode == istanbul.NeverCommit.Uint64() || c.random()
}

func (c *core) halfCommit() bool {
	return c.config.FaultyMode == istanbul.HalfCommit.Uint64() || c.random()
}
