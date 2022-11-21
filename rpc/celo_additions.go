package rpc

import (
	"fmt"
)

func (bnh *BlockNumberOrHash) String() string {
	if bnh.BlockHash != nil {
		return bnh.BlockHash.String()
	}
	canonical := ""
	if bnh.RequireCanonical {
		canonical = " (canonical)"
	}
	return fmt.Sprintf("%d%s", bnh.BlockNumber, canonical)
}
