package ethapi

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

// So I got the tx from block 470 and decoded it it has a protected V value, but
// it seems that signers for protected V were not added until espresso, so I am
// wondering whatactually happened here? This test fails if the espresso block is not enabled.
func TestTxDes(t *testing.T) {
	tx470 := "0xf903688085174876e8008401312d008080808080b9030e608060405234801561001057600080fd5b50336000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055506102ae806100606000396000f3fe608060405234801561001057600080fd5b506004361061004c5760003560e01c80630900f01014610051578063445df0ac146100955780638da5cb5b146100b3578063fdacd576146100fd575b600080fd5b6100936004803603602081101561006757600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061012b565b005b61009d6101f7565b6040518082815260200191505060405180910390f35b6100bb6101fd565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6101296004803603602081101561011357600080fd5b8101908080359060200190929190505050610222565b005b6000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff1614156101f45760008190508073ffffffffffffffffffffffffffffffffffffffff1663fdacd5766001546040518263ffffffff1660e01b815260040180828152602001915050600060405180830381600087803b1580156101da57600080fd5b505af11580156101ee573d6000803e3d6000fd5b50505050505b50565b60015481565b6000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1681565b6000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff16141561027f57806001819055505b5056fea165627a7a72305820ec6f958d6d8037237d9bbd9bb640bbc5805f9055a77f8dcdca49cc17f7f5f548002983015e0aa0cea4352d1b59ce5a3117b38e1ebe3dd5c70f26da9c9c966652443f51da911c4aa05bca5245404f6497591e6f654fd1429569c0cea1f5d72f03ac5ea1e7e91052f5"
	//txhex := "0x80e5074400000000000000000000000010c892a6ec43a53e45d0b916b4b7d383b1b78c0f00000000000000000000000000000000000000000001e2092887279c5176bea000000000000000000000000072710d8a8bdc7a2476cde04050849d29e873591c000000000000000000000000658d897ed714a250ebf979de19788dfb065b52eb"
	//tx2ex := "0xf8f2830235c58423c346008304890d80808094fdd8bd58115ffbf04e47411c1d228ecc45e9307580b88480e5074400000000000000000000000010c892a6ec43a53e45d0b916b4b7d383b1b78c0f00000000000000000000000000000000000000000001e2092887279c5176bea000000000000000000000000072710d8a8bdc7a2476cde04050849d29e873591c000000000000000000000000658d897ed714a250ebf979de19788dfb065b52eb83015e09a0b91ae8b811f1ac86d51450b3f7e0047cc1c2e99da507e2daa3359518264d926fa0353a21b67608d6b4d709292e5150bc9f9783a95421157e250f843dea5241a71f"
	chosen := tx470
	txBytes := hexutil.MustDecode(chosen)
	tx := &types.Transaction{}

	err := rlp.DecodeBytes(txBytes, tx)
	//err := tx.UnmarshalBinary(txBytes)
	require.NoError(t, err)
	fmt.Printf("hash: %s\n", tx.Hash().Hex())
	spew.Dump(tx)

	// config := &params.ChainConfig{
	// 	ChainID:        big.NewInt(44787),
	// 	HomesteadBlock: big.NewInt(0),
	// 	EIP150Block:    big.NewInt(0),
	// 	// EspressoBlock:  big.NewInt(0),
	// }
	blockNumber := uint64(100)
	number := new(big.Int).SetUint64(blockNumber)
	s := types.MakeSigner(params.BaklavaChainConfig, number)
	from, err := s.Sender(tx)
	require.NoError(t, err)
	require.Equal(t, "0x456f41406b32c45d59e539e4bba3d7898c3584da", strings.ToLower(from.String()))

	// key, err := crypto.GenerateKey()
	// require.NoError(t, err)
	// signed, err := types.SignTx(tx, s, key)
	// spew.Dump(signed)
}
