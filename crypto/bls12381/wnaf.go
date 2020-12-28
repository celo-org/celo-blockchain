package bls12381

import (
	"math/big"
)

type nafNumber []int

func (n nafNumber) neg() {
	for i := 0; i < len(n); i++ {
		n[i] = -n[i]
	}
}

var bigZero = big.NewInt(0)
var bigOne = big.NewInt(1)

// caution: does not cover negative case
func bigToWNAF(e *big.Int, w uint) nafNumber {
	naf := nafNumber{}
	if w == 0 {
		return naf
	}
	windowSize := new(big.Int).Lsh(bigOne, w+1)
	halfSize := new(big.Int).Rsh(windowSize, 1)
	ee := new(big.Int).Abs(e)
	for ee.Cmp(bigZero) != 0 {
		if ee.Bit(0) == 1 {
			nafSign := new(big.Int)
			nafSign.Mod(ee, windowSize)
			if nafSign.Cmp(halfSize) >= 0 {
				nafSign.Sub(nafSign, windowSize)
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

func bigFromWNAF(naf nafNumber) *big.Int {
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
