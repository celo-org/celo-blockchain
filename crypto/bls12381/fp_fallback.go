//go:build !amd64 || generic
// +build !amd64 generic

// Copyright 2020 ConsenSys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by goff (v0.3.5) DO NOT EDIT

// /!\ WARNING /!\
// this code has not been audited and is provided as-is. In particular,
// there is no security guarantees such as constant time implementation
// or side-channel attack resistance
// /!\ WARNING /!\

package bls12381

import "math/bits"

func add(z, x, y *fe) {
	var carry uint64

	z[0], carry = bits.Add64(x[0], y[0], 0)
	z[1], carry = bits.Add64(x[1], y[1], carry)
	z[2], carry = bits.Add64(x[2], y[2], carry)
	z[3], carry = bits.Add64(x[3], y[3], carry)
	z[4], carry = bits.Add64(x[4], y[4], carry)
	z[5], _ = bits.Add64(x[5], y[5], carry)

	// if z > q --> z -= q
	// note: this is NOT constant time
	if !(z[5] < 1873798617647539866 || (z[5] == 1873798617647539866 && (z[4] < 5412103778470702295 || (z[4] == 5412103778470702295 && (z[3] < 7239337960414712511 || (z[3] == 7239337960414712511 && (z[2] < 7435674573564081700 || (z[2] == 7435674573564081700 && (z[1] < 2210141511517208575 || (z[1] == 2210141511517208575 && (z[0] < 13402431016077863595))))))))))) {
		var b uint64
		z[0], b = bits.Sub64(z[0], 13402431016077863595, 0)
		z[1], b = bits.Sub64(z[1], 2210141511517208575, b)
		z[2], b = bits.Sub64(z[2], 7435674573564081700, b)
		z[3], b = bits.Sub64(z[3], 7239337960414712511, b)
		z[4], b = bits.Sub64(z[4], 5412103778470702295, b)
		z[5], _ = bits.Sub64(z[5], 1873798617647539866, b)
	}
}

func addAssign(z, y *fe) {
	var carry uint64

	z[0], carry = bits.Add64(z[0], y[0], 0)
	z[1], carry = bits.Add64(z[1], y[1], carry)
	z[2], carry = bits.Add64(z[2], y[2], carry)
	z[3], carry = bits.Add64(z[3], y[3], carry)
	z[4], carry = bits.Add64(z[4], y[4], carry)
	z[5], _ = bits.Add64(z[5], y[5], carry)

	// if z > q --> z -= q
	// note: this is NOT constant time
	if !(z[5] < 1873798617647539866 || (z[5] == 1873798617647539866 && (z[4] < 5412103778470702295 || (z[4] == 5412103778470702295 && (z[3] < 7239337960414712511 || (z[3] == 7239337960414712511 && (z[2] < 7435674573564081700 || (z[2] == 7435674573564081700 && (z[1] < 2210141511517208575 || (z[1] == 2210141511517208575 && (z[0] < 13402431016077863595))))))))))) {
		var b uint64
		z[0], b = bits.Sub64(z[0], 13402431016077863595, 0)
		z[1], b = bits.Sub64(z[1], 2210141511517208575, b)
		z[2], b = bits.Sub64(z[2], 7435674573564081700, b)
		z[3], b = bits.Sub64(z[3], 7239337960414712511, b)
		z[4], b = bits.Sub64(z[4], 5412103778470702295, b)
		z[5], _ = bits.Sub64(z[5], 1873798617647539866, b)
	}
}

func laddAssign(z, y *fe) {
	var carry uint64

	z[0], carry = bits.Add64(z[0], y[0], 0)
	z[1], carry = bits.Add64(z[1], y[1], carry)
	z[2], carry = bits.Add64(z[2], y[2], carry)
	z[3], carry = bits.Add64(z[3], y[3], carry)
	z[4], carry = bits.Add64(z[4], y[4], carry)
	z[5], _ = bits.Add64(z[5], y[5], carry)
}

func double(z, x *fe) {
	var carry uint64

	z[0], carry = bits.Add64(x[0], x[0], 0)
	z[1], carry = bits.Add64(x[1], x[1], carry)
	z[2], carry = bits.Add64(x[2], x[2], carry)
	z[3], carry = bits.Add64(x[3], x[3], carry)
	z[4], carry = bits.Add64(x[4], x[4], carry)
	z[5], _ = bits.Add64(x[5], x[5], carry)

	// if z > q --> z -= q
	// note: this is NOT constant time
	if !(z[5] < 1873798617647539866 || (z[5] == 1873798617647539866 && (z[4] < 5412103778470702295 || (z[4] == 5412103778470702295 && (z[3] < 7239337960414712511 || (z[3] == 7239337960414712511 && (z[2] < 7435674573564081700 || (z[2] == 7435674573564081700 && (z[1] < 2210141511517208575 || (z[1] == 2210141511517208575 && (z[0] < 13402431016077863595))))))))))) {
		var b uint64
		z[0], b = bits.Sub64(z[0], 13402431016077863595, 0)
		z[1], b = bits.Sub64(z[1], 2210141511517208575, b)
		z[2], b = bits.Sub64(z[2], 7435674573564081700, b)
		z[3], b = bits.Sub64(z[3], 7239337960414712511, b)
		z[4], b = bits.Sub64(z[4], 5412103778470702295, b)
		z[5], _ = bits.Sub64(z[5], 1873798617647539866, b)
	}
}

func doubleAssign(z *fe) {
	var carry uint64

	z[0], carry = bits.Add64(z[0], z[0], 0)
	z[1], carry = bits.Add64(z[1], z[1], carry)
	z[2], carry = bits.Add64(z[2], z[2], carry)
	z[3], carry = bits.Add64(z[3], z[3], carry)
	z[4], carry = bits.Add64(z[4], z[4], carry)
	z[5], _ = bits.Add64(z[5], z[5], carry)

	// if z > q --> z -= q
	// note: this is NOT constant time
	if !(z[5] < 1873798617647539866 || (z[5] == 1873798617647539866 && (z[4] < 5412103778470702295 || (z[4] == 5412103778470702295 && (z[3] < 7239337960414712511 || (z[3] == 7239337960414712511 && (z[2] < 7435674573564081700 || (z[2] == 7435674573564081700 && (z[1] < 2210141511517208575 || (z[1] == 2210141511517208575 && (z[0] < 13402431016077863595))))))))))) {
		var b uint64
		z[0], b = bits.Sub64(z[0], 13402431016077863595, 0)
		z[1], b = bits.Sub64(z[1], 2210141511517208575, b)
		z[2], b = bits.Sub64(z[2], 7435674573564081700, b)
		z[3], b = bits.Sub64(z[3], 7239337960414712511, b)
		z[4], b = bits.Sub64(z[4], 5412103778470702295, b)
		z[5], _ = bits.Sub64(z[5], 1873798617647539866, b)
	}
}

func sub(z, x, y *fe) {
	var b uint64
	z[0], b = bits.Sub64(x[0], y[0], 0)
	z[1], b = bits.Sub64(x[1], y[1], b)
	z[2], b = bits.Sub64(x[2], y[2], b)
	z[3], b = bits.Sub64(x[3], y[3], b)
	z[4], b = bits.Sub64(x[4], y[4], b)
	z[5], b = bits.Sub64(x[5], y[5], b)
	if b != 0 {
		var c uint64
		z[0], c = bits.Add64(z[0], 13402431016077863595, 0)
		z[1], c = bits.Add64(z[1], 2210141511517208575, c)
		z[2], c = bits.Add64(z[2], 7435674573564081700, c)
		z[3], c = bits.Add64(z[3], 7239337960414712511, c)
		z[4], c = bits.Add64(z[4], 5412103778470702295, c)
		z[5], _ = bits.Add64(z[5], 1873798617647539866, c)
	}
}

func subAssign(z, y *fe) {
	var b uint64
	z[0], b = bits.Sub64(z[0], y[0], 0)
	z[1], b = bits.Sub64(z[1], y[1], b)
	z[2], b = bits.Sub64(z[2], y[2], b)
	z[3], b = bits.Sub64(z[3], y[3], b)
	z[4], b = bits.Sub64(z[4], y[4], b)
	z[5], b = bits.Sub64(z[5], y[5], b)
	if b != 0 {
		var c uint64
		z[0], c = bits.Add64(z[0], 13402431016077863595, 0)
		z[1], c = bits.Add64(z[1], 2210141511517208575, c)
		z[2], c = bits.Add64(z[2], 7435674573564081700, c)
		z[3], c = bits.Add64(z[3], 7239337960414712511, c)
		z[4], c = bits.Add64(z[4], 5412103778470702295, c)
		z[5], _ = bits.Add64(z[5], 1873798617647539866, c)
	}
}

func lsubAssign(z, y *fe) {
	var b uint64
	z[0], b = bits.Sub64(z[0], y[0], 0)
	z[1], b = bits.Sub64(z[1], y[1], b)
	z[2], b = bits.Sub64(z[2], y[2], b)
	z[3], b = bits.Sub64(z[3], y[3], b)
	z[4], b = bits.Sub64(z[4], y[4], b)
	z[5], b = bits.Sub64(z[5], y[5], b)
}

func neg(z, x *fe) {
	if x.isZero() {
		z.zero()
		return
	}
	var borrow uint64
	z[0], borrow = bits.Sub64(13402431016077863595, x[0], 0)
	z[1], borrow = bits.Sub64(2210141511517208575, x[1], borrow)
	z[2], borrow = bits.Sub64(7435674573564081700, x[2], borrow)
	z[3], borrow = bits.Sub64(7239337960414712511, x[3], borrow)
	z[4], borrow = bits.Sub64(5412103778470702295, x[4], borrow)
	z[5], _ = bits.Sub64(1873798617647539866, x[5], borrow)
}

func mul(z, x, y *fe) {

	var t [6]uint64
	var c [3]uint64
	{
		// round 0
		v := x[0]
		c[1], c[0] = bits.Mul64(v, y[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd1(v, y[1], c[1])
		c[2], t[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd1(v, y[2], c[1])
		c[2], t[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd1(v, y[3], c[1])
		c[2], t[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd1(v, y[4], c[1])
		c[2], t[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd1(v, y[5], c[1])
		t[5], t[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}
	{
		// round 1
		v := x[1]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], t[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], t[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		c[2], t[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd2(v, y[4], c[1], t[4])
		c[2], t[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd2(v, y[5], c[1], t[5])
		t[5], t[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}
	{
		// round 2
		v := x[2]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], t[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], t[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		c[2], t[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd2(v, y[4], c[1], t[4])
		c[2], t[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd2(v, y[5], c[1], t[5])
		t[5], t[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}
	{
		// round 3
		v := x[3]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], t[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], t[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		c[2], t[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd2(v, y[4], c[1], t[4])
		c[2], t[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd2(v, y[5], c[1], t[5])
		t[5], t[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}
	{
		// round 4
		v := x[4]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], t[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], t[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		c[2], t[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd2(v, y[4], c[1], t[4])
		c[2], t[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd2(v, y[5], c[1], t[5])
		t[5], t[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}
	{
		// round 5
		v := x[5]
		c[1], c[0] = madd1(v, y[0], t[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd2(v, y[1], c[1], t[1])
		c[2], z[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd2(v, y[2], c[1], t[2])
		c[2], z[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd2(v, y[3], c[1], t[3])
		c[2], z[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd2(v, y[4], c[1], t[4])
		c[2], z[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd2(v, y[5], c[1], t[5])
		z[5], z[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}

	// if z > q --> z -= q
	// note: this is NOT constant time
	if !(z[5] < 1873798617647539866 || (z[5] == 1873798617647539866 && (z[4] < 5412103778470702295 || (z[4] == 5412103778470702295 && (z[3] < 7239337960414712511 || (z[3] == 7239337960414712511 && (z[2] < 7435674573564081700 || (z[2] == 7435674573564081700 && (z[1] < 2210141511517208575 || (z[1] == 2210141511517208575 && (z[0] < 13402431016077863595))))))))))) {
		var b uint64
		z[0], b = bits.Sub64(z[0], 13402431016077863595, 0)
		z[1], b = bits.Sub64(z[1], 2210141511517208575, b)
		z[2], b = bits.Sub64(z[2], 7435674573564081700, b)
		z[3], b = bits.Sub64(z[3], 7239337960414712511, b)
		z[4], b = bits.Sub64(z[4], 5412103778470702295, b)
		z[5], _ = bits.Sub64(z[5], 1873798617647539866, b)
	}
}

func square(z, x *fe) {

	var t [6]uint64
	var c [3]uint64
	{
		// round 0
		v := x[0]
		c[1], c[0] = bits.Mul64(v, x[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd1(v, x[1], c[1])
		c[2], t[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd1(v, x[2], c[1])
		c[2], t[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd1(v, x[3], c[1])
		c[2], t[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd1(v, x[4], c[1])
		c[2], t[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd1(v, x[5], c[1])
		t[5], t[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}
	{
		// round 1
		v := x[1]
		c[1], c[0] = madd1(v, x[0], t[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd2(v, x[1], c[1], t[1])
		c[2], t[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd2(v, x[2], c[1], t[2])
		c[2], t[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd2(v, x[3], c[1], t[3])
		c[2], t[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd2(v, x[4], c[1], t[4])
		c[2], t[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd2(v, x[5], c[1], t[5])
		t[5], t[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}
	{
		// round 2
		v := x[2]
		c[1], c[0] = madd1(v, x[0], t[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd2(v, x[1], c[1], t[1])
		c[2], t[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd2(v, x[2], c[1], t[2])
		c[2], t[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd2(v, x[3], c[1], t[3])
		c[2], t[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd2(v, x[4], c[1], t[4])
		c[2], t[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd2(v, x[5], c[1], t[5])
		t[5], t[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}
	{
		// round 3
		v := x[3]
		c[1], c[0] = madd1(v, x[0], t[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd2(v, x[1], c[1], t[1])
		c[2], t[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd2(v, x[2], c[1], t[2])
		c[2], t[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd2(v, x[3], c[1], t[3])
		c[2], t[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd2(v, x[4], c[1], t[4])
		c[2], t[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd2(v, x[5], c[1], t[5])
		t[5], t[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}
	{
		// round 4
		v := x[4]
		c[1], c[0] = madd1(v, x[0], t[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd2(v, x[1], c[1], t[1])
		c[2], t[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd2(v, x[2], c[1], t[2])
		c[2], t[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd2(v, x[3], c[1], t[3])
		c[2], t[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd2(v, x[4], c[1], t[4])
		c[2], t[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd2(v, x[5], c[1], t[5])
		t[5], t[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}
	{
		// round 5
		v := x[5]
		c[1], c[0] = madd1(v, x[0], t[0])
		m := c[0] * 9940570264628428797
		c[2] = madd0(m, 13402431016077863595, c[0])
		c[1], c[0] = madd2(v, x[1], c[1], t[1])
		c[2], z[0] = madd2(m, 2210141511517208575, c[2], c[0])
		c[1], c[0] = madd2(v, x[2], c[1], t[2])
		c[2], z[1] = madd2(m, 7435674573564081700, c[2], c[0])
		c[1], c[0] = madd2(v, x[3], c[1], t[3])
		c[2], z[2] = madd2(m, 7239337960414712511, c[2], c[0])
		c[1], c[0] = madd2(v, x[4], c[1], t[4])
		c[2], z[3] = madd2(m, 5412103778470702295, c[2], c[0])
		c[1], c[0] = madd2(v, x[5], c[1], t[5])
		z[5], z[4] = madd3(m, 1873798617647539866, c[0], c[2], c[1])
	}

	// if z > q --> z -= q
	// note: this is NOT constant time
	if !(z[5] < 1873798617647539866 || (z[5] == 1873798617647539866 && (z[4] < 5412103778470702295 || (z[4] == 5412103778470702295 && (z[3] < 7239337960414712511 || (z[3] == 7239337960414712511 && (z[2] < 7435674573564081700 || (z[2] == 7435674573564081700 && (z[1] < 2210141511517208575 || (z[1] == 2210141511517208575 && (z[0] < 13402431016077863595))))))))))) {
		var b uint64
		z[0], b = bits.Sub64(z[0], 13402431016077863595, 0)
		z[1], b = bits.Sub64(z[1], 2210141511517208575, b)
		z[2], b = bits.Sub64(z[2], 7435674573564081700, b)
		z[3], b = bits.Sub64(z[3], 7239337960414712511, b)
		z[4], b = bits.Sub64(z[4], 5412103778470702295, b)
		z[5], _ = bits.Sub64(z[5], 1873798617647539866, b)
	}
}