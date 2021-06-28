package ledger

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestCUSD(t *testing.T) {
	tokens, _ := LoadTokens(TokensBlob)
	cUSDAddress, _ := hex.DecodeString("765de816845861e75a25fca122bb6898b8b1282a")
	var cUSDAddressFixedSize common.Address
	copy(cUSDAddressFixedSize[:], cUSDAddress)
	cUSDInfo, err := tokens.ByContractAddressAndChainID(cUSDAddressFixedSize, big.NewInt(42220))
	if err != nil {
		t.Fatal("Could not find cUSD")
	}
	if cUSDInfo.Ticker != "cUSD" {
		t.Fatalf("Ticker incorrect: %s", cUSDInfo.Ticker)
	}
	if cUSDInfo.Decimals != 18 {
		t.Fatalf("Decimals incorrect: %d", cUSDInfo.Decimals)
	}
	if cUSDInfo.ChainID != 42220 {
		t.Fatalf("ChainID incorrect: %d", cUSDInfo.ChainID)
	}
}
