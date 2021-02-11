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

type Range struct {
	From *big.Int
	To   *big.Int
}

type LoadBotConfig struct {
	Accounts         []env.Account
	Amount           *big.Int
	TransactionDelay time.Duration
	ClientFactory    func() (*ethclient.Client, error)
}

func Start(ctx context.Context, cfg *LoadBotConfig) error {
	group, ctx := errgroup.WithContext(ctx)

	nextTransfer := func() (common.Address, *big.Int) {
		idx := rand.Intn(len(cfg.Accounts))
		return cfg.Accounts[idx].Address, cfg.Amount
	}

	for _, acc := range cfg.Accounts {
		acc := acc
		group.Go(func() error {
			client, err := cfg.ClientFactory()
			if err != nil {
				return err
			}
			return runBot(ctx, acc, cfg.TransactionDelay, client, nextTransfer)
		})

	}

	return group.Wait()
}

func runBot(ctx context.Context, acc env.Account, sleepTime time.Duration, client bind.ContractBackend, nextTransfer func() (common.Address, *big.Int)) error {
	abi := contract.AbiFor("StableToken")
	stableToken := bind.NewBoundContract(common.HexToAddress("0xd008"), *abi, client)

	transactor := bind.NewKeyedTransactor(acc.PrivateKey)
	transactor.Context = ctx

	for {
		err := waitFor(ctx, sleepTime)
		if err != nil {
			return err
		}

		recipient, value := nextTransfer()
		tx, err := stableToken.TxObj(transactor, "transfer", recipient, value).Send()
		if err != nil {
			return fmt.Errorf("Error sending transaction: %w", err)
		}
		fmt.Printf("cusd transfer generated: from: %s to: %s amount: %s\ttxhash: %s\n", acc.Address.Hex(), recipient.Hex(), value.String(), tx.Transaction.Hash().Hex())

		printJSON(tx)
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
