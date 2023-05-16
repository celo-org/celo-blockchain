package ethapi

import (
	"context"
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/currency"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rpc"
)

// CheckTxFee is an internal function used to check whether the fee of
// the given transaction is _reasonable_(under the cap).
func CheckTxFee(cp currency.Provider, feeCurrencyAddress *common.Address, fee *big.Int, cap float64) error {
	// Short circuit if there is no cap for transaction fee at all.
	if cap == 0 {
		return nil
	}
	weiCap := getWei(cap)
	feeCurrency, err := cp.GetCurrency(feeCurrencyAddress)
	if err != nil {
		return fmt.Errorf("Can't check tx fee cap: %s", err)
	}
	if feeCurrency.CmpToCurrency(fee, weiCap, &currency.CELOCurrency) > 0 {
		feeFloat := float64(fee.Uint64())
		feeFloat /= params.Ether
		if feeCurrencyAddress != nil {
			return fmt.Errorf("tx fee (%.2f of currency address '%s') exceeds the configured cap (%.2f celo)", feeFloat, feeCurrencyAddress.Hex(), cap)
		} else {
			return fmt.Errorf("tx fee (%.2f of currency celo) exceeds the configured cap (%.2f celo)", feeFloat, cap)
		}
	}
	return nil
}

// getWei converts a celo float to a big.Int Wei representation
func getWei(celo float64) *big.Int {
	floatWei := new(big.Float).Mul(big.NewFloat(params.Ether), big.NewFloat(celo))
	wei, _ := floatWei.Int(nil)
	return wei
}

func checkFeeFromCeloTx(ctx context.Context, b Backend, tx *types.Transaction) error {
	currencyManager, err := newCurrencyManager(ctx, b)
	if err != nil {
		return err
	}
	return CheckTxFee(currencyManager, tx.FeeCurrency(), tx.Fee(), b.RPCTxFeeCap())
}

func checkFeeFromCeloCurrency(ctx context.Context, b Backend, feeCurrency *common.Address, gasPrice *big.Int, gas uint64) error {
	currencyManager, err := newCurrencyManager(ctx, b)
	if err != nil {
		return err
	}
	fee := types.Fee(gasPrice, gas)
	return CheckTxFee(currencyManager, feeCurrency, fee, b.RPCTxFeeCap())
}

// newCurrencyManager creates and returns a currencyManager pointing to the latest block
// from the underlying chain from the Backend.
func newCurrencyManager(ctx context.Context, b Backend) (*currency.CurrencyManager, error) {
	stateDb, header, err := b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if err != nil {
		return nil, err
	}

	vmRunner := b.NewEVMRunner(header, stateDb)
	return currency.NewManager(vmRunner), nil
}
