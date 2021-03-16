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
}

// Start will start loads bots
func Start(ctx context.Context, cfg *Config) error {
	// Set up round robin selectors that rollover
	recvIdx := uint(len(cfg.Accounts) / 2)
	nextRecipient := func() common.Address {
		recvIdx++
		return cfg.Accounts[recvIdx%uint(len(cfg.Accounts))].Address
	}
	sendIdx := uint(0)
	nextSender := func() env.Account {
		sendIdx++
		return cfg.Accounts[sendIdx%uint(len(cfg.Accounts))]
	}
	clientIdx := uint(0)
	nextClient := func() *ethclient.Client {
		clientIdx++
		return cfg.Clients[clientIdx%uint(len(cfg.Clients))]
	}

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
			group.Go(func() error {
				return runTransaction(ctx, lg, nextSender(), cfg.Verbose, nextClient(), nextRecipient(), cfg.Amount)
			})
		case <-ctx.Done():
			return group.Wait()
		}
	}
}

func runTransaction(ctx context.Context, lg *LoadGenerator, acc env.Account, verbose bool, client *ethclient.Client, recipient common.Address, value *big.Int) error {
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

	stableTokenAddress := env.MustProxyAddressFor("StableToken")
	transactor.FeeCurrency = &stableTokenAddress

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
