package loadbot

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	bind "github.com/ethereum/go-ethereum/accounts/abi/bind_v2"
	"github.com/ethereum/go-ethereum/common"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/mycelo/contract"
	"github.com/ethereum/go-ethereum/mycelo/env"
	"golang.org/x/sync/errgroup"
)

const clientCap = 100

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
	ClientFactory         func() (*ethclient.Client, error)
}

// Start will start loads bots
func Start(ctx context.Context, cfg *Config) error {
	group, ctx := errgroup.WithContext(ctx)

	nextTransfer := func() (common.Address, *big.Int) {
		idx := rand.Intn(len(cfg.Accounts))
		return cfg.Accounts[idx].Address, cfg.Amount
	}

	// Use no more than clientCap clients
	clientCount := len(cfg.Accounts)
	if clientCount > clientCap {
		clientCount = clientCap
	}
	clients := make([]bind.ContractBackend, 0, clientCount)
	for i := 0; i < clientCount; i++ {
		client, err := cfg.ClientFactory()
		if err != nil {
			return err
		}
		clients = append(clients, client)
	}

	// devloper accounts / TPS = duration in seconds. Need the fudger factor to get up a consistent TPS at the target.
	delay := time.Duration(int(float64(len(cfg.Accounts)*1000/cfg.TransactionsPerSecond)*0.95)) * time.Millisecond
	startDelay := delay / time.Duration(len(cfg.Accounts))

	for i, acc := range cfg.Accounts {
		// Spread out client load accross different diallers
		client := clients[i%clientCount]

		err := waitFor(ctx, startDelay)
		if err != nil {
			return err
		}
		acc := acc
		group.Go(func() error {
			return runBot(ctx, acc, delay, client, nextTransfer)
		})

	}

	return group.Wait()
}

func runBot(ctx context.Context, acc env.Account, sleepTime time.Duration, client bind.ContractBackend, nextTransfer func() (common.Address, *big.Int)) error {
	abi := contract.AbiFor("StableToken")
	stableToken := bind.NewBoundContract(env.MustAddressFor("StableToken"), *abi, client)

	transactor := bind.NewKeyedTransactor(acc.PrivateKey)
	transactor.Context = ctx

	for {
		txSentTime := time.Now()
		recipient, value := nextTransfer()
		tx, err := stableToken.TxObj(transactor, "transfer", recipient, value).Send()
		if err != nil {
			if err != context.Canceled {
				fmt.Printf("Error sending transaction: %v\n", err)
			}
			return fmt.Errorf("Error sending transaction: %w", err)
		}
		// fmt.Printf("cusd transfer generated: from: %s to: %s amount: %s\ttxhash: %s\n", acc.Address.Hex(), recipient.Hex(), value.String(), tx.Transaction.Hash().Hex())

		// printJSON(tx)
		_, err = tx.WaitMined(ctx)
		if err != nil {
			if err != context.Canceled {
				fmt.Printf("Error waiting for tx: %v\n", err)
			}
			return fmt.Errorf("Error waitin for tx: %w", err)
		}

		nextSendTime := txSentTime.Add(sleepTime)

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
