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
		curr := "celo"
		if feeCurrencyAddress != nil {
			feeFloat /= params.Ether
			curr = feeCurrencyAddress.Hex()
		}
		return fmt.Errorf("tx fee (%.2f at address '%s') exceeds the configured cap (%.2f celo)", feeFloat, curr, cap)
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

func checkFeeFromCeloArgs(ctx context.Context, b Backend, args SendTxArgs) error {
	return checkFeeFromCeloCurrency(ctx, b, args.FeeCurrency, (*big.Int)(args.GasPrice), uint64(*args.Gas), (*big.Int)(args.GatewayFee))
}

func checkFeeFromCeloCurrency(ctx context.Context, b Backend, feeCurrency *common.Address, gasPrice *big.Int, gas uint64, gatewayFee *big.Int) error {
	currencyManager, err := newCurrencyManager(ctx, b)
	if err != nil {
		return err
	}
	gFee := gatewayFee
	if gFee == nil {
		gFee = big.NewInt(0)
	}
	fee := types.Fee(gasPrice, gas, gFee)
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
