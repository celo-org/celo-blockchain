package e2e_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func TestSendCelo(t *testing.T) {
	// log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))
	accounts := test.Accounts(3)
	gc := test.GenesisConfig(accounts)

	fmt.Printf("%+v\n", gc.Istanbul)

	gc.Istanbul = params.IstanbulConfig{
		BlockPeriod:    0,
		RequestTimeout: 50,
		Epoch:          10,
		LookbackWindow: 5,
		ProposerPolicy: uint64(istanbul.ShuffledRoundRobin),
	}

	fmt.Printf("%+v\n", gc.Istanbul)
	network, err := test.NewNetwork(accounts, gc)
	require.NoError(t, err)
	defer network.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	for i := range network {
		addr := network[i].DevAddress
		bal, err := network[i].WsClient.BalanceAt(ctx, addr, common.Big0)
		require.NoError(t, err)
		fmt.Printf("Addr: %v, Bal: %v\n", addr.String(), bal.String())
	}

	tx, err := network[0].SendCeloTracked(ctx, network[1].DevAddress, 1)
	require.NoError(t, err)
	receipt, err := network[0].WsClient.TransactionReceipt(ctx, tx.Hash())
	spew.Dump(receipt)
	time.Sleep(20 * time.Second)
}
