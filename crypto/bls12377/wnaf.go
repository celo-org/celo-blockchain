package bls12377

import (
	"math/big"
)

var bigZero = big.NewInt(0)
var bigOne = big.NewInt(1)
var twoBitMask = big.NewInt(2)

func wnaf(e0 *big.Int, window uint) []int64 {
	e := new(big.Int).Set(e0)
	zero := big.NewInt(0)
	if e.Cmp(zero) == 0 {
		return []int64{}
	}
	max := int64(1 << window)
	midpoint := int64(1 << (window - 1))
	modulusMask := uint64(1<<window) - 1
	var out []int64
	for e.Cmp(bigZero) != 0 {
		var z int64
		if e.Bit(0)&1 == 1 {
			maskedBits := int64(e.Uint64() & modulusMask)
			if maskedBits > midpoint {
				z = maskedBits - max
				e.Add(e, new(big.Int).SetInt64(0-z))
			} else {
				z = maskedBits
				e.Sub(e, new(big.Int).SetInt64(z))
			}
		} else {
			z = 0
		}
		out = append(out, z)
		e.Rsh(e, 1)
	}
	return out
}

// type nafSign int
// type nafNumber []nafSign

// const (
// 	nafNEG  nafSign = 0
// 	nafZERO nafSign = 1
// 	nafPOS  nafSign = 2
// )

// func toNaf(e *big.Int) nafNumber {
// 	naf := []nafSign{}
// 	for e.Cmp(bigZero) != 0 {
// 		if e.Bit(0) == 1 {
// 			nafBit := new(big.Int).And(e, twoBitMask)
// 			naf = append(naf, nafSign(nafBit.Int64()))
// 			e.Add(e, nafBit)
// 		} else {
// 			naf = append(naf, nafZERO)
// 		}
// 		e.Rsh(e, 1)
// 	}
// 	return naf
// }

// func fromNaf(naf nafNumber) *big.Int {
// 	acc := new(big.Int)
// 	for i := len(naf) - 1; i >= 0; i-- {
// 		d := big.NewInt(1)
// 		d.Lsh(d, uint(i))
// 		if naf[len(naf)-i-1] == nafNEG {
// 			acc.Sub(acc, d)
// 		} else if naf[len(naf)-i-1] == nafPOS {
// 			acc.Add(acc, d)
// 		}
// 	}
// 	return acc
// }
