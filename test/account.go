package test

import (
	"context"
	"crypto/ecdsa"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/params"
)

type Account struct {
	Address     common.Address
	Key         *ecdsa.PrivateKey
	ChainConfig *params.ChainConfig
	Nonce       *uint64
}

func NewAccount(key *ecdsa.PrivateKey, address common.Address, chainConfig *params.ChainConfig) *Account {
	return &Account{
		Address:     address,
		Key:         key,
		ChainConfig: chainConfig,
	}
}

// SendCelo submits a value transfer transaction via the provided Node to send
// celo to the recipient. The submitted transaction is returned.
func (a *Account) SendCelo(ctx context.Context, recipient common.Address, value int64, node *Node, calldatasize ...int) (*types.Transaction, error) {
	var err error
	// Lazy set nonce
	if a.Nonce == nil {
		a.Nonce = new(uint64)
		*a.Nonce, err = node.WsClient.PendingNonceAt(ctx, a.Address)
		if err != nil {
			return nil, err
		}
	}
	num, err := node.WsClient.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}

	signer := types.MakeSigner(a.ChainConfig, new(big.Int).SetUint64(num))
	tx, err := ValueTransferTransaction(
		node.WsClient,
		a.Key,
		a.Address,
		recipient,
		*a.Nonce,
		big.NewInt(value),
		signer,
		calldatasize...)

	if err != nil {
		return nil, err
	}
	err = node.WsClient.SendTransaction(ctx, tx)
	if err != nil {
		return nil, err
	}
	*a.Nonce++
	return tx, nil
}

// SendCelo submits a value transfer transaction via the provided Node to send
// celo to the recipient. The submitted transaction is returned.
func (a *Account) SendCeloWithDynamicFee(ctx context.Context, recipient common.Address, value int64, gasFeeCap *big.Int, gasTipCap *big.Int, node *Node) (*types.Transaction, error) {
	var err error
	// Lazy set nonce
	if a.Nonce == nil {
		a.Nonce = new(uint64)
		*a.Nonce, err = node.WsClient.PendingNonceAt(ctx, a.Address)
		if err != nil {
			return nil, err
		}
	}
	num, err := node.WsClient.BlockNumber(ctx)
	if err != nil {
		return nil, err
	}

	signer := types.MakeSigner(a.ChainConfig, new(big.Int).SetUint64(num))
	tx, err := ValueTransferTransactionWithDynamicFee(
		node.WsClient,
		a.Key,
		a.Address,
		recipient,
		*a.Nonce,
		big.NewInt(value),
		gasFeeCap,
		gasTipCap,
		signer)

	if err != nil {
		return nil, err
	}
	err = node.WsClient.SendTransaction(ctx, tx)
	if err != nil {
		return nil, err
	}
	*a.Nonce++
	return tx, nil
}

// SendCeloTracked functions like SendCelo but also waits for the transaction
// to be processed by the sending node.
func (a *Account) SendCeloTracked(ctx context.Context, recipient common.Address, value int64, node *Node) (*types.Transaction, error) {
	tx, err := a.SendCelo(ctx, recipient, value, node)
	if err != nil {
		return nil, err
	}
	err = node.AwaitTransactions(ctx, tx)
	if err != nil {
		return nil, err
	}
	return node.Tracker.GetProcessedTx(tx.Hash()), nil
}

// Accounts converts a slice of env.Account objects to Account objects.
func Accounts(accts []env.Account, chainConfig *params.ChainConfig) []*Account {
	accounts := make([]*Account, 0, len(accts))
	for _, a := range accts {
		accounts = append(accounts, NewAccount(a.PrivateKey, a.Address, chainConfig))
	}
	return accounts
}
