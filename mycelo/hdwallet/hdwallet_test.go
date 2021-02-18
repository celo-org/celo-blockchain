package hdwallet

import (
	"fmt"
	"testing"
)

func Test(t *testing.T) {
	mnemonic := "tag volcano eight thank tide danger coast health above argue embrace heavy"
	wallet, err := NewFromMnemonic(mnemonic)
	if err != nil {
		t.Fatal(err)
	}

	path := MustParseDerivationPath("m/1/0")
	account, err := wallet.Derive(path, false)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(account.Address.Hex()) // 0xC49926C4124cEe1cbA0Ea94Ea31a6c12318df947

}
