package bls12377

import (
	"crypto/rand"
	"math/big"
	"testing"
)

var maxWindowSize uint = 9

func TestWNAFConversion(t *testing.T) {
	var w uint
	for w = 1; w <= maxWindowSize; w++ {
		for i := 0; i < fuz; i++ {
			e0, err := rand.Int(rand.Reader, new(big.Int).SetUint64(100))
			if err != nil {
				t.Fatal(err)
			}
			n0 := toWNAF(e0, w)
			e1 := fromWNAF(n0)
			if e0.Cmp(e1) != 0 {
				t.Fatal("wnaf conversion failed")
			}
		}
	}
}
