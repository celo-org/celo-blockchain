package service

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/bloombits"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

type SignerApiBackend struct {
	extRPCEnabled bool
	eth           *SignerEthereum
}

func (b *SignerApiBackend) ChainConfig() *params.ChainConfig {
	return nil
}

func (b *SignerApiBackend) CurrentBlock() *types.Block {
	return nil
}

func (b *SignerApiBackend) SetHead(number uint64) {
}

func (b *SignerApiBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) HeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) BlockByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Block, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) StateAndHeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	return nil, nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	return nil, nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) GetTd(hash common.Hash) *big.Int {
	return nil
}

func (b *SignerApiBackend) GetEVM(ctx context.Context, msg vm.Message, header *types.Header, state *state.StateDB) (*vm.EVM, func() error, error) {
	return nil, nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return errors.New("node in signer mode")
}

func (b *SignerApiBackend) RemoveTx(txHash common.Hash) {
}

func (b *SignerApiBackend) GetPoolTransactions() (types.Transactions, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) GetPoolTransaction(txHash common.Hash) *types.Transaction {
	return nil
}

func (b *SignerApiBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	return nil, common.Hash{}, 0, 0, errors.New("node in signer mode")
}

func (b *SignerApiBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return 0, errors.New("node in signer mode")
}

func (b *SignerApiBackend) Stats() (pending int, queued int) {
	return 0, 0
}

func (b *SignerApiBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return nil, nil
}

func (b *SignerApiBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return nil
}

func (b *SignerApiBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return nil
}

func (b *SignerApiBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return nil
}

func (b *SignerApiBackend) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return nil
}

func (b *SignerApiBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return nil
}

func (b *SignerApiBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return nil
}

func (b *SignerApiBackend) Downloader() *downloader.Downloader {
	return nil
}

func (b *SignerApiBackend) ProtocolVersion() int {
	return 0
}

func (b *SignerApiBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) SuggestPriceInCurrency(ctx context.Context, currencyAddress *common.Address, header *types.Header, state *state.StateDB) (*big.Int, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) GetGasPriceMinimum(ctx context.Context, currencyAddress *common.Address) (*big.Int, error) {
	return nil, errors.New("node in signer mode")
}

func (b *SignerApiBackend) ChainDb() ethdb.Database {
	return nil
}

func (b *SignerApiBackend) EventMux() *event.TypeMux {
	return nil
}

func (b *SignerApiBackend) AccountManager() *accounts.Manager {
	return b.eth.accountManager
}

func (b *SignerApiBackend) ExtRPCEnabled() bool {
	return b.extRPCEnabled
}

func (b *SignerApiBackend) RPCGasCap() *big.Int {
	return b.eth.config.RPCGasCap
}

func (b *SignerApiBackend) BloomStatus() (uint64, uint64) {
	return 0, 0
}

func (b *SignerApiBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
}

func (b *SignerApiBackend) GatewayFeeRecipient() common.Address {
	return common.Address{}
}

func (b *SignerApiBackend) GatewayFee() *big.Int {
	// TODO(nategraf): Create a method to fetch the gateway fee values of peers along with the coinbase.
	return eth.DefaultConfig.GatewayFee
}
