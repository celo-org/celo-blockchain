package ethapi

import (
	"errors"
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/currency"
	"github.com/celo-org/celo-blockchain/params"
)

var mockCurrencyAddress = common.HexToAddress("01010101010101")
var noCurrError = errors.New("noCurrErr")
var mockRate, _ = currency.NewExchangeRate(common.Big2, big.NewInt(7*params.Ether)) // 3.5 celo per mock token

type mockProvider struct{}

func (mp *mockProvider) GetCurrency(currencyAddress *common.Address) (*currency.Currency, error) {
	if currencyAddress == nil {
		return &currency.CELOCurrency, nil
	}
	if currencyAddress.Hash() != mockCurrencyAddress.Hash() {
		return nil, noCurrError
	}
	return currency.NewCurrency(mockCurrencyAddress, *mockRate), nil
}

func TestCheckTxFeeCap_Currency(t *testing.T) {
	p := &mockProvider{}
	fee := big.NewInt(50) // * mockRate == 175 celo
	if err := CheckTxFee(p, &mockCurrencyAddress, fee, 175.0+0.1); err != nil {
		t.Fatal("Failed tx fee cap check on non-celo currency: false negative", err)
	}
	if err := CheckTxFee(p, &mockCurrencyAddress, fee, 175.0-0.1); err == nil {
		t.Fatal("Failed tx fee cap check on non-celo currency: false positive")
	}
}

func TestCheckTxFeeCap_CeloCurrency(t *testing.T) {
	p := &mockProvider{}
	fee := big.NewInt(2 * params.Ether)
	if err := CheckTxFee(p, nil, fee, 2.0+0.1); err != nil {
		t.Fatal("Failed tx fee cap check on non-celo currency: false negative", err)
	}
	if err := CheckTxFee(p, nil, fee, 2.0-0.1); err == nil {
		t.Fatal("Failed tx fee cap check on non-celo currency: false positive")
	}
}

func TestCheckTxFeeCap_NoCurrency(t *testing.T) {
	p := &mockProvider{}
	addr := common.HexToAddress("FFF")
	if err := CheckTxFee(p, &addr, common.Big1, 999999999); err == nil {
		t.Fatal("Failed tx fee cap check on non-celo currency: false positive")
	}
}
