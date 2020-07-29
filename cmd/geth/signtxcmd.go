package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/bloombits"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"gopkg.in/urfave/cli.v1"
	"math/big"
)

var (
	signTxCommand = cli.Command{
		Action:    signTx,
		Name:      "sign-tx",
		Usage:     "Sign a transaction",
		ArgsUsage: "<json encoded tx> <account passphrase>",
		Category: "BLOCKCHAIN COMMANDS",
		Description: `
The sign-tx command signs a transaction as long as the transaction originates
from an account that is unlockable using the passphrase.

It exepcts a json payload of the transaction and the passphrase as arguments.
`,
	}
)

func signTx(ctx *cli.Context) error {
	var tx ethapi.SendTxArgs
	var err error

	if ctx.NArg() < 2 {
		utils.Fatalf("This command requires two arguments")
	}
	rawTx := ctx.Args()[0]
	passphrase := ctx.Args()[1]

	err = json.Unmarshal([]byte(rawTx), &tx)
	if err != nil {
		return err
	}

	stack, config := makeConfigNode(ctx)
	nonceLock := new(ethapi.AddrLocker)
	if err != nil {
		return err
	}
	chainConfig := &params.ChainConfig{
		ChainID: big.NewInt(int64(config.Eth.NetworkId)),
	}

	backend := &dummyBackend{accountManager: stack.AccountManager(), chainConfig: chainConfig}

	api := ethapi.NewPrivateAccountAPI(backend, nonceLock)
	result, err := api.SignTransaction(context.Background(), tx, passphrase)
	if err != nil {
		return err
	}

	output, err := json.Marshal(result)
	if err != nil {
		return err
	}

	fmt.Println(string(output))
	return nil
}

type dummyBackend struct {
	accountManager *accounts.Manager
	chainConfig *params.ChainConfig
}

func (b *dummyBackend) ChainConfig() *params.ChainConfig {
	return b.chainConfig
}

func (b *dummyBackend) CurrentBlock() *types.Block {
	return nil
}

func (b *dummyBackend) SetHead(number uint64) {
}

func (b *dummyBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error) {
	return nil, nil
}

func (b *dummyBackend) HeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error) {
	return nil, nil
}

func (b *dummyBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return nil, nil
}

func (b *dummyBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error) {
	return nil, nil
}

func (b *dummyBackend) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return nil, nil
}

func (b *dummyBackend) BlockByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Block, error) {
	return nil, nil
}

func (b *dummyBackend) StateAndHeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	return nil, nil, nil
}

func (b *dummyBackend) StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	return nil, nil, nil
}

func (b *dummyBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	return nil, nil
}

func (b *dummyBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	return nil, nil
}

func (b *dummyBackend) GetTd(hash common.Hash) *big.Int {
	return nil
}

func (b *dummyBackend) GetEVM(ctx context.Context, msg vm.Message, header *types.Header, state *state.StateDB) (*vm.EVM, func() error, error) {
	return nil, nil, nil
}

func (b *dummyBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return nil
}

func (b *dummyBackend) RemoveTx(txHash common.Hash) {
}

func (b *dummyBackend) GetPoolTransactions() (types.Transactions, error) {
	return nil, nil
}

func (b *dummyBackend) GetPoolTransaction(txHash common.Hash) *types.Transaction {
	return nil
}

func (b *dummyBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	return nil, common.Hash{}, 0, 0, nil
}

func (b *dummyBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return 0, nil
}

func (b *dummyBackend) Stats() (pending int, queued int) {
	return 0, 0
}

func (b *dummyBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return nil, nil
}

func (b *dummyBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return nil
}

func (b *dummyBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return nil
}

func (b *dummyBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return nil
}

func (b *dummyBackend) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return nil
}

func (b *dummyBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return nil
}

func (b *dummyBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return nil
}

func (b *dummyBackend) Downloader() *downloader.Downloader {
	return nil
}

func (b *dummyBackend) ProtocolVersion() int {
	return 0
}

func (b *dummyBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return nil, nil
}

func (b *dummyBackend) SuggestPriceInCurrency(ctx context.Context, currencyAddress *common.Address, header *types.Header, state *state.StateDB) (*big.Int, error) {
	return nil, nil
}

func (b *dummyBackend) GetGasPriceMinimum(ctx context.Context, currencyAddress *common.Address) (*big.Int, error) {
	return nil, nil
}

func (b *dummyBackend) ChainDb() ethdb.Database {
	return nil
}

func (b *dummyBackend) EventMux() *event.TypeMux {
	return nil
}

func (b *dummyBackend) AccountManager() *accounts.Manager {
	return b.accountManager
}

func (b *dummyBackend) ExtRPCEnabled() bool {
	return false
}

func (b *dummyBackend) RPCGasCap() *big.Int {
	return nil
}

func (b *dummyBackend) BloomStatus() (uint64, uint64) {
	return 0, 0
}

func (b *dummyBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
}

func (b *dummyBackend) GatewayFeeRecipient() common.Address {
	return common.Address{}
}

func (b *dummyBackend) GatewayFee() *big.Int {
	return nil
}
