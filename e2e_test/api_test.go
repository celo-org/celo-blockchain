package e2e_test

import (
	"context"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/abis"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/require"
)

var (
	getGasPriceMinimumMethod = contracts.NewRegisteredContractMethod(params.GasPriceMinimumRegistryId, abis.GasPriceMinimum, "getGasPriceMinimum", params.MaxGasForGetGasPriceMinimum)
)

func TestGasEstimationOnPendingBlock(t *testing.T) {
	gpmAddr := env.MustProxyAddressFor("GasPriceMinimum")
	ac := test.AccountConfig(1, 1)
	gc, ec, err := test.BuildConfig(ac)
	require.NoError(t, err)

	// Prevent block changes
	ec.Istanbul.BlockPeriod = math.MaxUint64

	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	var gasPriceMinimum *big.Int
	packed, err := abis.GasPriceMinimum.Pack("getGasPriceMinimum", env.MustProxyAddressFor("GoldToken"))
	require.NoError(t, err)
	ret, _, err = evm.StaticCall(vm.AccountRef(evm.Origin), recipient, input, gas)

	// err = getGasPriceMinimumMethod.Query(vmRunner, &gasPriceMinimum, currencyAddress)

	abis.GasPriceMinimum.one
	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)
}
