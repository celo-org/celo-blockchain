package ledger

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestCUSD(t *testing.T) {
	tokens, _ := LoadTokens(TokensBlob)
	cUSDAddress, _ := hex.DecodeString("ee21fae7d422c551e59ec68f56b6899e149537c1")
	var cUSDAddressFixedSize common.Address
	copy(cUSDAddressFixedSize[:], cUSDAddress)
	cUSDInfo, err := tokens.ByContractAddressAndChainID(cUSDAddressFixedSize, big.NewInt(1101))
	if err != nil {
		t.Fatal("Could not find cUSD")
	}
	if cUSDInfo.Ticker != "cUSD" {
		t.Fatalf("Ticker incorrect: %s", cUSDInfo.Ticker)
	}
	if cUSDInfo.Decimals != 18 {
		t.Fatalf("Decimals incorrect: %d", cUSDInfo.Decimals)
	}
	if cUSDInfo.ChainID != 1101 {
		t.Fatalf("ChainID incorrect: %d", cUSDInfo.ChainID)
	}
}
