package e2e_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/require"
)

func TestSendCelo(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))
	accounts := test.Accounts(3)
	gc := test.GenesisConfig(accounts)
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

	_, err = network[0].SendCeloTracked(ctx, network[1].DevAddress, 1)
	require.NoError(t, err)
}
