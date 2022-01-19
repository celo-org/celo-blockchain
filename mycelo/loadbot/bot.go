package loadbot

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"sync"
	"time"

	ethereum "github.com/celo-org/celo-blockchain"
	bind "github.com/celo-org/celo-blockchain/accounts/abi/bind_v2"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"

	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/mycelo/contract"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"golang.org/x/sync/errgroup"
)

// 110k gas for stable token transfer is pretty reasonable. It's just under 100k in practice
const GasForTransferWithComment = 110000

// LoadGenerator keeps track of in-flight transactions
type LoadGenerator struct {
	MaxPending      uint64
	Pending         chan struct{}
	PendingMap      map[common.Hash]chan struct{}
	PendingMapMutex sync.Mutex
}

// TxConfig contains the options for a transaction
type txConfig struct {
	Acc               env.Account
	Nonce             uint64
	Recipient         common.Address
	Value             *big.Int
	Verbose           bool
	SkipGasEstimation bool
	MixFeeCurrency    bool
}

// Config represent the load bot run configuration
type Config struct {
	ChainID               *big.Int
	Accounts              []env.Account
	Amount                *big.Int
	TransactionsPerSecond int
	Clients               []*ethclient.Client
	Verbose               bool
	MaxPending            uint64
	SkipGasEstimation     bool
	MixFeeCurrency        bool
}

// Start will start loads bots
func Start(ctx context.Context, cfg *Config) error {
	// Set up nonces, we have to manage nonces because calling PendingNonceAt
	// is racy and often results in using the same nonce more than once when
	// applying heavy load.
	nonces := make([]uint64, len(cfg.Accounts))
	for i, a := range cfg.Accounts {
		nonce, err := cfg.Clients[0].PendingNonceAt(ctx, a.Address)
		if err != nil {
			return fmt.Errorf("failed to retrieve pending nonce for account %s: %v", a.Address.String(), err)
		}
		nonces[i] = nonce
	}

	// Offset the receiver from the sender so that they are different
	recvIdx := len(cfg.Accounts) / 2
	sendIdx := 0
	clientIdx := 0

	// Create a ticker to trigger a transaction go routine TPS times per second.
	// Note that a ticker will drop events when the receiver is keeping up.
	period := 1 * time.Second / time.Duration(cfg.TransactionsPerSecond)
	ticker := time.NewTicker(period)

	// Create the worker group and initialize the load generator state.
	group, ctx := errgroup.WithContext(ctx)
	lg := &LoadGenerator{
		MaxPending: cfg.MaxPending,
		Pending:    make(chan struct{}, cfg.MaxPending),
		PendingMap: make(map[common.Hash]chan struct{}),
	}

	// Kick off a consumer for newly produced blocks to check for mined transactions.
	// This method of marking transactions mined is implemented to be more efficient on the
	// validator nodes RPC resources (than calling eth_getTransactionReceipt in a loop).
	go func() {
		latest := big.NewInt(1)
		client := cfg.Clients[0]
		ticker := time.NewTicker(time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			// Fetch the latest block(s) and extract any tranactions that have been mined.
			header, err := client.HeaderByNumber(ctx, latest)
			for err == nil {
				// Using the header, get the block body with transactions.
				var block *types.Block
				block, err = client.BlockByHash(ctx, header.Hash())
				if err != nil {
					fmt.Printf("Error in fetching block by hash: %v\n", err)
					break
				}

				// Loop through all the transactions in a block and mark them as mined.
				lg.PendingMapMutex.Lock()
				for _, tx := range block.Transactions() {
					txMined, ok := lg.PendingMap[tx.Hash()]
					if !ok {
						txMined = make(chan struct{})
						lg.PendingMap[tx.Hash()] = txMined
					}

					// Signal that the transaction has been mined.
					close(txMined)
				}
				lg.PendingMapMutex.Unlock()

				// Adavance the latest block number by one and try to fetch again.
				latest.Add(latest, big.NewInt(1))
				header, err = client.HeaderByNumber(ctx, latest)
			}
			if err != ethereum.NotFound {
				fmt.Printf("Error in fetching header by number: %d, %v\n", latest.Uint64(), err)
			}
		}
	}()

	// Fire off transactions in a loop.
	for {
		select {
		case <-ticker.C:
			select {
			// Block until there is an open slot for a pending transaction.
			case lg.Pending <- struct{}{}:
			case <-ctx.Done():
				continue
			}

			// Choose the next receiver index via round robin selction.
			recvIdx++
			recipient := cfg.Accounts[recvIdx%len(cfg.Accounts)].Address

			// Choose the next receiver index via round robin selection, and set the nonce.
			sendIdx++
			sender := cfg.Accounts[sendIdx%len(cfg.Accounts)]
			nonce := nonces[sendIdx%len(cfg.Accounts)]
			nonces[sendIdx%len(cfg.Accounts)]++

			// Choose the next ethclient instance to use.
			clientIdx++
			client := cfg.Clients[clientIdx%len(cfg.Clients)]

			// Kick off the goroutine to run the transaction.
			group.Go(func() error {
				txCfg := txConfig{
					Acc:               sender,
					Nonce:             nonce,
					Recipient:         recipient,
					Value:             cfg.Amount,
					Verbose:           cfg.Verbose,
					SkipGasEstimation: cfg.SkipGasEstimation,
					MixFeeCurrency:    cfg.MixFeeCurrency,
				}
				return runTransaction(ctx, client, cfg.ChainID, lg, txCfg)
			})
		case <-ctx.Done():
			return group.Wait()
		}
	}
}

func runTransaction(ctx context.Context, client *ethclient.Client, chainID *big.Int, lg *LoadGenerator, txCfg txConfig) error {
	defer func() {
		select {
		case <-lg.Pending:
		default:
			fmt.Printf("Transaction concluded with empty pending channel\n")
		}
	}()

	abi := contract.AbiFor("StableToken")
	stableToken := bind.NewBoundContract(env.MustProxyAddressFor("StableToken"), *abi, client)

	transactor, _ := bind.NewKeyedTransactorWithChainID(txCfg.Acc.PrivateKey, chainID)
	transactor.Context = ctx
	transactor.ChainID = chainID
	transactor.Nonce = new(big.Int).SetUint64(txCfg.Nonce)

	stableTokenAddress := env.MustProxyAddressFor("StableToken")

	if n := rand.Intn(2); txCfg.MixFeeCurrency && n == 0 {
		transactor.FeeCurrency = nil

	} else {
		transactor.FeeCurrency = &stableTokenAddress
	}
	if txCfg.SkipGasEstimation {
		transactor.GasLimit = GasForTransferWithComment
	}

	tx, err := stableToken.TxObj(transactor, "transferWithComment", txCfg.Recipient, txCfg.Value, "need to proivde some long comment to make it similar to an encrypted comment").Send()
	if err != nil {
		if err != context.Canceled {
			fmt.Printf("Error sending transaction: %v\n", err)
		}
		return fmt.Errorf("Error sending transaction: %w", err)
	}
	if txCfg.Verbose {
		fmt.Printf("cusd transfer generated: from: %s to: %s amount: %s\ttxhash: %s\n", txCfg.Acc.Address.Hex(), txCfg.Recipient.Hex(), txCfg.Value.String(), tx.Transaction.Hash().Hex())
		printJSON(tx)
	}

	// Add a channel to the pending map to be notified when the transaction appears in a block.
	// It's possible the transaction will have been mined before we get here, in which case the
	// PendingMap will already have a channel set.
	lg.PendingMapMutex.Lock()
	txMined, ok := lg.PendingMap[tx.Transaction.Hash()]
	if !ok {
		txMined = make(chan struct{})
		lg.PendingMap[tx.Transaction.Hash()] = txMined
	}
	lg.PendingMapMutex.Unlock()

	// Wait for the transaction to appear in a block, for the context to be canceled.
	select {
	case <-txMined:
	case <-ctx.Done():
	}

	// Remove the reference to the channel so that it can be cleaned up.
	lg.PendingMapMutex.Lock()
	delete(lg.PendingMap, tx.Transaction.Hash())
	lg.PendingMapMutex.Unlock()

	return nil
}

func printJSON(obj interface{}) {
	b, _ := json.MarshalIndent(obj, " ", " ")
	fmt.Println(string(b))
}
