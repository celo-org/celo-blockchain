package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEthereumHeaderHashCompatibility(t *testing.T) {
	// Ethereum tools calculating block header hashes show work on Celo just as
	// the do on Ethereum. To verify that this is the case, we import an
	// Ethereum header, calculate the hash and show that we calculate the same
	// hash as the header has on Ethereum.

	// Last Ethereum block before the merge: https://etherscan.io/block/15537393
	ethereumJsonHeader := []byte(`{
		"number": "0xed14f1",
		"hash": "0x55b11b918355b1ef9c5db810302ebad0bf2544255b530cdce90674d5887bb286",
		"parentHash": "0x2b3ea3cd4befcab070812443affb08bf17a91ce382c714a536ca3cacab82278b",
		"sha3Uncles": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
		"logsBloom": "0x00000400000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000080000000000000000000000008000000000000000000000000000000000000000000000000020000000000000000000800000000004000000000000010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008400000000001002000000000000000000000000000000000002000000020000000020000000000000000000000000000000000000000040000000000000000000000000",
		"transactionsRoot": "0xdd5eec02b019ff76e359b09bfa19395a2a0e97bc01e70d8d5491e640167c96a8",
		"stateRoot": "0x4919dafa6ac8becfbbd0c2808f6c9511a057c21e42839caff5dfb6d3ef514951",
		"receiptsRoot": "0xbaa842cfd552321a9c2450576126311e071680a1258032219c6490b663c1dab8",
		"miner": "0x829bd824b016326a401d083b33d092293333a830",
		"difficulty": "0x27472e1db3626a",
		"totalDifficulty": "0xc70d815d562d3cfa955",
		"extraData": "0xe4b883e5bda9e7a59ee4bb99e9b1bc460021",
		"size": "0x664",
		"gasLimit": "0x1c9c380",
		"gasUsed": "0x1c9a205",
		"timestamp": "0x6322c962",
		"uncles": [],
		"baseFeePerGas": "0xa1a4e5f06",
		"nonce": "0x62a3ee77461d4fc9",
		"mixHash": "0x4cbec03dddd4b939730a7fe6048729604d4266e82426d472a2b2024f3cc4043f"
	}`)
	var h Header
	var jsonHeader map[string]interface{}
	require.NoError(t, json.Unmarshal(ethereumJsonHeader, &jsonHeader))
	require.NoError(t, json.Unmarshal(ethereumJsonHeader, &h))
	require.Equal(t, jsonHeader["hash"], h.Hash().Hex())
}

func TestCeloBeforeGingerbreadHeaderHashCompatibility(t *testing.T) {
	// Before Gingerbread, certain header fields were not included in the
	// header hash calculation. Blocks generated before Gingerbread must keep
	// the same hash, even though we added fields for later blocks.

	// The `difficulty` field has been manually removed from the RPC result. It
	// is only added to old blocks by the RPCEthCompatibility option.
	celoJsonHeader := []byte(`{
		"epochSnarkData": {
		  "bitmap": "0x325007fdf89adf7e7f7e2cfff8ff",
		  "signature": "0x65c0b4238a63c90fbe93619f1f787cc9c24ecb07bb06501fe99bef938d6c74a16beb49593e4e5ead625c244cc20dce80"
		},
		"extraData": "0xd983010704846765746889676f312e31372e3133856c696e7578000000000000f8ccc0c080b84177cdc8f22ff53bd1ed75f2a09b920957d7cbbc01a283c8eb40c7dcf96bbcd41434e06fa9ddafbe61e83dec0a2cb11d5435971242b4a25e6f129768b183e8886700f8418e325007fdf89adf7e7f7e2cfff8ffb05e98abf6071b03bd3b2d420188991b04c874a2cce546e299fef6fccb2b2dc8828e42da4b3dffa483e5b7cf7e79807b8080f8418e3fffffffffffffffffffffffffffb06009638e885c0870c288f816403d0dd04055e4feb3b8e157dc0acdf5f0da3b84636a4c997234969ace1a56084733840080",
		"gasUsed": "0x1063b1",
		"hash": "0xf09d2d8cb7d4ac4c8b49b18030ce09c0be3a40a33a83bb627d27363513907240",
		"logsBloom": "0x2833101222d26aa2000821a9c0a28529948200409056708002002108083a02408908988388a0c480003126142e41a48c0190884321c43010005c8c0242209002379005102fc20c5e6941505a430a7da0085c115008f22f000010199042228c03000006300224d00420341400700408010e4107c09a001021601dc292043d930025843c00830170c00400103002f840421a4500710111981a0434805400d6e8104223b8009121010d0301404a88230c10380200107a2b00050020d18c44159440a0b024222425d2522142f12610eac0910c09612700153911b0a8a510c01060a1da1303a546482c40008a0d282a80b30020402411100f2c00404c305555200035",
		"miner": "0xaa397ca08bbebc31717f8ab2cea35320c51568e6",
		"number": "0x1229100",
		"parentHash": "0x1469ec2ace32ba4f18a7abbc634cf13119a07167b65bc0a2319bb2513625d1c7",
		"randomness": {
		  "committed": "0x60dfa4c93980a2eb42e55df77f7ea945296b63b322ef8d593c955d699992e1aa",
		  "revealed": "0xc22bede1a7919d30eda482820e1b6fd393543cc1dd4bac22b6b915b5812bf2bb"
		},
		"receiptsRoot": "0x7c0af0b3d3d9d750eb8dccffe633794583960b7e72bef95e33aaa0aaf09c58ac",
		"size": "0x5ee",
		"stateRoot": "0x481088b07b8e111f532b8edb2d2881e1747f5d61248b28c096c120b010b336f2",
		"timestamp": "0x644eb9f3",
		"totalDifficulty": "0x1229101",
		"transactions": [
		  "0x8516bb82865b367d6c33b93cb69424b3e30ba19e4a23c95c39a4980cb8a803db",
		  "0x9101d965f19a46eb6246f648fb906a26b966dcb4c5cb001a3422f18974be3dfc",
		  "0xba94878140aefa8a0fc23ae885537224191e3bf4a6d1dcf0706103245a2d73ac",
		  "0xfa8bc080e0925105b16a58a435cf757b510c010509545a6719138babcd52dfea"
		],
		"transactionsRoot": "0x4a1e09afdc7e40256ac262da49396fe7c5bae0b7d9f46e4bc1fe495ef6d8514b"
	}`)
	var h Header
	var jsonHeader map[string]interface{}
	require.NoError(t, json.Unmarshal(celoJsonHeader, &jsonHeader))
	require.NoError(t, json.Unmarshal(celoJsonHeader, &h))
	require.Equal(t, jsonHeader["hash"], h.Hash().Hex())
}
