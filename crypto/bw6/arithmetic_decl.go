// +build amd64,!generic

package bw6

import "golang.org/x/sys/cpu"

func init() {
	if !cpu.X86.HasADX || !cpu.X86.HasBMI2 {
		mul = mulNoADX
	}
}

var mul func(c, a, b *fe) = mulADX

func neg(c, a *fe) {
	if a.isZero() {
		c.set(a)
	} else {
		_neg(c, a)
	}
}

func square(c, a *fe) {
	mul(c, a, a)
}

//go:noescape
func add(c, a, b *fe)

//go:noescape
func addAssign(a, b *fe)

//go:noescape
func ladd(c, a, b *fe)

//go:noescape
func laddAssign(a, b *fe)

//go:noescape
func double(c, a *fe)

//go:noescape
func doubleAssign(a *fe)

//go:noescape
func ldouble(c, a *fe)

//go:noescape
func ldoubleAssign(a *fe)

//go:noescape
func sub(c, a, b *fe)

//go:noescape
func subAssign(a, b *fe)

//go:noescape
func lsub(c, a, b *fe)

//go:noescape
func lsubAssign(a, b *fe)

//go:noescape
func _neg(c, a *fe)

//go:noescape
func mulNoADX(c, a, b *fe)

//go:noescape
func mulADX(c, a, b *fe)
