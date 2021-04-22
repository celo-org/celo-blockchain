package e2e_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func TestNetworkStartupShutdown(t *testing.T) {
	// log.Root().SetHandler(log.LvlFilterHandler(log.LvlDebug, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))
	now := time.Now()
	println("starting test", now.String())
	network, err := test.NewNetworkFromUsers()
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
	spew.Dump(tx)
	println("test took", time.Since(now).String())
}
