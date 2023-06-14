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
