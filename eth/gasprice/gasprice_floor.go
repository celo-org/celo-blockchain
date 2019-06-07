package gasprice

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

func CalculateGasPriceFloor(header *types.Header, oldGasPrice *big.Int, targetDensity *big.Int, adjSpeed *big.Int) (*big.Int, error) {
	log.Info("Header:", "header", header)
	log.Info("oldGasPrice:", "gas price", oldGasPrice)
	log.Info("targetDensity", "target", targetDensity)
	log.Info("gasUsed", "gasUsed", header.GasUsed)
	log.Info("gasLimit", "limit", header.GasLimit)
	log.Info("adjustment speed", "adjSpeed", adjSpeed)
	if oldGasPrice == nil || targetDensity == nil || adjSpeed == nil {
		return nil, errors.New("Invalid gas price calculation parameters")
	}

	// TODO: Clearer variable names
	denom := big.NewInt(1000)
	divisor := new(big.Int).Exp(denom, big.NewInt(2), nil) // TODO (jarmg 6/6/19): remove magic number
	one := new(big.Int).Exp(denom, big.NewInt(2), nil)
	gasUsed := new(big.Int).Mul(big.NewInt(int64(header.GasUsed)), denom)

	density := new(big.Int).Div(gasUsed, big.NewInt(int64(header.GasLimit)))
	distanceFromTarget := new(big.Int).Sub(density, targetDensity)
	gasPriceAdjustment := new(big.Int).Mul(distanceFromTarget, adjSpeed)
	log.Info("Adjusting Gas Price Floor", "Adjustment percentage", new(big.Int).Div(gasPriceAdjustment, big.NewInt(10000)))
	adjustmentMultiplier := new(big.Int).Add(one, gasPriceAdjustment)
	undividedNewGasPriceFloor := new(big.Int).Mul(oldGasPrice, adjustmentMultiplier)
	finalNewGasPriceFloor := new(big.Int).Div(undividedNewGasPriceFloor, divisor)

	return finalNewGasPriceFloor, nil
}
