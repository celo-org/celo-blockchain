// Copyright 2017 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
       "math/big"
)

type exchangeRate struct {
        Numerator     *big.Int
	Denominator   *big.Int
}

type PriceComparator struct {
	exchangeRates map[uint64]*exchangeRate // indexedCurrency:CeloGold exchange rate	
}

func (pc *PriceComparator) Cmp(val1 *big.Int, currency1 uint64, val2 *big.Int, currency2 uint64) int {
	exchangeRate1 := pc.exchangeRates[currency1]
	exchangeRate1Num := exchangeRate1.Numerator
	exchangeRate1Den := exchangeRate1.Denominator

	exchangeRate2 := pc.exchangeRates[currency2]
	exchangeRate2Num := exchangeRate2.Numerator
	exchangeRate2Den := exchangeRate2.Denominator

	// Basically checking this:
	// val1 * exchangeRate1Num/exchangeRate1Den < val2 * exchangeRate2Num/exchangeRate2Den
	// Will transform that equality to this:
	// val1 * exchangeRate1Num * exchangeRate2Den < val2 * exchangeRate2Num * exchangeRate1Den
	leftSide := new(big.Int).Mul(val1, new(big.Int).Mul(exchangeRate1Num, exchangeRate2Den))
	rightSide := new(big.Int).Mul(val2, new(big.Int).Mul(exchangeRate2Num, exchangeRate1Den))
	return leftSide.Cmp(rightSide)
}

func NewPriceComparator() *PriceComparator {
        return &PriceComparator{
	       exchangeRates: nil,
	}
}