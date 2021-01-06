package bw6

import (
	"math/big"
	"testing"
)

func TestGLVConstruction(t *testing.T) {
	zero := new(big.Int)
	one := new(big.Int).SetUint64(1)
	t.Run("Parameters", func(t *testing.T) {
		t0 := new(big.Int).Mul(glvLambda, glvLambda)
		t0.Add(t0, glvLambda)
		t0.Add(t0, one)
		t0.Mod(t0, q)
		if t0.Cmp(zero) != 0 {
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
		g := NewG()
		{
			p0, p1 := g.randG1Affine(), g.New()
			g.mulScalar(p1, p0, glvLambda)
			g.Affine(p1)
			r := g.New()
			g.glvEndomorphismG1(r, p0)
			if !g.Equal(r, p1) {
				t.Fatal("f(x, y) = (phi * x, y)")
			}
		}
	})
	t.Run("Endomorphism G2", func(t *testing.T) {
		g := NewG()
		{
			p0, p1 := g.randG2Affine(), g.New()
			g.mulScalar(p1, p0, glvLambda)
			g.Affine(p1)
			r := g.New()
			g.glvEndomorphismG2(r, p0)
			if !g.Equal(r, p1) {
				t.Fatal("f(x, y) = (phi * x, y)")
			}
		}
	})
	t.Run("Scalar Decomposition", func(t *testing.T) {
		for i := 0; i < fuz; i++ {
			k := randScalar(q)
			var v *glvVector

			r192 := new(big.Int).SetUint64(1)
			r192.Lsh(r192, 192)
			{
				v = new(glvVector).new(k)

				if new(big.Int).Abs(v.k1).Cmp(r192) >= 0 {
					t.Fatal("bad scalar component, k1")
				}
				if new(big.Int).Abs(v.k2).Cmp(r192) >= 0 {
					t.Fatal("bad scalar component, k2")
				}

				_k := new(big.Int)
				_k.Mul(glvLambda, v.k2)
				_k.Add(v.k1, _k).Mod(_k, q)
				if k.Cmp(_k) != 0 {
					t.Fatal("scalar decomposing failed", i)
				}
			}
		}
	})
}
