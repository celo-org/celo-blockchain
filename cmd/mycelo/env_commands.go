package main

import (
	"fmt"

	"github.com/celo-org/celo-blockchain/cmd/utils"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"gopkg.in/urfave/cli.v1"
)

var envCommand = cli.Command{
	Name:  "env",
	Usage: "Environment utility commands",
	Subcommands: []cli.Command{
		getAccountCommand,
	},
}

var getAccountCommand = cli.Command{
	Name:   "account",
	Action: getAccount,
	Flags: []cli.Flag{
		idxFlag,
		accountTypeFlag,
	},
}

var (
	idxFlag = cli.IntFlag{
		Name:  "idx",
		Usage: "account index",
		Value: 0,
	}
	accountTypeFlag = utils.TextMarshalerFlag{
		Name:  "type",
		Usage: `Account type (validator, developer, txNode, faucet, attestation, priceOracle, attestationBot, votingBot, txNodePrivate, validatorGroup, admin)`,
		Value: &env.DeveloperAT,
	}
)

func getAccount(ctx *cli.Context) error {
	myceloEnv, err := readEnv(ctx)
	if err != nil {
		return err
	}

	idx := ctx.Int(idxFlag.Name)
	accountType := *utils.LocalTextMarshaler(ctx, accountTypeFlag.Name).(*env.AccountType)

	account, err := myceloEnv.Accounts().Account(accountType, idx)
	if err != nil {
		return err
	}

	fmt.Printf("AccountType: %s\nIndex:%d\nAddress: %s\nPrivateKey: %s\n", accountType, idx, account.Address.Hex(), account.PrivateKeyHex())
	return nil
}
