// Copyright 2021 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
)

// AddBlockReceipt checks whether logs were emitted by the core contract calls made as part
// of block processing outside of transactions.  If there are any, it creates a receipt for
// them (the so-called "block receipt") and appends it to receipts
func AddBlockReceipt(receipts types.Receipts, statedb *state.StateDB, blockHash common.Hash) types.Receipts {
	if len(statedb.GetLogs(common.Hash{})) > 0 {
		receipt := types.NewReceipt(nil, false, 0)
		receipt.Logs = statedb.GetLogs(common.Hash{})
		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		for i := range receipt.Logs {
			receipt.Logs[i].TxIndex = uint(len(receipts))
			receipt.Logs[i].TxHash = blockHash
		}
		receipts = append(receipts, receipt)
	}
	return receipts
}
