package les

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/light"
	"github.com/celo-org/celo-blockchain/params"
)

type LightChainReader struct {
	config     *params.ChainConfig
	blockchain *light.LightChain
}

// Config returns the chain configuration.
func (lcr *LightChainReader) Config() *params.ChainConfig {
	return lcr.config
}

func (lcr *LightChainReader) CurrentHeader() *types.Header {
	return lcr.blockchain.CurrentHeader()
}
func (lcr *LightChainReader) GetHeaderByNumber(number uint64) *types.Header {
	return lcr.blockchain.GetHeaderByNumber(number)
}
func (lcr *LightChainReader) GetHeaderByHash(hash common.Hash) *types.Header {
	return lcr.blockchain.GetHeaderByHash(hash)
}
func (lcr *LightChainReader) GetHeader(hash common.Hash, number uint64) *types.Header {
	return lcr.blockchain.GetHeader(hash, number)
}
func (lcr *LightChainReader) GetBlock(hash common.Hash, number uint64) *types.Block {
	panic("GetBlock cannot be called on LightChainReader")
}
