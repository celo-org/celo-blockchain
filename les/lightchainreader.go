package les

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/params"
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
	return nil
}
