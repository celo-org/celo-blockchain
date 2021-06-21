package e2e_test

import (
	"context"
	"encoding/hex"
	"fmt"
	_ "os"
	"testing"
	"time"

	// "github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	_ "github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/require"
)

func init() {
	// This statement is commented out but left here since its very useful for
	// debugging problems and its non trivial to construct.
	//
	// log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))
}

// This test starts a network submits a transaction and waits for the whole
// network to process the transaction.
func TestSendCelo(t *testing.T) {
	t.Skip("")
	accounts := test.Accounts(3)
	gc := test.GenesisConfig(accounts)
	network, err := test.NewNetwork(accounts, gc)
	require.NoError(t, err)
	defer network.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	// Send 1 celo from the dev account attached to node 0 to the dev account
	// attached to node 1.
	tx, err := network[0].SendCelo(ctx, network[1].DevAddress, 1)
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)
}

func MustDecodeTxHex(hs string) *types.Transaction {
	bytes, err := hex.DecodeString(hs)
	if err != nil {
		panic("Failed to decode the hex string")
	}
	var tx types.Transaction
	err = rlp.DecodeBytes(bytes, &tx)
	if err != nil {
		panic("Failed to rlp decode")
	}
	return &tx
}

// This test starts a network submits a transaction and waits for the whole
// network to process the transaction.
func TestUpdateOracleThenSendCUSD(t *testing.T) {
	accounts := test.StableAccounts(1)
	gc := test.GenesisConfig(accounts)
	gc.Istanbul.BlockPeriod = 2

	network, err := test.NewNetwork(accounts, gc)
	require.NoError(t, err)
	defer network.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	oracleHex := "f8ec80841dcd6500830487c980808094000000000000000000000000000000000000d00480b88480e50744000000000000000000000000000000000000000000000000000000000000d00800000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000025a06ae5781a661ea76733b62c3f223588bb0b94da4e427282acace4af5fee7efb76a03bb9f7af12824718655442f99173804785ba35693e40fd151d691b6ede449572"
	cusdHex := "f8c001847735940083011fd394000000000000000000000000000000000000d008808094000000000000000000000000000000000000d00880b844a9059cbb000000000000000000000000e439d548f77115bdf3a650f5d8ff0a3cffe285c2000000000000000000000000000000000000000000000000000000000000000a26a0cc1fe894fced859edb030e0ece4cd24049a28580a9f9ff812123635374da323ba07d6f36f15362ef31ae2344db166a38ded70349fe9e24a72841ab8615ed22c076"

	oracleTx := MustDecodeTxHex(oracleHex)
	cusdTx := MustDecodeTxHex(cusdHex)

	err = network[0].WsClient.SendTransaction(ctx, oracleTx)
	require.NoError(t, err)

	// Send 10 cUSD from the dev account attached to node 0 to the dev account
	// attached to node 1.
	err = network[0].WsClient.SendTransaction(ctx, cusdTx)
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, cusdTx, oracleTx)
	require.NoError(t, err)

	oracleBlock := network[0].ProcessedTxBlock(oracleTx)
	cUSDBlock := network[0].ProcessedTxBlock(cusdTx)

	fmt.Printf("Block Hash: %v\n", oracleBlock.Hash().String())
	fmt.Printf("Block root: %v\n", oracleBlock.Header().Root.String())

	require.Equal(t, oracleBlock.NumberU64(), cUSDBlock.NumberU64())

}
