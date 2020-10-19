package bls12377

import (
	"math/big"
)

type nafNumber []int

var bigZero = big.NewInt(0)
var bigOne = big.NewInt(1)

func toWNAF(e *big.Int, w int) nafNumber {

	naf := nafNumber{}
	k := new(big.Int).Lsh(bigOne, uint(w))
	halfK := new(big.Int).Rsh(k, 1)

	ee := new(big.Int).Set(e)
	for ee.Cmp(bigZero) != 0 {

		if ee.Bit(0) == 1 {

			nafSign := new(big.Int)
			nafSign.Mod(ee, k)

			if nafSign.Cmp(halfK) >= 0 {
				nafSign.Sub(nafSign, k)
			}

			naf = append(naf, int(nafSign.Int64()))

			ee.Sub(ee, nafSign)

		} else {
			naf = append(naf, 0)
		}

		ee.Rsh(ee, 1)
	}
	return naf
}

func fromWNAF(naf nafNumber, w int) *big.Int {
	acc := new(big.Int)
	k := new(big.Int).Set(bigOne)
	for i := 0; i < len(naf); i++ {
		if naf[i] != 0 {
			z := new(big.Int).Mul(k, big.NewInt(int64(naf[i])))
			acc.Add(acc, z)
		}
		k.Lsh(k, 1)
	}
	return acc
}
