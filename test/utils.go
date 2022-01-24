package test

import (
	"context"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/ethclient"
)

type Token int

type Balances map[common.Address]*big.Int

// BalanceWatcher is a helper to watch balance changes over addresses.
type BalanceWatcher struct {
	accounts []common.Address
	client   *ethclient.Client
	initial  Balances
	current  Balances
}

// NewBalanceWatcher creates a BalanceWatcher, records initial balances of the accounts.
func NewBalanceWatcher(client *ethclient.Client, addresses []common.Address) *BalanceWatcher {
	watcher := &BalanceWatcher{
		client:   client,
		accounts: addresses,
	}
	if initial, err := fetch(client, addresses); err != nil {
		panic("Unable to initialize Balances")
	} else {
		watcher.initial = initial
		watcher.current = initial
	}

	return watcher
}

// Initial returns initial balance of given address, or nil if address is not found.
func (w *BalanceWatcher) Initial(address common.Address) *big.Int {
	if _, ok := w.initial[address]; ok {
		if balance, ok := w.initial[address]; ok {
			return balance
		}
	}
	return nil
}

// Current returns current balance of given address.
func (w *BalanceWatcher) Current(address common.Address) *big.Int {
	return w.current[address] // TODO: support stable coin
}

// Delta returns difference between current and initial balance of given address for given token.
func (w *BalanceWatcher) Delta(address common.Address) *big.Int {
	return new(big.Int).Sub(w.current[address], w.initial[address]) // TODO: support stable coin
}

// Update updates current balances in the BalanceWatcher
func (w *BalanceWatcher) Update() {
	if current, err := fetch(w.client, w.accounts); err != nil {
		panic("Unable to fetch current balances")
	} else {
		w.current = current
	}
}

// fetch retrieves balances of given addresses.
func fetch(client *ethclient.Client, addresses []common.Address) (Balances, error) {
	balances := make(Balances)
	for _, address := range addresses {
		balance, err := client.BalanceAt(context.Background(), address, nil)
		if err != nil {
			return nil, err
		}
		balances[address] = balance // TODO: support stable coin
	}
	return balances, nil
}
