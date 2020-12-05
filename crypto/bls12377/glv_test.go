package bls12377

import (
	"math/big"
	"testing"
)

func TestGLVConstruction(t *testing.T) {
	t.Run("Parameters", func(t *testing.T) {
		t0, t1 := new(big.Int), new(big.Int)
		one := new(big.Int).SetUint64(1)
		t0.Mul(glvLambda, glvLambda)
		t0.Add(t0, glvLambda)
		t1.Sub(q, one)
		if t0.Cmp(t1) != 0 {
			t.Fatal("lambda1^2 + lambda1 + 1 = 0")
		}
		c0 := new(fe)
		square(c0, glvPhi1)
		mul(c0, c0, glvPhi1)
		if !c0.isOne() {
			t.Fatal("phi1^3 = 1")
		}
		square(c0, glvPhi2)
		mul(c0, c0, glvPhi2)
		if !c0.isOne() {
			t.Fatal("phi2^3 = 1")
		}
	})
	t.Run("Endomorphism G1", func(t *testing.T) {
		g := NewG1()
		{
			p0, p1 := g.randAffine(), g.New()
			g.MulScalar(p1, p0, glvLambda)
			g.Affine(p1)
			r := g.New()
			g.glvEndomorphism(r, p0)
			if !g.Equal(r, p1) {
				t.Fatal("f(x, y) = (phi * x, y)")
			}
		}
	})
	t.Run("Endomorphism G2", func(t *testing.T) {
		g := NewG2()
		{
			p0, p1 := g.randAffine(), g.New()
			g.MulScalar(p1, p0, glvLambda)
			g.Affine(p1)
			r := g.New()
			g.glvEndomorphism(r, p0)
			if !g.Equal(r, p1) {
				t.Fatal("f(x, y) = (phi * x, y)")
			}
		}
	})
	t.Run("Scalar Decomposition", func(t *testing.T) {
		for i := 0; i < fuz; i++ {
			m := randScalar(q)
			var v *glvVector
			r128 := bigFromHex("0xffffffffffffffffffffffffffffffff")
			v = new(glvVector).new(m)
			if new(big.Int).Abs(v.k1).Cmp(r128) >= 0 {
				t.Fatal("bad scalar component, big k1")
			}
			if new(big.Int).Abs(v.k2).Cmp(r128) >= 0 {
				t.Fatal("bad scalar component, big k2")
			}
			k := new(big.Int)
			k.Mul(glvLambda, v.k2)
			k.Sub(v.k1, k).Mod(k, q)
			if k.Cmp(m) != 0 {
				t.Fatal("scalar decomposing with big.Int failed", i)
			}
		}
	})
}
