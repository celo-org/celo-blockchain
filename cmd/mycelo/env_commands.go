package main

import (
	"encoding/json"
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
		jsonFlag,
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
		Usage: `Account type (validator, developer, txNode, faucet, attestation, priceOracle, proxy, attestationBot, votingBot, txNodePrivate, validatorGroup, admin)`,
		Value: &env.DeveloperAT,
	}
	jsonFlag = cli.BoolFlag{
		Name:  "json",
		Usage: "output json",
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

	jsonOutput := ctx.Bool(jsonFlag.Name)

	if jsonOutput {
		output := struct {
			AccountType string `json:"accountType"`
			Index       int    `json:"index"`
			Address     string `json:"address"`
			PrivateKey  string `json:"privateKey"`
		}{
			AccountType: accountType.String(),
			Index:       idx,
			Address:     account.Address.Hex(),
			PrivateKey:  account.PrivateKeyHex(),
		}

		jsonData, err := json.Marshal(output)
		if err != nil {
			return err
		}
		fmt.Println(string(jsonData))
	} else {
		fmt.Printf("AccountType: %s\nIndex:%d\nAddress: %s\nPrivateKey: %s\n", accountType, idx, account.Address.Hex(), account.PrivateKeyHex())
	}
	return nil
}
