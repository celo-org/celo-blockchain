package loadbot

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	bind "github.com/celo-org/celo-blockchain/accounts/abi/bind_v2"
	"github.com/celo-org/celo-blockchain/common"

	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/mycelo/contract"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"golang.org/x/sync/errgroup"
)

// 110k gas for stable token transfer is pretty reasonable. It's just under 100k in practice
const GasForTransferWithComment = 110000

// LoadGenerator keeps track of in-flight transactions
type LoadGenerator struct {
	MaxPending uint64
	Pending    uint64
	PendingMu  sync.Mutex
}

// Config represent the load bot run configuration
type Config struct {
	Accounts              []env.Account
	Amount                *big.Int
	TransactionsPerSecond int
	Clients               []*ethclient.Client
	Verbose               bool
	MaxPending            uint64
	SkipGasEstimation     bool
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

	// Fire off transactions
	period := 1 * time.Second / time.Duration(cfg.TransactionsPerSecond)
	ticker := time.NewTicker(period)
	group, ctx := errgroup.WithContext(ctx)
	lg := &LoadGenerator{
		MaxPending: cfg.MaxPending,
	}
	for {
		select {
		case <-ticker.C:
			lg.PendingMu.Lock()
			if lg.MaxPending != 0 && lg.Pending > lg.MaxPending {
				lg.PendingMu.Unlock()
				continue
			} else {
				lg.Pending++
				lg.PendingMu.Unlock()
			}
			// We use round robin selectors that rollover
			recvIdx++
			recipient := cfg.Accounts[recvIdx%len(cfg.Accounts)].Address

			sendIdx++
			sender := cfg.Accounts[sendIdx%len(cfg.Accounts)]
			nonce := nonces[sendIdx%len(cfg.Accounts)]
			nonces[sendIdx%len(cfg.Accounts)]++

			clientIdx++
			client := cfg.Clients[clientIdx%len(cfg.Clients)]
			group.Go(func() error {
				return runTransaction(ctx, lg, sender, nonce, cfg.Verbose, cfg.SkipGasEstimation, client, recipient, cfg.Amount)
			})
		case <-ctx.Done():
			return group.Wait()
		}
	}
}

func runTransaction(ctx context.Context, lg *LoadGenerator, acc env.Account, nonce uint64, verbose, skipEstimation bool, client *ethclient.Client, recipient common.Address, value *big.Int) error {
	defer func() {
		lg.PendingMu.Lock()
		if lg.MaxPending != 0 {
			lg.Pending--
		}
		lg.PendingMu.Unlock()
	}()

	abi := contract.AbiFor("StableToken")
	stableToken := bind.NewBoundContract(env.MustProxyAddressFor("StableToken"), *abi, client)

	transactor := bind.NewKeyedTransactor(acc.PrivateKey)
	transactor.Context = ctx
	transactor.Nonce = new(big.Int).SetUint64(nonce)

	stableTokenAddress := env.MustProxyAddressFor("StableToken")
	transactor.FeeCurrency = &stableTokenAddress
	if skipEstimation {
		transactor.GasLimit = GasForTransferWithComment
	}

	tx, err := stableToken.TxObj(transactor, "transferWithComment", recipient, value, "need to proivde some long comment to make it similar to an encrypted comment").Send()
	if err != nil {
		if err != context.Canceled {
			fmt.Printf("Error sending transaction: %v\n", err)
		}
		return fmt.Errorf("Error sending transaction: %w", err)
	}
	if verbose {
		fmt.Printf("cusd transfer generated: from: %s to: %s amount: %s\ttxhash: %s\n", acc.Address.Hex(), recipient.Hex(), value.String(), tx.Transaction.Hash().Hex())
		printJSON(tx)
	}

	_, err = tx.WaitMined(ctx)

	if err != nil {
		if err != context.Canceled {
			fmt.Printf("Error waiting for tx: %v\n", err)
		}
		return fmt.Errorf("Error waiting for tx: %w", err)
	}
	return err
}

func printJSON(obj interface{}) {
	b, _ := json.MarshalIndent(obj, " ", " ")
	fmt.Println(string(b))
}
