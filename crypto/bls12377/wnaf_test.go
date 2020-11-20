package bls12377

import (
	"crypto/rand"
	"testing"
)

var maxWindowSize = 20

func TestWNAFConversion(t *testing.T) {
	for w := 2; w <= maxWindowSize; w++ {
		for i := 0; i < fuz; i++ {
			e0, err := rand.Int(rand.Reader, q)
			if err != nil {
				t.Fatal(err)
			}
			n := toWNAF(e0, w)
			e1 := fromWNAF(n, w)
			if e0.Cmp(e1) != 0 {
				t.Fatal("wnaf conversion failed")
			}
		}
	}
}
