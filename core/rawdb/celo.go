package rawdb

import (
	"bytes"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/rlp"
)

// Extra hash comparison is necessary since ancient database only maintains
// the canonical data.
func headerHash(data []byte) common.Hash {
	header := new(types.Header)
	if err := rlp.Decode(bytes.NewReader(data), header); err != nil {
		log.Error("Error decoding stored block header", "err", err)
	}

	return header.Hash()
}
