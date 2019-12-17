package les

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

type LightChainReader struct {
	config *params.ChainConfig
}

// Config returns the chain configuration.
func (lcr *LightChainReader) Config() *params.ChainConfig {
	return lcr.config
}

func (lcr *LightChainReader) CurrentHeader() *types.Header                            { return nil }
func (lcr *LightChainReader) GetHeaderByNumber(number uint64) *types.Header           { return nil }
func (lcr *LightChainReader) GetHeaderByHash(hash common.Hash) *types.Header          { return nil }
func (lcr *LightChainReader) GetHeader(hash common.Hash, number uint64) *types.Header { return nil }
func (lcr *LightChainReader) GetBlock(hash common.Hash, number uint64) *types.Block   { return nil }
