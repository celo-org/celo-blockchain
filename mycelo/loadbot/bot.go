package loadbot

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	bind "github.com/celo-org/celo-blockchain/accounts/abi/bind_v2"
	"github.com/celo-org/celo-blockchain/common"

	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/mycelo/contract"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"golang.org/x/sync/errgroup"
)

// Range represents an inclusive big.Int range
type Range struct {
	From *big.Int
	To   *big.Int
}

// Config represent the load bot run configuration
type Config struct {
	Accounts              []env.Account
	Amount                *big.Int
	TransactionsPerSecond int
	Clients               []*ethclient.Client
	Verbose               bool
}

// Start will start loads bots
func Start(ctx context.Context, cfg *Config) error {
	group, ctx := errgroup.WithContext(ctx)

	idx := len(cfg.Accounts) / 2
	nextTransfer := func() (common.Address, *big.Int) {
		idx++
		return cfg.Accounts[idx%len(cfg.Accounts)].Address, cfg.Amount
	}

	// developer accounts / TPS = duration in seconds.
	// Need the fudger factor to get up a consistent TPS at the target.
	delay := time.Duration(int(float64(len(cfg.Accounts)*1000/cfg.TransactionsPerSecond)*0.95)) * time.Millisecond
	startDelay := delay / time.Duration(len(cfg.Accounts))

	for i, acc := range cfg.Accounts {
		// Spread out client load across different diallers
		client := cfg.Clients[i%len(cfg.Clients)]
		acc := acc

		err := waitFor(ctx, startDelay)
		if err != nil {
			return err
		}
		group.Go(func() error {
			return runBot(ctx, acc, cfg.Verbose, delay, client, nextTransfer)
		})
	}

	return group.Wait()
}

func runBot(ctx context.Context, acc env.Account, verbose bool, sleepTime time.Duration, client bind.ContractBackend, nextTransfer func() (common.Address, *big.Int)) error {
	abi := contract.AbiFor("StableToken")
	stableToken := bind.NewBoundContract(env.MustProxyAddressFor("StableToken"), *abi, client)

	transactor := bind.NewKeyedTransactor(acc.PrivateKey)
	transactor.Context = ctx

	stableTokenAddress := env.MustProxyAddressFor("StableToken")
	transactor.FeeCurrency = &stableTokenAddress

	for {
		txSentTime := time.Now()
		recipient, value := nextTransfer()
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

		nextSendTime := txSentTime.Add(sleepTime)
		if time.Now().After(nextSendTime) {
			continue
		}

		err = waitFor(ctx, time.Until(nextSendTime))
		if err != nil {
			return err
		}
	}

}

func waitFor(ctx context.Context, waitTime time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(waitTime):
		return nil
	}
}

func printJSON(obj interface{}) {
	b, _ := json.MarshalIndent(obj, " ", " ")
	fmt.Println(string(b))
}
