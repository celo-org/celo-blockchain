// +build amd64,!generic

#include "textflag.h"


// c =  (a + b) % q
// func add(c *[12]uint64, a *[12]uint64, b *[12]uint64)
TEXT ·add(SB), NOSPLIT, $64-24
	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI
	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	ADDQ (SI), CX
	ADCQ 8(SI), DX
	ADCQ 16(SI), R8
	ADCQ 24(SI), R9
	ADCQ 32(SI), R10
	ADCQ 40(SI), R11
	ADCQ 48(SI), R12
	ADCQ 56(SI), R13
	ADCQ 64(SI), R14
	ADCQ 72(SI), R15
	ADCQ 80(SI), AX
	ADCQ 88(SI), BX

	MOVQ CX, (SP)
	MOVQ DX, 8(SP)
	MOVQ R8, 16(SP)
	MOVQ R9, 24(SP)
	MOVQ R10, 32(SP)
	MOVQ R11, 40(SP)
	MOVQ R12, 48(SP)
	MOVQ R13, 56(SP)
	MOVQ R14, 64(SP)
	MOVQ R15, DI
	MOVQ AX, BP
	MOVQ BX, SI

	SUBQ ·modulus+0(SB), CX
	SBBQ ·modulus+8(SB), DX
	SBBQ ·modulus+16(SB), R8
	SBBQ ·modulus+24(SB), R9
	SBBQ ·modulus+32(SB), R10
	SBBQ ·modulus+40(SB), R11
	SBBQ ·modulus+48(SB), R12
	SBBQ ·modulus+56(SB), R13
	SBBQ ·modulus+64(SB), R14
	SBBQ ·modulus+72(SB), R15
	SBBQ ·modulus+80(SB), AX
	SBBQ ·modulus+88(SB), BX

	// |
	CMOVQCS 0(SP), CX
	CMOVQCS 8(SP), DX
	CMOVQCS 16(SP), R8
	CMOVQCS 24(SP), R9
	CMOVQCS 32(SP), R10
	CMOVQCS 40(SP), R11
	CMOVQCS 48(SP), R12
	CMOVQCS 56(SP), R13
	CMOVQCS 64(SP), R14
	CMOVQCS DI, R15
	CMOVQCS BP, AX
	CMOVQCS SI, BX

	MOVQ c+0(FP), DI
	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// a =  (a + b) % q
// func addAssign(a *[12]uint64, b *[12]uint64)
TEXT ·addAssign(SB), NOSPLIT, $64-16
	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	ADDQ (SI), CX
	ADCQ 8(SI), DX
	ADCQ 16(SI), R8
	ADCQ 24(SI), R9
	ADCQ 32(SI), R10
	ADCQ 40(SI), R11
	ADCQ 48(SI), R12
	ADCQ 56(SI), R13
	ADCQ 64(SI), R14
	ADCQ 72(SI), R15
	ADCQ 80(SI), AX
	ADCQ 88(SI), BX

	MOVQ CX, (SP)
	MOVQ DX, 8(SP)
	MOVQ R8, 16(SP)
	MOVQ R9, 24(SP)
	MOVQ R10, 32(SP)
	MOVQ R11, 40(SP)
	MOVQ R12, 48(SP)
	MOVQ R13, 56(SP)
	MOVQ R14, 64(SP)
	MOVQ R15, DI
	MOVQ AX, BP
	MOVQ BX, SI

	SUBQ ·modulus+0(SB), CX
	SBBQ ·modulus+8(SB), DX
	SBBQ ·modulus+16(SB), R8
	SBBQ ·modulus+24(SB), R9
	SBBQ ·modulus+32(SB), R10
	SBBQ ·modulus+40(SB), R11
	SBBQ ·modulus+48(SB), R12
	SBBQ ·modulus+56(SB), R13
	SBBQ ·modulus+64(SB), R14
	SBBQ ·modulus+72(SB), R15
	SBBQ ·modulus+80(SB), AX
	SBBQ ·modulus+88(SB), BX

	// |
	CMOVQCS 0(SP), CX
	CMOVQCS 8(SP), DX
	CMOVQCS 16(SP), R8
	CMOVQCS 24(SP), R9
	CMOVQCS 32(SP), R10
	CMOVQCS 40(SP), R11
	CMOVQCS 48(SP), R12
	CMOVQCS 56(SP), R13
	CMOVQCS 64(SP), R14
	CMOVQCS DI, R15
	CMOVQCS BP, AX
	CMOVQCS SI, BX

	MOVQ a+0(FP), DI
	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// lazy addition
// c = (a + b)
// func ladd(c *[12]uint64, a *[12]uint64, b *[12]uint64)
TEXT ·ladd(SB), NOSPLIT, $0-24
	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	ADDQ (SI), CX
	ADCQ 8(SI), DX
	ADCQ 16(SI), R8
	ADCQ 24(SI), R9
	ADCQ 32(SI), R10
	ADCQ 40(SI), R11
	ADCQ 48(SI), R12
	ADCQ 56(SI), R13
	ADCQ 64(SI), R14
	ADCQ 72(SI), R15
	ADCQ 80(SI), AX
	ADCQ 88(SI), BX

	MOVQ c+0(FP), DI
	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// lazy addition
// a = (a + b)
// func laddAssign(a *[12]uint64, b *[12]uint64)
TEXT ·laddAssign(SB), NOSPLIT, $0-16
	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	ADDQ (SI), CX
	ADCQ 8(SI), DX
	ADCQ 16(SI), R8
	ADCQ 24(SI), R9
	ADCQ 32(SI), R10
	ADCQ 40(SI), R11
	ADCQ 48(SI), R12
	ADCQ 56(SI), R13
	ADCQ 64(SI), R14
	ADCQ 72(SI), R15
	ADCQ 80(SI), AX
	ADCQ 88(SI), BX

	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// c =  (2 * a) % q
// func double(c *[12]uint64, a *[12]uint64)
TEXT ·double(SB), NOSPLIT, $64-16
	// |
	MOVQ a+8(FP), DI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	ADDQ CX, CX
	ADCQ DX, DX
	ADCQ R8, R8
	ADCQ R9, R9
	ADCQ R10, R10
	ADCQ R11, R11
	ADCQ R12, R12
	ADCQ R13, R13
	ADCQ R14, R14
	ADCQ R15, R15
	ADCQ AX, AX
	ADCQ BX, BX

	MOVQ CX, (SP)
	MOVQ DX, 8(SP)
	MOVQ R8, 16(SP)
	MOVQ R9, 24(SP)
	MOVQ R10, 32(SP)
	MOVQ R11, 40(SP)
	MOVQ R12, 48(SP)
	MOVQ R13, 56(SP)
	MOVQ R14, 64(SP)
	MOVQ R15, DI
	MOVQ AX, BP
	MOVQ BX, SI

	SUBQ ·modulus+0(SB), CX
	SBBQ ·modulus+8(SB), DX
	SBBQ ·modulus+16(SB), R8
	SBBQ ·modulus+24(SB), R9
	SBBQ ·modulus+32(SB), R10
	SBBQ ·modulus+40(SB), R11
	SBBQ ·modulus+48(SB), R12
	SBBQ ·modulus+56(SB), R13
	SBBQ ·modulus+64(SB), R14
	SBBQ ·modulus+72(SB), R15
	SBBQ ·modulus+80(SB), AX
	SBBQ ·modulus+88(SB), BX

	// |
	CMOVQCS 0(SP), CX
	CMOVQCS 8(SP), DX
	CMOVQCS 16(SP), R8
	CMOVQCS 24(SP), R9
	CMOVQCS 32(SP), R10
	CMOVQCS 40(SP), R11
	CMOVQCS 48(SP), R12
	CMOVQCS 56(SP), R13
	CMOVQCS 64(SP), R14
	CMOVQCS DI, R15
	CMOVQCS BP, AX
	CMOVQCS SI, BX

	MOVQ c+0(FP), DI
	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// c =  (2 * a) % q
// func doubleAssign(a *[12]uint64)
TEXT ·doubleAssign(SB), NOSPLIT, $64-8
	// |
	MOVQ a+0(FP), DI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	ADDQ CX, CX
	ADCQ DX, DX
	ADCQ R8, R8
	ADCQ R9, R9
	ADCQ R10, R10
	ADCQ R11, R11
	ADCQ R12, R12
	ADCQ R13, R13
	ADCQ R14, R14
	ADCQ R15, R15
	ADCQ AX, AX
	ADCQ BX, BX

	MOVQ CX, (SP)
	MOVQ DX, 8(SP)
	MOVQ R8, 16(SP)
	MOVQ R9, 24(SP)
	MOVQ R10, 32(SP)
	MOVQ R11, 40(SP)
	MOVQ R12, 48(SP)
	MOVQ R13, 56(SP)
	MOVQ R14, 64(SP)
	MOVQ R15, DI
	MOVQ AX, BP
	MOVQ BX, SI

	SUBQ ·modulus+0(SB), CX
	SBBQ ·modulus+8(SB), DX
	SBBQ ·modulus+16(SB), R8
	SBBQ ·modulus+24(SB), R9
	SBBQ ·modulus+32(SB), R10
	SBBQ ·modulus+40(SB), R11
	SBBQ ·modulus+48(SB), R12
	SBBQ ·modulus+56(SB), R13
	SBBQ ·modulus+64(SB), R14
	SBBQ ·modulus+72(SB), R15
	SBBQ ·modulus+80(SB), AX
	SBBQ ·modulus+88(SB), BX

	// |
	CMOVQCS 0(SP), CX
	CMOVQCS 8(SP), DX
	CMOVQCS 16(SP), R8
	CMOVQCS 24(SP), R9
	CMOVQCS 32(SP), R10
	CMOVQCS 40(SP), R11
	CMOVQCS 48(SP), R12
	CMOVQCS 56(SP), R13
	CMOVQCS 64(SP), R14
	CMOVQCS DI, R15
	CMOVQCS BP, AX
	CMOVQCS SI, BX

	MOVQ a+0(FP), DI
	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// lazy addition
// c = (2 * a)
// func ldouble(c *[12]uint64, a *[12]uint64)
TEXT ·ldouble(SB), NOSPLIT, $0-16
	// |
	MOVQ a+8(FP), DI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	ADDQ CX, CX
	ADCQ DX, DX
	ADCQ R8, R8
	ADCQ R9, R9
	ADCQ R10, R10
	ADCQ R11, R11
	ADCQ R12, R12
	ADCQ R13, R13
	ADCQ R14, R14
	ADCQ R15, R15
	ADCQ AX, AX
	ADCQ BX, BX

	MOVQ c+0(FP), DI
	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// lazy addition
// a = (2 * a)
// func ldoubleAssign(a *[12]uint64)
TEXT ·ldoubleAssign(SB), NOSPLIT, $0-8
	// |
	MOVQ a+0(FP), DI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	ADDQ CX, CX
	ADCQ DX, DX
	ADCQ R8, R8
	ADCQ R9, R9
	ADCQ R10, R10
	ADCQ R11, R11
	ADCQ R12, R12
	ADCQ R13, R13
	ADCQ R14, R14
	ADCQ R15, R15
	ADCQ AX, AX
	ADCQ BX, BX

	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// c =  (a - b) % q
// func sub(c *[12]uint64, a *[12]uint64, b *[12]uint64)
TEXT ·sub(SB), NOSPLIT, $64-24
	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	SUBQ (SI), CX
	SBBQ 8(SI), DX
	SBBQ 16(SI), R8
	SBBQ 24(SI), R9
	SBBQ 32(SI), R10
	SBBQ 40(SI), R11
	SBBQ 48(SI), R12
	SBBQ 56(SI), R13
	SBBQ 64(SI), R14
	SBBQ 72(SI), R15
	SBBQ 80(SI), AX
	SBBQ 88(SI), BX

	// | decide
	JCC ret

reduce:

	ADDQ ·modulus+0(SB), CX
	ADCQ ·modulus+8(SB), DX
	ADCQ ·modulus+16(SB), R8
	ADCQ ·modulus+24(SB), R9
	ADCQ ·modulus+32(SB), R10
	ADCQ ·modulus+40(SB), R11
	ADCQ ·modulus+48(SB), R12
	ADCQ ·modulus+56(SB), R13
	ADCQ ·modulus+64(SB), R14
	ADCQ ·modulus+72(SB), R15
	ADCQ ·modulus+80(SB), AX
	ADCQ ·modulus+88(SB), BX

ret:

	MOVQ c+0(FP), DI
	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/

// a =  (a - b) % q
// func subAssign(a *[12]uint64, b *[12]uint64)
TEXT ·subAssign(SB), NOSPLIT, $0-16
	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	SUBQ (SI), CX
	SBBQ 8(SI), DX
	SBBQ 16(SI), R8
	SBBQ 24(SI), R9
	SBBQ 32(SI), R10
	SBBQ 40(SI), R11
	SBBQ 48(SI), R12
	SBBQ 56(SI), R13
	SBBQ 64(SI), R14
	SBBQ 72(SI), R15
	SBBQ 80(SI), AX
	SBBQ 88(SI), BX

	// | decide
	JCC ret

reduce:

	ADDQ ·modulus+0(SB), CX
	ADCQ ·modulus+8(SB), DX
	ADCQ ·modulus+16(SB), R8
	ADCQ ·modulus+24(SB), R9
	ADCQ ·modulus+32(SB), R10
	ADCQ ·modulus+40(SB), R11
	ADCQ ·modulus+48(SB), R12
	ADCQ ·modulus+56(SB), R13
	ADCQ ·modulus+64(SB), R14
	ADCQ ·modulus+72(SB), R15
	ADCQ ·modulus+80(SB), AX
	ADCQ ·modulus+88(SB), BX

ret:

	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// c =  (a - b)
// func lsub(c *[12]uint64, a *[12]uint64, b *[12]uint64)
TEXT ·lsub(SB), NOSPLIT, $0-24
	// |
	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	SUBQ (SI), CX
	SBBQ 8(SI), DX
	SBBQ 16(SI), R8
	SBBQ 24(SI), R9
	SBBQ 32(SI), R10
	SBBQ 40(SI), R11
	SBBQ 48(SI), R12
	SBBQ 56(SI), R13
	SBBQ 64(SI), R14
	SBBQ 72(SI), R15
	SBBQ 80(SI), AX
	SBBQ 88(SI), BX

	MOVQ c+0(FP), DI
	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// a =  (a - b)
// func lsubAssign(c *[12]uint64, a *[12]uint64, b *[12]uint64)
TEXT ·lsubAssign(SB), NOSPLIT, $0-16
	// |
	MOVQ a+0(FP), DI
	MOVQ b+8(FP), SI

	// |
	MOVQ (DI), CX
	MOVQ 8(DI), DX
	MOVQ 16(DI), R8
	MOVQ 24(DI), R9
	MOVQ 32(DI), R10
	MOVQ 40(DI), R11
	MOVQ 48(DI), R12
	MOVQ 56(DI), R13
	MOVQ 64(DI), R14
	MOVQ 72(DI), R15
	MOVQ 80(DI), AX
	MOVQ 88(DI), BX

	SUBQ (SI), CX
	SBBQ 8(SI), DX
	SBBQ 16(SI), R8
	SBBQ 24(SI), R9
	SBBQ 32(SI), R10
	SBBQ 40(SI), R11
	SBBQ 48(SI), R12
	SBBQ 56(SI), R13
	SBBQ 64(SI), R14
	SBBQ 72(SI), R15
	SBBQ 80(SI), AX
	SBBQ 88(SI), BX

	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// c = -a
// func _neg(c *[12]uint64, a *[12]uint64)
TEXT ·_neg(SB), NOSPLIT, $0-16
	// |
	MOVQ a+8(FP), DI

	// |
	MOVQ ·modulus+0(SB), CX
	MOVQ ·modulus+8(SB), DX
	MOVQ ·modulus+16(SB), R8
	MOVQ ·modulus+24(SB), R9
	MOVQ ·modulus+32(SB), R10
	MOVQ ·modulus+40(SB), R11
	MOVQ ·modulus+48(SB), R12
	MOVQ ·modulus+56(SB), R13
	MOVQ ·modulus+64(SB), R14
	MOVQ ·modulus+72(SB), R15
	MOVQ ·modulus+80(SB), AX
	MOVQ ·modulus+88(SB), BX

	SUBQ (DI), CX
	SBBQ 8(DI), DX
	SBBQ 16(DI), R8
	SBBQ 24(DI), R9
	SBBQ 32(DI), R10
	SBBQ 40(DI), R11
	SBBQ 48(DI), R12
	SBBQ 56(DI), R13
	SBBQ 64(DI), R14
	SBBQ 72(DI), R15
	SBBQ 80(DI), AX
	SBBQ 88(DI), BX

	// |
	MOVQ c+0(FP), DI
	MOVQ CX, (DI)
	MOVQ DX, 8(DI)
	MOVQ R8, 16(DI)
	MOVQ R9, 24(DI)
	MOVQ R10, 32(DI)
	MOVQ R11, 40(DI)
	MOVQ R12, 48(DI)
	MOVQ R13, 56(DI)
	MOVQ R14, 64(DI)
	MOVQ R15, 72(DI)
	MOVQ AX, 80(DI)
	MOVQ BX, 88(DI)
	RET
/*	 | end													*/


// func mulNoADX(c *[12]uint64, a *[12]uint64, b *[12]uint64)
TEXT ·mulNoADX(SB), NOSPLIT, $216-24
	// | 

/* inputs                                  */

	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI
	MOVQ $0x00, R9
	MOVQ $0x00, R10
	MOVQ $0x00, R11
	MOVQ $0x00, R12
	MOVQ $0x00, R13
	MOVQ $0x00, R14
	MOVQ $0x00, R15

	// | 

/* i = 0                                   */

	// | a0 @ CX
	MOVQ (DI), CX

	// | a0 * b0 
	MOVQ (SI), AX
	MULQ CX
	MOVQ AX, (SP)
	MOVQ DX, R8

	// | a0 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9

	// | a0 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10

	// | a0 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11

	// | a0 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12

	// | a0 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13

	// | a0 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14

	// | a0 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15

	// | 

/* i = 1                                   */

	// | a1 @ CX
	MOVQ 8(DI), CX
	MOVQ $0x00, BX

	// | a1 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0x00, R10
	ADCQ $0x00, BX
	MOVQ R8, 8(SP)
	MOVQ $0x00, R8

	// | a1 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a1 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a1 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a1 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a1 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a1 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8

	// | a1 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8

	// | 

/* i = 2                                   */

	// | a2 @ CX
	MOVQ 16(DI), CX
	MOVQ $0x00, BX

	// | a2 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ $0x00, R11
	ADCQ $0x00, BX
	MOVQ R9, 16(SP)
	MOVQ $0x00, R9

	// | a2 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a2 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a2 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a2 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a2 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a2 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ BX, R9

	// | a2 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9

	// | 

/* i = 3                                   */

	// | a3 @ CX
	MOVQ 24(DI), CX
	MOVQ $0x00, BX

	// | a3 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ $0x00, R12
	ADCQ $0x00, BX
	MOVQ R10, 24(SP)
	MOVQ $0x00, R10

	// | a3 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a3 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a3 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a3 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a3 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ BX, R9
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a3 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ BX, R10

	// | a3 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10

	// | 

/* i = 4                                   */

	// | a4 @ CX
	MOVQ 32(DI), CX
	MOVQ $0x00, BX

	// | a4 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ $0x00, R13
	ADCQ $0x00, BX
	MOVQ R11, 32(SP)
	MOVQ $0x00, R11

	// | a4 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a4 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a4 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a4 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ BX, R9
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a4 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ BX, R10
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a4 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11

	// | a4 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11

	// | 

/* i = 5                                   */

	// | a5 @ CX
	MOVQ 40(DI), CX
	MOVQ $0x00, BX

	// | a5 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0x00, R14
	ADCQ $0x00, BX
	MOVQ R12, 40(SP)
	MOVQ $0x00, R12

	// | a5 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a5 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a5 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ BX, R9
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a5 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ BX, R10
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a5 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a5 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12

	// | a5 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12

	// | 

/* i = 6                                   */

	// | a6 @ CX
	MOVQ 48(DI), CX
	MOVQ $0x00, BX

	// | a6 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ $0x00, R15
	ADCQ $0x00, BX
	MOVQ R13, 48(SP)
	MOVQ $0x00, R13

	// | a6 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a6 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ BX, R9
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a6 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ BX, R10
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a6 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a6 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a6 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13

	// | a6 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13

	// | 

/* i = 7                                   */

	// | a7 @ CX
	MOVQ 56(DI), CX
	MOVQ $0x00, BX

	// | a7 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ $0x00, R8
	ADCQ $0x00, BX
	MOVQ R14, 56(SP)
	MOVQ $0x00, R14

	// | a7 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ BX, R9
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a7 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ BX, R10
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a7 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a7 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a7 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a7 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14

	// | a7 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14

	// | 

/* i = 8                                   */

	// | a8 @ CX
	MOVQ 64(DI), CX
	MOVQ $0x00, BX

	// | a8 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ $0x00, R9
	ADCQ $0x00, BX
	MOVQ R15, 64(SP)
	MOVQ $0x00, R15

	// | a8 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ BX, R10
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a8 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a8 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a8 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a8 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a8 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15

	// | a8 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15

	// | 

/* i = 9                                   */

	// | a9 @ CX
	MOVQ 72(DI), CX
	MOVQ $0x00, BX

	// | a9 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0x00, R10
	ADCQ $0x00, BX
	MOVQ R8, 72(SP)
	MOVQ $0x00, R8

	// | a9 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a9 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a9 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a9 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a9 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a9 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8

	// | a9 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8

	// | 

/* i = 10                                  */

	// | a10 @ CX
	MOVQ 80(DI), CX
	MOVQ $0x00, BX

	// | a10 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ $0x00, R11
	ADCQ $0x00, BX
	MOVQ R9, 80(SP)
	MOVQ $0x00, R9

	// | a10 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a10 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a10 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a10 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a10 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a10 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ BX, R9

	// | a10 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9

	// | 

/* i = 11                                  */

	// | a11 @ CX
	MOVQ 88(DI), CX
	MOVQ $0x00, BX

	// | a11 * b0 
	MOVQ (SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ $0x00, R12
	ADCQ $0x00, BX

	// | a11 * b1 
	MOVQ 8(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a11 * b2 
	MOVQ 16(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a11 * b3 
	MOVQ 24(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a11 * b4 
	MOVQ 32(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a11 * b5 
	MOVQ 40(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ BX, R9
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a11 * b6 
	MOVQ 48(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0x00, BX

	// | a11 * b7 
	MOVQ 56(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, BX

	// | 

/* 			                                   */

	// | 
	// | W part 1 multiplication
	// | 0   (SP)      | 1   8(SP)     | 2   16(SP)    | 3   24(SP)    | 4   32(SP)    | 5   40(SP)    | 6   48(SP)    | 7   56(SP)    | 8   64(SP)    | 9   72(SP)    | 10  80(SP)    | 11  R10       
	// | 12  R11       | 13  R12       | 14  R13       | 15  R14       | 16  R15       | 17  R8        | 18  R9        | 19  BX        | 20  -         | 21  -         | 22  -         | 23  -         


	MOVQ R10, 88(SP)
	MOVQ R11, 96(SP)
	MOVQ R12, 104(SP)
	MOVQ R13, 112(SP)
	MOVQ R14, 120(SP)
	MOVQ R15, 128(SP)
	MOVQ R8, 136(SP)
	MOVQ R9, 144(SP)
	MOVQ BX, 152(SP)

	// | 
	// | W part 1 moved to stack
	// | 0   (SP)      | 1   8(SP)     | 2   16(SP)    | 3   24(SP)    | 4   32(SP)    | 5   40(SP)    | 6   48(SP)    | 7   56(SP)    | 8   64(SP)    | 9   72(SP)    | 10  80(SP)    | 11  88(SP)    
	// | 12  96(SP)    | 13  104(SP)   | 14  112(SP)   | 15  120(SP)   | 16  128(SP)   | 17  136(SP)   | 18  144(SP)   | 19  152(SP)   | 20  -         | 21  -         | 22  -         | 23  -         


	MOVQ $0x00, R9
	MOVQ $0x00, R10
	MOVQ $0x00, R11
	MOVQ $0x00, R12
	MOVQ $0x00, R13
	MOVQ $0x00, R14
	MOVQ $0x00, R15

	// | 

/* i = 0                                   */

	// | a0 @ CX
	MOVQ (DI), CX

	// | a0 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	MOVQ AX, 160(SP)
	MOVQ DX, R8

	// | a0 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9

	// | a0 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10

	// | a0 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11

	// | 

/* i = 1                                   */

	// | a1 @ CX
	MOVQ 8(DI), CX
	MOVQ $0x00, BX

	// | a1 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0x00, R10
	ADCQ $0x00, BX
	MOVQ R8, 168(SP)
	MOVQ $0x00, R8

	// | a1 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a1 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12

	// | a1 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12

	// | 

/* i = 2                                   */

	// | a2 @ CX
	MOVQ 16(DI), CX
	MOVQ $0x00, BX

	// | a2 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ $0x00, R11
	ADCQ $0x00, BX
	MOVQ R9, 176(SP)
	MOVQ $0x00, R9

	// | a2 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a2 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13

	// | a2 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13

	// | 

/* i = 3                                   */

	// | a3 @ CX
	MOVQ 24(DI), CX
	MOVQ $0x00, BX

	// | a3 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ $0x00, R12
	ADCQ $0x00, BX
	MOVQ R10, 184(SP)
	MOVQ $0x00, R10

	// | a3 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a3 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14

	// | a3 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14

	// | 

/* i = 4                                   */

	// | a4 @ CX
	MOVQ 32(DI), CX
	MOVQ $0x00, BX

	// | a4 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ $0x00, R13
	ADCQ $0x00, BX
	MOVQ R11, 192(SP)
	MOVQ $0x00, R11

	// | a4 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ BX, R14
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a4 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15

	// | a4 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15

	// | 

/* i = 5                                   */

	// | a5 @ CX
	MOVQ 40(DI), CX
	MOVQ $0x00, BX

	// | a5 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0x00, R14
	ADCQ $0x00, BX
	MOVQ R12, 200(SP)
	MOVQ $0x00, R12

	// | a5 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ BX, R15
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a5 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8

	// | a5 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8

	// | 

/* i = 6                                   */

	// | a6 @ CX
	MOVQ 48(DI), CX
	MOVQ $0x00, BX

	// | a6 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, R14
	ADCQ $0x00, R15
	ADCQ $0x00, BX
	MOVQ R13, 208(SP)
	MOVQ $0x00, R13

	// | a6 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ BX, R8
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a6 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ BX, R9

	// | a6 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9

	// | 

/* i = 7                                   */

	// | a7 @ CX
	MOVQ 56(DI), CX
	MOVQ $0x00, BX

	// | a7 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R14
	ADCQ DX, R15
	ADCQ $0x00, R8
	ADCQ $0x00, BX

	// | a7 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ BX, R9
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a7 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ BX, R10

	// | a7 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10

	// | 

/* i = 8                                   */

	// | a8 @ CX
	MOVQ 64(DI), CX
	MOVQ $0x00, BX

	// | a8 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R15
	ADCQ DX, R8
	ADCQ $0x00, R9
	ADCQ $0x00, BX

	// | a8 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ BX, R10
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a8 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11

	// | a8 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11

	// | 

/* i = 9                                   */

	// | a9 @ CX
	MOVQ 72(DI), CX
	MOVQ $0x00, BX

	// | a9 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R8
	ADCQ DX, R9
	ADCQ $0x00, R10
	ADCQ $0x00, BX

	// | a9 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ BX, R11
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a9 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12

	// | a9 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12

	// | 

/* i = 10                                  */

	// | a10 @ CX
	MOVQ 80(DI), CX
	MOVQ $0x00, BX

	// | a10 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R9
	ADCQ DX, R10
	ADCQ $0x00, R11
	ADCQ $0x00, BX

	// | a10 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ BX, R12
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a10 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13

	// | a10 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13

	// | 

/* i = 11                                  */

	// | a11 @ CX
	MOVQ 88(DI), CX
	MOVQ $0x00, BX

	// | a11 * b8 
	MOVQ 64(SI), AX
	MULQ CX
	ADDQ AX, R10
	ADCQ DX, R11
	ADCQ $0x00, R12
	ADCQ $0x00, BX

	// | a11 * b9 
	MOVQ 72(SI), AX
	MULQ CX
	ADDQ AX, R11
	ADCQ DX, R12
	ADCQ BX, R13
	MOVQ $0x00, BX
	ADCQ $0x00, BX

	// | a11 * b10 
	MOVQ 80(SI), AX
	MULQ CX
	ADDQ AX, R12
	ADCQ DX, R13
	ADCQ $0x00, BX

	// | a11 * b11 
	MOVQ 88(SI), AX
	MULQ CX
	ADDQ AX, R13
	ADCQ DX, BX

	// | 

/* 			                                   */

	// | 
	// | W part 2 multiplication
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   160(SP)   | 9   168(SP)   | 10  176(SP)   | 11  184(SP)   
	// | 12  192(SP)   | 13  200(SP)   | 14  208(SP)   | 15  R14       | 16  R15       | 17  R8        | 18  R9        | 19  R10       | 20  R11       | 21  R12       | 22  R13       | 23  BX        


	// | 
	// | W part 1
	// | 0   (SP)      | 1   8(SP)     | 2   16(SP)    | 3   24(SP)    | 4   32(SP)    | 5   40(SP)    | 6   48(SP)    | 7   56(SP)    | 8   64(SP)    | 9   72(SP)    | 10  80(SP)    | 11  88(SP)    
	// | 12  96(SP)    | 13  104(SP)   | 14  112(SP)   | 15  120(SP)   | 16  128(SP)   | 17  136(SP)   | 18  144(SP)   | 19  152(SP)   | 20  -         | 21  -         | 22  -         | 23  -         


	MOVQ 64(SP), AX
	ADDQ AX, 160(SP)
	MOVQ 72(SP), AX
	ADCQ AX, 168(SP)
	MOVQ 80(SP), AX
	ADCQ AX, 176(SP)
	MOVQ 88(SP), AX
	ADCQ AX, 184(SP)
	MOVQ 96(SP), AX
	ADCQ AX, 192(SP)
	MOVQ 104(SP), AX
	ADCQ AX, 200(SP)
	MOVQ 112(SP), AX
	ADCQ AX, 208(SP)
	ADCQ 120(SP), R14
	ADCQ 128(SP), R15
	ADCQ 136(SP), R8
	ADCQ 144(SP), R9
	ADCQ 152(SP), R10
	ADCQ $0x00, R11
	ADCQ $0x00, R12
	ADCQ $0x00, R13
	ADCQ $0x00, BX

	// | 
	// | W combined
	// | 0   (SP)      | 1   8(SP)     | 2   16(SP)    | 3   24(SP)    | 4   32(SP)    | 5   40(SP)    | 6   48(SP)    | 7   56(SP)    | 8   160(SP)   | 9   168(SP)   | 10  176(SP)   | 11  184(SP)   
	// | 12  192(SP)   | 13  200(SP)   | 14  208(SP)   | 15  R14       | 16  R15       | 17  R8        | 18  R9        | 19  R10       | 20  R11       | 21  R12       | 22  R13       | 23  BX        


	MOVQ (SP), CX
	MOVQ 8(SP), DI
	MOVQ 16(SP), SI
	MOVQ BX, (SP)
	MOVQ 24(SP), BX
	MOVQ R13, 8(SP)
	MOVQ 32(SP), R13
	MOVQ R12, 16(SP)
	MOVQ 40(SP), R12
	MOVQ R11, 24(SP)
	MOVQ 48(SP), R11
	MOVQ R10, 32(SP)
	MOVQ 56(SP), R10
	MOVQ R9, 40(SP)
	MOVQ 160(SP), R9
	MOVQ R8, 48(SP)
	MOVQ 168(SP), R8
	MOVQ R15, 56(SP)
	MOVQ R14, 64(SP)

	// | 

/* montgomery reduction q1                 */

	// | 

/* i = 0                                   */

	// | 
	// | W
	// | 0   CX        | 1   DI        | 2   SI        | 3   BX        | 4   R13       | 5   R12       | 6   R11       | 7   R10       | 8   R9        | 9   R8        | 10  176(SP)   | 11  184(SP)   
	// | 12  192(SP)   | 13  200(SP)   | 14  208(SP)   | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u0 = w0 * inp
	MOVQ CX, AX
	MULQ ·inp+0(SB)
	MOVQ AX, R15
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u0
	MOVQ R15, 72(SP)

	// | j0

	// | w0 @ CX
	MOVQ ·modulus+0(SB), AX
	MULQ R15
	ADDQ AX, CX
	ADCQ DX, R14

	// | j1

	// | w1 @ DI
	MOVQ ·modulus+8(SB), AX
	MULQ R15
	ADDQ AX, DI
	ADCQ $0x00, DX
	ADDQ R14, DI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w2 @ SI
	MOVQ ·modulus+16(SB), AX
	MULQ R15
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w3 @ BX
	MOVQ ·modulus+24(SB), AX
	MULQ R15
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w4 @ R13
	MOVQ ·modulus+32(SB), AX
	MULQ R15
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w5 @ R12
	MOVQ ·modulus+40(SB), AX
	MULQ R15
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w6 @ R11
	MOVQ ·modulus+48(SB), AX
	MULQ R15
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w7 @ R10
	MOVQ ·modulus+56(SB), AX
	MULQ R15
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w8 @ R9
	MOVQ ·modulus+64(SB), AX
	MULQ R15
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9

	// | w9 @ R8
	ADCQ DX, R8
	ADCQ $0x00, CX

	// | 

/* i = 1                                   */

	// | 
	// | W
	// | 0   -         | 1   DI        | 2   SI        | 3   BX        | 4   R13       | 5   R12       | 6   R11       | 7   R10       | 8   R9        | 9   R8        | 10  176(SP)   | 11  184(SP)   
	// | 12  192(SP)   | 13  200(SP)   | 14  208(SP)   | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u1 = w1 * inp
	MOVQ DI, AX
	MULQ ·inp+0(SB)
	MOVQ AX, R15
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u1
	MOVQ R15, 80(SP)

	// | j0

	// | w1 @ DI
	MOVQ ·modulus+0(SB), AX
	MULQ R15
	ADDQ AX, DI
	ADCQ DX, R14

	// | j1

	// | w2 @ SI
	MOVQ ·modulus+8(SB), AX
	MULQ R15
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w3 @ BX
	MOVQ ·modulus+16(SB), AX
	MULQ R15
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w4 @ R13
	MOVQ ·modulus+24(SB), AX
	MULQ R15
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w5 @ R12
	MOVQ ·modulus+32(SB), AX
	MULQ R15
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w6 @ R11
	MOVQ ·modulus+40(SB), AX
	MULQ R15
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w7 @ R10
	MOVQ ·modulus+48(SB), AX
	MULQ R15
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w8 @ R9
	MOVQ ·modulus+56(SB), AX
	MULQ R15
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w9 @ R8
	MOVQ ·modulus+64(SB), AX
	MULQ R15
	ADDQ AX, R8
	ADCQ DX, CX
	ADDQ R14, R8

	// | move to idle register
	MOVQ 176(SP), DI

	// | w10 @ DI
	ADCQ CX, DI
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 2                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   SI        | 3   BX        | 4   R13       | 5   R12       | 6   R11       | 7   R10       | 8   R9        | 9   R8        | 10  DI        | 11  184(SP)   
	// | 12  192(SP)   | 13  200(SP)   | 14  208(SP)   | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u2 = w2 * inp
	MOVQ SI, AX
	MULQ ·inp+0(SB)
	MOVQ AX, R15
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u2
	MOVQ R15, 88(SP)

	// | j0

	// | w2 @ SI
	MOVQ ·modulus+0(SB), AX
	MULQ R15
	ADDQ AX, SI
	ADCQ DX, R14

	// | j1

	// | w3 @ BX
	MOVQ ·modulus+8(SB), AX
	MULQ R15
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w4 @ R13
	MOVQ ·modulus+16(SB), AX
	MULQ R15
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w5 @ R12
	MOVQ ·modulus+24(SB), AX
	MULQ R15
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w6 @ R11
	MOVQ ·modulus+32(SB), AX
	MULQ R15
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w7 @ R10
	MOVQ ·modulus+40(SB), AX
	MULQ R15
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w8 @ R9
	MOVQ ·modulus+48(SB), AX
	MULQ R15
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w9 @ R8
	MOVQ ·modulus+56(SB), AX
	MULQ R15
	ADDQ AX, R8
	ADCQ $0x00, DX
	ADDQ R14, R8
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w10 @ DI
	MOVQ ·modulus+64(SB), AX
	MULQ R15
	ADDQ AX, DI
	ADCQ DX, CX
	ADDQ R14, DI

	// | move to idle register
	MOVQ 184(SP), SI

	// | w11 @ SI
	ADCQ CX, SI
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 3                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   BX        | 4   R13       | 5   R12       | 6   R11       | 7   R10       | 8   R9        | 9   R8        | 10  DI        | 11  SI        
	// | 12  192(SP)   | 13  200(SP)   | 14  208(SP)   | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u3 = w3 * inp
	MOVQ BX, AX
	MULQ ·inp+0(SB)
	MOVQ AX, R15
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u3
	MOVQ R15, 96(SP)

	// | j0

	// | w3 @ BX
	MOVQ ·modulus+0(SB), AX
	MULQ R15
	ADDQ AX, BX
	ADCQ DX, R14

	// | j1

	// | w4 @ R13
	MOVQ ·modulus+8(SB), AX
	MULQ R15
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w5 @ R12
	MOVQ ·modulus+16(SB), AX
	MULQ R15
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w6 @ R11
	MOVQ ·modulus+24(SB), AX
	MULQ R15
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w7 @ R10
	MOVQ ·modulus+32(SB), AX
	MULQ R15
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w8 @ R9
	MOVQ ·modulus+40(SB), AX
	MULQ R15
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w9 @ R8
	MOVQ ·modulus+48(SB), AX
	MULQ R15
	ADDQ AX, R8
	ADCQ $0x00, DX
	ADDQ R14, R8
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w10 @ DI
	MOVQ ·modulus+56(SB), AX
	MULQ R15
	ADDQ AX, DI
	ADCQ $0x00, DX
	ADDQ R14, DI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w11 @ SI
	MOVQ ·modulus+64(SB), AX
	MULQ R15
	ADDQ AX, SI
	ADCQ DX, CX
	ADDQ R14, SI

	// | move to idle register
	MOVQ 192(SP), BX

	// | w12 @ BX
	ADCQ CX, BX
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 4                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   R13       | 5   R12       | 6   R11       | 7   R10       | 8   R9        | 9   R8        | 10  DI        | 11  SI        
	// | 12  BX        | 13  200(SP)   | 14  208(SP)   | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u4 = w4 * inp
	MOVQ R13, AX
	MULQ ·inp+0(SB)
	MOVQ AX, R15
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u4
	MOVQ R15, 104(SP)

	// | j0

	// | w4 @ R13
	MOVQ ·modulus+0(SB), AX
	MULQ R15
	ADDQ AX, R13
	ADCQ DX, R14

	// | j1

	// | w5 @ R12
	MOVQ ·modulus+8(SB), AX
	MULQ R15
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w6 @ R11
	MOVQ ·modulus+16(SB), AX
	MULQ R15
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w7 @ R10
	MOVQ ·modulus+24(SB), AX
	MULQ R15
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w8 @ R9
	MOVQ ·modulus+32(SB), AX
	MULQ R15
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w9 @ R8
	MOVQ ·modulus+40(SB), AX
	MULQ R15
	ADDQ AX, R8
	ADCQ $0x00, DX
	ADDQ R14, R8
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w10 @ DI
	MOVQ ·modulus+48(SB), AX
	MULQ R15
	ADDQ AX, DI
	ADCQ $0x00, DX
	ADDQ R14, DI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w11 @ SI
	MOVQ ·modulus+56(SB), AX
	MULQ R15
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w12 @ BX
	MOVQ ·modulus+64(SB), AX
	MULQ R15
	ADDQ AX, BX
	ADCQ DX, CX
	ADDQ R14, BX

	// | move to idle register
	MOVQ 200(SP), R13

	// | w13 @ R13
	ADCQ CX, R13
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 5                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   R12       | 6   R11       | 7   R10       | 8   R9        | 9   R8        | 10  DI        | 11  SI        
	// | 12  BX        | 13  R13       | 14  208(SP)   | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u5 = w5 * inp
	MOVQ R12, AX
	MULQ ·inp+0(SB)
	MOVQ AX, R15
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u5
	MOVQ R15, 112(SP)

	// | j0

	// | w5 @ R12
	MOVQ ·modulus+0(SB), AX
	MULQ R15
	ADDQ AX, R12
	ADCQ DX, R14

	// | j1

	// | w6 @ R11
	MOVQ ·modulus+8(SB), AX
	MULQ R15
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w7 @ R10
	MOVQ ·modulus+16(SB), AX
	MULQ R15
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w8 @ R9
	MOVQ ·modulus+24(SB), AX
	MULQ R15
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w9 @ R8
	MOVQ ·modulus+32(SB), AX
	MULQ R15
	ADDQ AX, R8
	ADCQ $0x00, DX
	ADDQ R14, R8
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w10 @ DI
	MOVQ ·modulus+40(SB), AX
	MULQ R15
	ADDQ AX, DI
	ADCQ $0x00, DX
	ADDQ R14, DI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w11 @ SI
	MOVQ ·modulus+48(SB), AX
	MULQ R15
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w12 @ BX
	MOVQ ·modulus+56(SB), AX
	MULQ R15
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w13 @ R13
	MOVQ ·modulus+64(SB), AX
	MULQ R15
	ADDQ AX, R13
	ADCQ DX, CX
	ADDQ R14, R13

	// | move to idle register
	MOVQ 208(SP), R12

	// | w14 @ R12
	ADCQ CX, R12
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 6                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   R11       | 7   R10       | 8   R9        | 9   R8        | 10  DI        | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u6 = w6 * inp
	MOVQ R11, AX
	MULQ ·inp+0(SB)
	MOVQ AX, R15
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u6
	MOVQ R15, 120(SP)

	// | j0

	// | w6 @ R11
	MOVQ ·modulus+0(SB), AX
	MULQ R15
	ADDQ AX, R11
	ADCQ DX, R14

	// | j1

	// | w7 @ R10
	MOVQ ·modulus+8(SB), AX
	MULQ R15
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w8 @ R9
	MOVQ ·modulus+16(SB), AX
	MULQ R15
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w9 @ R8
	MOVQ ·modulus+24(SB), AX
	MULQ R15
	ADDQ AX, R8
	ADCQ $0x00, DX
	ADDQ R14, R8
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w10 @ DI
	MOVQ ·modulus+32(SB), AX
	MULQ R15
	ADDQ AX, DI
	ADCQ $0x00, DX
	ADDQ R14, DI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w11 @ SI
	MOVQ ·modulus+40(SB), AX
	MULQ R15
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w12 @ BX
	MOVQ ·modulus+48(SB), AX
	MULQ R15
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w13 @ R13
	MOVQ ·modulus+56(SB), AX
	MULQ R15
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w14 @ R12
	MOVQ ·modulus+64(SB), AX
	MULQ R15
	ADDQ AX, R12
	ADCQ DX, CX
	ADDQ R14, R12

	// | move to idle register
	MOVQ 64(SP), R11

	// | w15 @ R11
	ADCQ CX, R11
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 7                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   R10       | 8   R9        | 9   R8        | 10  DI        | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u7 = w7 * inp
	MOVQ R10, AX
	MULQ ·inp+0(SB)
	MOVQ AX, R15
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u7
	MOVQ R15, 64(SP)

	// | j0

	// | w7 @ R10
	MOVQ ·modulus+0(SB), AX
	MULQ R15
	ADDQ AX, R10
	ADCQ DX, R14

	// | j1

	// | w8 @ R9
	MOVQ ·modulus+8(SB), AX
	MULQ R15
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w9 @ R8
	MOVQ ·modulus+16(SB), AX
	MULQ R15
	ADDQ AX, R8
	ADCQ $0x00, DX
	ADDQ R14, R8
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w10 @ DI
	MOVQ ·modulus+24(SB), AX
	MULQ R15
	ADDQ AX, DI
	ADCQ $0x00, DX
	ADDQ R14, DI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w11 @ SI
	MOVQ ·modulus+32(SB), AX
	MULQ R15
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w12 @ BX
	MOVQ ·modulus+40(SB), AX
	MULQ R15
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w13 @ R13
	MOVQ ·modulus+48(SB), AX
	MULQ R15
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w14 @ R12
	MOVQ ·modulus+56(SB), AX
	MULQ R15
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w15 @ R11
	MOVQ ·modulus+64(SB), AX
	MULQ R15
	ADDQ AX, R11
	ADCQ DX, CX
	ADDQ R14, R11

	// | move to idle register
	MOVQ 56(SP), R10

	// | w16 @ R10
	ADCQ CX, R10
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 8                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   R9        | 9   R8        | 10  DI        | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u8 = w8 * inp
	MOVQ R9, AX
	MULQ ·inp+0(SB)
	MOVQ AX, R15
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u8
	MOVQ R15, 56(SP)

	// | j0

	// | w8 @ R9
	MOVQ ·modulus+0(SB), AX
	MULQ R15
	ADDQ AX, R9
	ADCQ DX, R14

	// | j1

	// | w9 @ R8
	MOVQ ·modulus+8(SB), AX
	MULQ R15
	ADDQ AX, R8
	ADCQ $0x00, DX
	ADDQ R14, R8
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w10 @ DI
	MOVQ ·modulus+16(SB), AX
	MULQ R15
	ADDQ AX, DI
	ADCQ $0x00, DX
	ADDQ R14, DI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w11 @ SI
	MOVQ ·modulus+24(SB), AX
	MULQ R15
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w12 @ BX
	MOVQ ·modulus+32(SB), AX
	MULQ R15
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w13 @ R13
	MOVQ ·modulus+40(SB), AX
	MULQ R15
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w14 @ R12
	MOVQ ·modulus+48(SB), AX
	MULQ R15
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w15 @ R11
	MOVQ ·modulus+56(SB), AX
	MULQ R15
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w16 @ R10
	MOVQ ·modulus+64(SB), AX
	MULQ R15
	ADDQ AX, R10
	ADCQ DX, CX
	ADDQ R14, R10

	// | move to idle register
	MOVQ 48(SP), R9

	// | w17 @ R9
	ADCQ CX, R9
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 
	// | W q1
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  DI        | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | save the carry from q1
	// | should be added to w18
	MOVQ CX, 48(SP)

	// | 

/* montgomerry reduction q2                */

	// | 

/* i = 0                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  DI        | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w9 @ R8
	MOVQ ·modulus+72(SB), AX
	MULQ 72(SP)
	ADDQ AX, R8
	ADCQ DX, R14

	// | j10

	// | w10 @ DI
	MOVQ ·modulus+80(SB), AX
	MULQ 72(SP)
	ADDQ AX, DI
	ADCQ $0x00, DX
	ADDQ R14, DI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w11 @ SI
	MOVQ ·modulus+88(SB), AX
	MULQ 72(SP)
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI

	// | w12 @ BX
	ADCQ DX, BX
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 1                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  DI        | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w10 @ DI
	MOVQ ·modulus+72(SB), AX
	MULQ 80(SP)
	ADDQ AX, DI
	ADCQ DX, R14
	MOVQ DI, 72(SP)

	// | j10

	// | w11 @ SI
	MOVQ ·modulus+80(SB), AX
	MULQ 80(SP)
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w12 @ BX
	MOVQ ·modulus+88(SB), AX
	MULQ 80(SP)
	ADDQ AX, BX
	ADCQ DX, CX
	ADDQ R14, BX

	// | w13 @ R13
	ADCQ CX, R13
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 2                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  72(SP)    | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w11 @ SI
	MOVQ ·modulus+72(SB), AX
	MULQ 88(SP)
	ADDQ AX, SI
	ADCQ DX, R14

	// | j10

	// | w12 @ BX
	MOVQ ·modulus+80(SB), AX
	MULQ 88(SP)
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w13 @ R13
	MOVQ ·modulus+88(SB), AX
	MULQ 88(SP)
	ADDQ AX, R13
	ADCQ DX, CX
	ADDQ R14, R13

	// | w14 @ R12
	ADCQ CX, R12
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 3                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  72(SP)    | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w12 @ BX
	MOVQ ·modulus+72(SB), AX
	MULQ 96(SP)
	ADDQ AX, BX
	ADCQ DX, R14

	// | j10

	// | w13 @ R13
	MOVQ ·modulus+80(SB), AX
	MULQ 96(SP)
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w14 @ R12
	MOVQ ·modulus+88(SB), AX
	MULQ 96(SP)
	ADDQ AX, R12
	ADCQ DX, CX
	ADDQ R14, R12

	// | w15 @ R11
	ADCQ CX, R11
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 4                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  72(SP)    | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w13 @ R13
	MOVQ ·modulus+72(SB), AX
	MULQ 104(SP)
	ADDQ AX, R13
	ADCQ DX, R14

	// | j10

	// | w14 @ R12
	MOVQ ·modulus+80(SB), AX
	MULQ 104(SP)
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w15 @ R11
	MOVQ ·modulus+88(SB), AX
	MULQ 104(SP)
	ADDQ AX, R11
	ADCQ DX, CX
	ADDQ R14, R11

	// | w16 @ R10
	ADCQ CX, R10
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 5                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  72(SP)    | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w14 @ R12
	MOVQ ·modulus+72(SB), AX
	MULQ 112(SP)
	ADDQ AX, R12
	ADCQ DX, R14

	// | j10

	// | w15 @ R11
	MOVQ ·modulus+80(SB), AX
	MULQ 112(SP)
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w16 @ R10
	MOVQ ·modulus+88(SB), AX
	MULQ 112(SP)
	ADDQ AX, R10
	ADCQ DX, CX
	ADDQ R14, R10

	// | w17 @ R9
	ADCQ CX, R9

	// | bring the carry from q1
	MOVQ 48(SP), CX
	ADCQ $0x00, CX

	// | 

/* i = 6                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  72(SP)    | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w15 @ R11
	MOVQ ·modulus+72(SB), AX
	MULQ 120(SP)
	ADDQ AX, R11
	ADCQ DX, R14

	// | j10

	// | w16 @ R10
	MOVQ ·modulus+80(SB), AX
	MULQ 120(SP)
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w17 @ R9
	MOVQ ·modulus+88(SB), AX
	MULQ 120(SP)
	ADDQ AX, R9
	ADCQ DX, CX
	ADDQ R14, R9

	// | move to an idle register
	MOVQ 40(SP), R15

	// | w18 @ R15
	ADCQ CX, R15
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 7                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  72(SP)    | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w16 @ R10
	MOVQ ·modulus+72(SB), AX
	MULQ 64(SP)
	ADDQ AX, R10
	ADCQ DX, R14

	// | j10

	// | w17 @ R9
	MOVQ ·modulus+80(SB), AX
	MULQ 64(SP)
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w18 @ R15
	MOVQ ·modulus+88(SB), AX
	MULQ 64(SP)
	ADDQ AX, R15
	ADCQ DX, CX
	ADDQ R14, R15

	// | move to an idle register
	MOVQ 32(SP), DI

	// | w19 @ DI
	ADCQ CX, DI
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 

/* i = 8                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  72(SP)    | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  DI        | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w17 @ R9
	MOVQ ·modulus+72(SB), AX
	MULQ 56(SP)
	ADDQ AX, R9
	ADCQ DX, R14

	// | j10

	// | w18 @ R15
	MOVQ ·modulus+80(SB), AX
	MULQ 56(SP)
	ADDQ AX, R15
	ADCQ $0x00, DX
	ADDQ R14, R15
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w19 @ DI
	MOVQ ·modulus+88(SB), AX
	MULQ 56(SP)
	ADDQ AX, DI
	ADCQ DX, CX
	ADDQ R14, DI

	// | tolarete this limb to stay in stack
	// | w20 @ 24(SP)
	ADCQ CX, 24(SP)
	MOVQ $0x00, CX
	ADCQ $0x00, CX

	// | 
	// | q2
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  72(SP)    | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  DI        | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | save the carry from q2
	// | should be added to w21
	MOVQ CX, 48(SP)

	// | 

/* q2 q3 transition swap                   */

	MOVQ 72(SP), CX
	MOVQ DI, 72(SP)

	// | 
	// | W q2 q3 transition
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  CX        | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  72(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | 

/* montgomery reduction q3                 */

	// | 

/* i = 9                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R8        | 10  CX        | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  72(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u9 = w9 * inp
	MOVQ R8, AX
	MULQ ·inp+0(SB)
	MOVQ AX, DI
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u9
	MOVQ DI, 56(SP)

	// | j0

	// | w9 @ R8
	MOVQ ·modulus+0(SB), AX
	MULQ DI
	ADDQ AX, R8
	ADCQ DX, R14

	// | j1

	// | w10 @ CX
	MOVQ ·modulus+8(SB), AX
	MULQ DI
	ADDQ AX, CX
	ADCQ $0x00, DX
	ADDQ R14, CX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w11 @ SI
	MOVQ ·modulus+16(SB), AX
	MULQ DI
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w12 @ BX
	MOVQ ·modulus+24(SB), AX
	MULQ DI
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w13 @ R13
	MOVQ ·modulus+32(SB), AX
	MULQ DI
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w14 @ R12
	MOVQ ·modulus+40(SB), AX
	MULQ DI
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w15 @ R11
	MOVQ ·modulus+48(SB), AX
	MULQ DI
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w16 @ R10
	MOVQ ·modulus+56(SB), AX
	MULQ DI
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w17 @ R9
	MOVQ ·modulus+64(SB), AX
	MULQ DI
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9

	// | w18 @ R15
	ADCQ DX, R15
	ADCQ $0x00, R8

	// | 

/* i = 10                                  */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  CX        | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  72(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u10 = w10 * inp
	MOVQ CX, AX
	MULQ ·inp+0(SB)
	MOVQ AX, DI
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u10
	MOVQ DI, 64(SP)

	// | j0

	// | w10 @ CX
	MOVQ ·modulus+0(SB), AX
	MULQ DI
	ADDQ AX, CX
	ADCQ DX, R14

	// | j1

	// | w11 @ SI
	MOVQ ·modulus+8(SB), AX
	MULQ DI
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w12 @ BX
	MOVQ ·modulus+16(SB), AX
	MULQ DI
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w13 @ R13
	MOVQ ·modulus+24(SB), AX
	MULQ DI
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w14 @ R12
	MOVQ ·modulus+32(SB), AX
	MULQ DI
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w15 @ R11
	MOVQ ·modulus+40(SB), AX
	MULQ DI
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w16 @ R10
	MOVQ ·modulus+48(SB), AX
	MULQ DI
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w17 @ R9
	MOVQ ·modulus+56(SB), AX
	MULQ DI
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w18 @ R15
	MOVQ ·modulus+64(SB), AX
	MULQ DI
	ADDQ AX, R15
	ADCQ DX, R8
	ADDQ R14, R15

	// | move to idle register
	MOVQ 72(SP), CX

	// | w19 @ CX
	ADCQ R8, CX
	MOVQ $0x00, R8
	ADCQ $0x00, R8

	// | 

/* i = 11                                  */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  SI        
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  CX        | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u11 = w11 * inp
	MOVQ SI, AX
	MULQ ·inp+0(SB)
	MOVQ AX, DI
	MOVQ $0x00, R14

	// | 

/*                                         */

	// | save u11
	MOVQ DI, 72(SP)

	// | j0

	// | w11 @ SI
	MOVQ ·modulus+0(SB), AX
	MULQ DI
	ADDQ AX, SI
	ADCQ DX, R14

	// | j1

	// | w12 @ BX
	MOVQ ·modulus+8(SB), AX
	MULQ DI
	ADDQ AX, BX
	ADCQ $0x00, DX
	ADDQ R14, BX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j2

	// | w13 @ R13
	MOVQ ·modulus+16(SB), AX
	MULQ DI
	ADDQ AX, R13
	ADCQ $0x00, DX
	ADDQ R14, R13
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j3

	// | w14 @ R12
	MOVQ ·modulus+24(SB), AX
	MULQ DI
	ADDQ AX, R12
	ADCQ $0x00, DX
	ADDQ R14, R12
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j4

	// | w15 @ R11
	MOVQ ·modulus+32(SB), AX
	MULQ DI
	ADDQ AX, R11
	ADCQ $0x00, DX
	ADDQ R14, R11
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j5

	// | w16 @ R10
	MOVQ ·modulus+40(SB), AX
	MULQ DI
	ADDQ AX, R10
	ADCQ $0x00, DX
	ADDQ R14, R10
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j6

	// | w17 @ R9
	MOVQ ·modulus+48(SB), AX
	MULQ DI
	ADDQ AX, R9
	ADCQ $0x00, DX
	ADDQ R14, R9
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j7

	// | w18 @ R15
	MOVQ ·modulus+56(SB), AX
	MULQ DI
	ADDQ AX, R15
	ADCQ $0x00, DX
	ADDQ R14, R15
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j8

	// | w19 @ CX
	MOVQ ·modulus+64(SB), AX
	MULQ DI
	ADDQ AX, CX
	ADCQ DX, R8
	ADDQ R14, CX

	// | move to idle register
	MOVQ 24(SP), SI

	// | w20 @ SI
	ADCQ R8, SI
	MOVQ $0x00, R8
	ADCQ $0x00, R8

	// | 
	// | W q3
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  -         
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  CX        | 20  SI        | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | aggregate carries from q2 & q3
	// | should be added to w21
	ADCQ R8, 48(SP)

	// | 

/* montgomerry reduction q4                */

	// | 

/* i = 0                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  -         
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  CX        | 20  SI        | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w18 @ R15
	MOVQ ·modulus+72(SB), AX
	MULQ 56(SP)
	ADDQ AX, R15
	ADCQ DX, R14

	// | j10

	// | w19 @ CX
	MOVQ ·modulus+80(SB), AX
	MULQ 56(SP)
	ADDQ AX, CX
	ADCQ $0x00, DX
	ADDQ R14, CX
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w20 @ SI
	MOVQ ·modulus+88(SB), AX
	MULQ 56(SP)
	ADDQ AX, SI
	ADCQ 48(SP), DX
	ADDQ R14, SI
	MOVQ 16(SP), DI

	// | w21 @ DI
	ADCQ DX, DI
	MOVQ $0x00, R8
	ADCQ $0x00, R8

	// | 

/* i = 1                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  -         
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  CX        | 20  SI        | 21  DI        | 22  8(SP)     | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w19 @ CX
	MOVQ ·modulus+72(SB), AX
	MULQ 64(SP)
	ADDQ AX, CX
	ADCQ DX, R14
	MOVQ CX, 24(SP)

	// | j10

	// | w20 @ SI
	MOVQ ·modulus+80(SB), AX
	MULQ 64(SP)
	ADDQ AX, SI
	ADCQ $0x00, DX
	ADDQ R14, SI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w21 @ DI
	MOVQ ·modulus+88(SB), AX
	MULQ 64(SP)
	ADDQ AX, DI
	ADCQ DX, R8
	ADDQ R14, DI
	MOVQ 8(SP), CX

	// | w22 @ CX
	ADCQ R8, CX
	MOVQ $0x00, R8
	ADCQ $0x00, R8

	// | 

/* i = 2                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  -         
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  24(SP)    | 20  SI        | 21  DI        | 22  CX        | 23  (SP)      


	MOVQ $0x00, R14

	// | 

/*                                         */

	// | j9

	// | w20 @ SI
	MOVQ ·modulus+72(SB), AX
	MULQ 72(SP)
	ADDQ AX, SI
	ADCQ DX, R14

	// | j10

	// | w21 @ DI
	MOVQ ·modulus+80(SB), AX
	MULQ 72(SP)
	ADDQ AX, DI
	ADCQ $0x00, DX
	ADDQ R14, DI
	MOVQ $0x00, R14
	ADCQ DX, R14

	// | j11

	// | w22 @ CX
	MOVQ ·modulus+88(SB), AX
	MULQ 72(SP)
	ADDQ AX, CX
	ADCQ DX, R8
	ADDQ R14, CX

	// | very last limb goes to short carry register
	MOVQ (SP), R14

	// | w-1 @ R14
	ADCQ R8, R14
	MOVQ $0x00, R8
	ADCQ $0x00, R8

	// | 
	// | W q4
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  -         
	// | 12  BX        | 13  R13       | 14  R12       | 15  R11       | 16  R10       | 17  R9        | 18  R15       | 19  24(SP)    | 20  SI        | 21  DI        | 22  CX        | 23  R14       


	// | 

/* modular reduction                       */

	MOVQ BX, DX
	SUBQ ·modulus+0(SB), DX
	MOVQ DX, (SP)
	MOVQ R13, DX
	SBBQ ·modulus+8(SB), DX
	MOVQ DX, 8(SP)
	MOVQ R12, DX
	SBBQ ·modulus+16(SB), DX
	MOVQ DX, 80(SP)
	MOVQ R11, DX
	SBBQ ·modulus+24(SB), DX
	MOVQ DX, 88(SP)
	MOVQ R10, DX
	SBBQ ·modulus+32(SB), DX
	MOVQ DX, 96(SP)
	MOVQ R9, DX
	SBBQ ·modulus+40(SB), DX
	MOVQ DX, 104(SP)
	MOVQ R15, DX
	SBBQ ·modulus+48(SB), DX
	MOVQ DX, 112(SP)
	MOVQ 24(SP), DX
	SBBQ ·modulus+56(SB), DX
	MOVQ DX, 120(SP)
	MOVQ SI, DX
	SBBQ ·modulus+64(SB), DX
	MOVQ DX, 128(SP)
	MOVQ DI, DX
	SBBQ ·modulus+72(SB), DX
	MOVQ DX, 136(SP)
	MOVQ CX, DX
	SBBQ ·modulus+80(SB), DX
	MOVQ DX, 144(SP)
	MOVQ R14, DX
	SBBQ ·modulus+88(SB), DX
	MOVQ DX, 152(SP)
	SBBQ $0x00, R8

	// | 

/* out                                     */

	MOVQ    c+0(FP), R8
	CMOVQCC (SP), BX
	MOVQ    BX, (R8)
	CMOVQCC 8(SP), R13
	MOVQ    R13, 8(R8)
	CMOVQCC 80(SP), R12
	MOVQ    R12, 16(R8)
	CMOVQCC 88(SP), R11
	MOVQ    R11, 24(R8)
	CMOVQCC 96(SP), R10
	MOVQ    R10, 32(R8)
	CMOVQCC 104(SP), R9
	MOVQ    R9, 40(R8)
	CMOVQCC 112(SP), R15
	MOVQ    R15, 48(R8)
	MOVQ    24(SP), DX
	CMOVQCC 120(SP), DX
	MOVQ    DX, 56(R8)
	CMOVQCC 128(SP), SI
	MOVQ    SI, 64(R8)
	CMOVQCC 136(SP), DI
	MOVQ    DI, 72(R8)
	CMOVQCC 144(SP), CX
	MOVQ    CX, 80(R8)
	CMOVQCC 152(SP), R14
	MOVQ    R14, 88(R8)
	RET

	// | 

/* end                                     */


// func mulADX(c *[12]uint64, a *[12]uint64, b *[12]uint64)
TEXT ·mulADX(SB), NOSPLIT, $208-24
	// | 

/* inputs                                  */

	MOVQ a+8(FP), DI
	MOVQ b+16(FP), SI
	XORQ AX, AX

	// | 

/* i = 0                                   */

	// | a0 @ DX
	MOVQ (DI), DX

	// | a0 * b0 
	MULXQ (SI), AX, CX
	MOVQ  AX, (SP)

	// | a0 * b1 
	MULXQ 8(SI), AX, R8
	ADCXQ AX, CX

	// | a0 * b2 
	MULXQ 16(SI), AX, R9
	ADCXQ AX, R8

	// | a0 * b3 
	MULXQ 24(SI), AX, R10
	ADCXQ AX, R9

	// | a0 * b4 
	MULXQ 32(SI), AX, R11
	ADCXQ AX, R10

	// | a0 * b5 
	MULXQ 40(SI), AX, R12
	ADCXQ AX, R11

	// | a0 * b6 
	MULXQ 48(SI), AX, R13
	ADCXQ AX, R12

	// | a0 * b7 
	MULXQ 56(SI), AX, R14
	ADCXQ AX, R13

	// | a0 * b8 
	MULXQ 64(SI), AX, R15
	ADCXQ AX, R14
	ADCQ  $0x00, R15

	// | 

/* i = 1                                   */

	// | a1 @ DX
	MOVQ 8(DI), DX
	XORQ AX, AX

	// | a1 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8
	MOVQ  CX, 8(SP)
	MOVQ  $0x00, CX

	// | a1 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a1 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a1 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	// | a1 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a1 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a1 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a1 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a1 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, R15
	ADOXQ CX, CX
	ADCXQ BX, CX

	// | 

/* i = 2                                   */

	// | a2 @ DX
	MOVQ 16(DI), DX
	XORQ AX, AX

	// | a2 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9
	MOVQ  R8, 16(SP)
	MOVQ  $0x00, R8

	// | a2 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a2 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	// | a2 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a2 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a2 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a2 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a2 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX

	// | a2 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, CX
	ADOXQ R8, R8
	ADCXQ BX, R8

	// | 

/* i = 3                                   */

	// | a3 @ DX
	MOVQ 24(DI), DX
	XORQ AX, AX

	// | a3 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10
	MOVQ  R9, 24(SP)
	MOVQ  $0x00, R9

	// | a3 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	// | a3 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a3 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a3 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a3 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a3 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX

	// | a3 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8

	// | a3 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, R8
	ADOXQ R9, R9
	ADCXQ BX, R9

	// | 

/* i = 4                                   */

	// | a4 @ DX
	MOVQ 32(DI), DX
	XORQ AX, AX

	// | a4 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11
	MOVQ  R10, 32(SP)
	MOVQ  $0x00, R10

	// | a4 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a4 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a4 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a4 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a4 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX

	// | a4 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8

	// | a4 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a4 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, R9
	ADOXQ R10, R10
	ADCXQ BX, R10

	// | 

/* i = 5                                   */

	// | a5 @ DX
	MOVQ 40(DI), DX
	XORQ AX, AX

	// | a5 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12
	MOVQ  R11, 40(SP)
	MOVQ  $0x00, R11

	// | a5 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a5 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a5 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a5 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX

	// | a5 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8

	// | a5 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a5 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a5 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, R10
	ADOXQ R11, R11
	ADCXQ BX, R11

	// | 

/* i = 6                                   */

	// | a6 @ DX
	MOVQ 48(DI), DX
	XORQ AX, AX

	// | a6 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13
	MOVQ  R12, 48(SP)
	MOVQ  $0x00, R12

	// | a6 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a6 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a6 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX

	// | a6 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8

	// | a6 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a6 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a6 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	// | a6 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, R11
	ADOXQ R12, R12
	ADCXQ BX, R12

	// | 

/* i = 7                                   */

	// | a7 @ DX
	MOVQ 56(DI), DX
	XORQ AX, AX

	// | a7 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14
	MOVQ  R13, 56(SP)
	MOVQ  $0x00, R13

	// | a7 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a7 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX

	// | a7 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8

	// | a7 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a7 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a7 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	// | a7 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a7 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, R12
	ADOXQ R13, R13
	ADCXQ BX, R13

	// | 

/* i = 8                                   */

	// | a8 @ DX
	MOVQ 64(DI), DX
	XORQ AX, AX

	// | a8 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15
	MOVQ  R14, 64(SP)
	MOVQ  $0x00, R14

	// | a8 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX

	// | a8 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8

	// | a8 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a8 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a8 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	// | a8 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a8 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a8 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, R13
	ADOXQ R14, R14
	ADCXQ BX, R14

	// | 

/* i = 9                                   */

	// | a9 @ DX
	MOVQ 72(DI), DX
	XORQ AX, AX

	// | a9 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX
	MOVQ  R15, 72(SP)
	MOVQ  $0x00, R15

	// | a9 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8

	// | a9 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a9 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a9 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	// | a9 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a9 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a9 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a9 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, R14
	ADOXQ R15, R15
	ADCXQ BX, R15

	// | 

/* i = 10                                  */

	// | a10 @ DX
	MOVQ 80(DI), DX
	XORQ AX, AX

	// | a10 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8
	MOVQ  CX, 80(SP)
	MOVQ  $0x00, CX

	// | a10 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a10 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a10 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	// | a10 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a10 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a10 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a10 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a10 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, R15
	ADOXQ CX, CX
	ADCXQ BX, CX

	// | 

/* i = 11                                  */

	// | a11 @ DX
	MOVQ 88(DI), DX
	XORQ AX, AX

	// | a11 * b0 
	MULXQ (SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9
	MOVQ  R8, 88(SP)
	MOVQ  $0x00, R8

	// | a11 * b1 
	MULXQ 8(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a11 * b2 
	MULXQ 16(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	// | a11 * b3 
	MULXQ 24(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a11 * b4 
	MULXQ 32(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a11 * b5 
	MULXQ 40(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a11 * b6 
	MULXQ 48(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a11 * b7 
	MULXQ 56(SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX

	// | a11 * b8 
	MULXQ 64(SI), AX, BX
	ADOXQ AX, CX
	ADOXQ BX, R8
	ADCQ  $0x00, R8

	// | 

/* 			                                   */

	// | 
	// | W right
	// | 0   (SP)      | 1   8(SP)     | 2   16(SP)    | 3   24(SP)    | 4   32(SP)    | 5   40(SP)    | 6   48(SP)    | 7   56(SP)    | 8   64(SP)    | 9   72(SP)    | 10  80(SP)    | 11  88(SP)    
	// | 12  R9        | 13  R10       | 14  R11       | 15  R12       | 16  R13       | 17  R14       | 18  R15       | 19  CX        | 20  R8        | 21  -         | 22  -         | 23  -         


	MOVQ R9, 96(SP)
	MOVQ R10, 104(SP)
	MOVQ R11, 112(SP)
	MOVQ R12, 120(SP)
	MOVQ R13, 128(SP)
	MOVQ R14, 136(SP)
	MOVQ R15, 144(SP)
	MOVQ CX, 152(SP)
	MOVQ R8, 160(SP)

	// | 
	// | W right at stack
	// | 0   (SP)      | 1   8(SP)     | 2   16(SP)    | 3   24(SP)    | 4   32(SP)    | 5   40(SP)    | 6   48(SP)    | 7   56(SP)    | 8   64(SP)    | 9   72(SP)    | 10  80(SP)    | 11  88(SP)    
	// | 12  96(SP)    | 13  104(SP)   | 14  112(SP)   | 15  120(SP)   | 16  128(SP)   | 17  136(SP)   | 18  144(SP)   | 19  152(SP)   | 20  160(SP)   | 21  -         | 22  -         | 23  -         


	XORQ AX, AX

	// | 

/* i = 0                                   */

	// | a0 @ DX
	MOVQ (DI), DX

	// | a0 * b9 
	MULXQ 72(SI), AX, CX
	MOVQ  AX, 168(SP)

	// | a0 * b10 
	MULXQ 80(SI), AX, R8
	ADCXQ AX, CX

	// | a0 * b11 
	MULXQ 88(SI), AX, R9
	ADCXQ AX, R8
	ADCQ  $0x00, R9

	// | 

/* i = 1                                   */

	// | a1 @ DX
	MOVQ 8(DI), DX
	XORQ R10, R10

	// | a1 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8
	MOVQ  CX, 176(SP)

	// | a1 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a1 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, R9
	ADOXQ R10, R10
	ADCXQ BX, R10

	// | 

/* i = 2                                   */

	// | a2 @ DX
	MOVQ 16(DI), DX
	XORQ R11, R11

	// | a2 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9
	MOVQ  R8, 184(SP)

	// | a2 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a2 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, R10
	ADOXQ R11, R11
	ADCXQ BX, R11

	// | 

/* i = 3                                   */

	// | a3 @ DX
	MOVQ 24(DI), DX
	XORQ R12, R12

	// | a3 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10
	MOVQ  R9, 192(SP)

	// | a3 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11

	// | a3 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, R11
	ADOXQ R12, R12
	ADCXQ BX, R12

	// | 

/* i = 4                                   */

	// | a4 @ DX
	MOVQ 32(DI), DX
	XORQ R13, R13

	// | a4 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, R10
	ADCXQ BX, R11
	MOVQ  R10, 200(SP)

	// | a4 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a4 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, R12
	ADOXQ R13, R13
	ADCXQ BX, R13

	// | 

/* i = 5                                   */

	// | a5 @ DX
	MOVQ 40(DI), DX
	XORQ R14, R14

	// | a5 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, R11
	ADCXQ BX, R12

	// | a5 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a5 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, R13
	ADOXQ R14, R14
	ADCXQ BX, R14

	// | 

/* i = 6                                   */

	// | a6 @ DX
	MOVQ 48(DI), DX
	XORQ R15, R15

	// | a6 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, R12
	ADCXQ BX, R13

	// | a6 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a6 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, R14
	ADOXQ R15, R15
	ADCXQ BX, R15

	// | 

/* i = 7                                   */

	// | a7 @ DX
	MOVQ 56(DI), DX
	XORQ CX, CX

	// | a7 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, R13
	ADCXQ BX, R14

	// | a7 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a7 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, R15
	ADOXQ CX, CX
	ADCXQ BX, CX

	// | 

/* i = 8                                   */

	// | a8 @ DX
	MOVQ 64(DI), DX
	XORQ R8, R8

	// | a8 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, R14
	ADCXQ BX, R15

	// | a8 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX

	// | a8 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, CX
	ADOXQ R8, R8
	ADCXQ BX, R8

	// | 

/* i = 9                                   */

	// | a9 @ DX
	MOVQ 72(DI), DX
	XORQ R9, R9

	// | a9 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, R15
	ADCXQ BX, CX

	// | a9 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8

	// | a9 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, R8
	ADOXQ R9, R9
	ADCXQ BX, R9

	// | 

/* i = 10                                  */

	// | a10 @ DX
	MOVQ 80(DI), DX
	XORQ R10, R10

	// | a10 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, CX
	ADCXQ BX, R8

	// | a10 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a10 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, R9
	ADOXQ R10, R10
	ADCXQ BX, R10

	// | 

/* i = 11                                  */

	// | a11 @ DX
	MOVQ 88(DI), DX
	XORQ DI, DI

	// | a11 * b9 
	MULXQ 72(SI), AX, BX
	ADOXQ AX, R8
	ADCXQ BX, R9

	// | a11 * b10 
	MULXQ 80(SI), AX, BX
	ADOXQ AX, R9
	ADCXQ BX, R10

	// | a11 * b11 
	MULXQ 88(SI), AX, BX
	ADOXQ AX, R10
	ADOXQ BX, DI
	ADCQ  $0x00, DI

	// | 

/* 			                                    */

	// | 
	// | W left
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   168(SP)   | 10  176(SP)   | 11  184(SP)   
	// | 12  192(SP)   | 13  200(SP)   | 14  R11       | 15  R12       | 16  R13       | 17  R14       | 18  R15       | 19  CX        | 20  R8        | 21  R9        | 22  R10       | 23  DI        


	// | 
	// | W right
	// | 0   (SP)      | 1   8(SP)     | 2   16(SP)    | 3   24(SP)    | 4   32(SP)    | 5   40(SP)    | 6   48(SP)    | 7   56(SP)    | 8   64(SP)    | 9   72(SP)    | 10  80(SP)    | 11  88(SP)    
	// | 12  96(SP)    | 13  104(SP)   | 14  112(SP)   | 15  120(SP)   | 16  128(SP)   | 17  136(SP)   | 18  144(SP)   | 19  152(SP)   | 20  160(SP)   | 21  -         | 22  -         | 23  -         


	MOVQ 72(SP), AX
	ADDQ AX, 168(SP)
	MOVQ 80(SP), AX
	ADCQ AX, 176(SP)
	MOVQ 88(SP), AX
	ADCQ AX, 184(SP)
	MOVQ 96(SP), AX
	ADCQ AX, 192(SP)
	MOVQ 104(SP), AX
	ADCQ AX, 200(SP)
	ADCQ 112(SP), R11
	ADCQ 120(SP), R12
	ADCQ 128(SP), R13
	ADCQ 136(SP), R14
	ADCQ 144(SP), R15
	ADCQ 152(SP), CX
	ADCQ 160(SP), R8
	ADCQ $0x00, R9
	ADCQ $0x00, R10
	ADCQ $0x00, DI

	// | 
	// | W combined
	// | 0   (SP)      | 1   8(SP)     | 2   16(SP)    | 3   24(SP)    | 4   32(SP)    | 5   40(SP)    | 6   48(SP)    | 7   56(SP)    | 8   64(SP)    | 9   168(SP)   | 10  176(SP)   | 11  184(SP)   
	// | 12  192(SP)   | 13  200(SP)   | 14  R11       | 15  R12       | 16  R13       | 17  R14       | 18  R15       | 19  CX        | 20  R8        | 21  R9        | 22  R10       | 23  DI        


	MOVQ (SP), BX
	MOVQ 8(SP), SI
	MOVQ DI, (SP)
	MOVQ 16(SP), DI
	MOVQ R10, 8(SP)
	MOVQ 24(SP), R10
	MOVQ R9, 16(SP)
	MOVQ 32(SP), R9
	MOVQ R8, 24(SP)
	MOVQ 40(SP), R8
	MOVQ CX, 32(SP)
	MOVQ 48(SP), CX
	MOVQ R15, 40(SP)
	MOVQ 56(SP), R15
	MOVQ R14, 48(SP)
	MOVQ 64(SP), R14
	MOVQ R13, 56(SP)
	MOVQ 168(SP), R13
	MOVQ R12, 64(SP)
	MOVQ 176(SP), R12
	MOVQ R11, 72(SP)

	// | 
	// | W ready to mont
	// | 0   BX        | 1   SI        | 2   DI        | 3   R10       | 4   R9        | 5   R8        | 6   CX        | 7   R15       | 8   R14       | 9   R13       | 10  R12       | 11  184(SP)   
	// | 12  192(SP)   | 13  200(SP)   | 14  72(SP)    | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | 

/* montgomery reduction q1                 */

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 0                                   */

	// | 
	// | W
	// | 0   BX        | 1   SI        | 2   DI        | 3   R10       | 4   R9        | 5   R8        | 6   CX        | 7   R15       | 8   R14       | 9   R13       | 10  R12       | 11  184(SP)   
	// | 12  192(SP)   | 13  200(SP)   | 14  72(SP)    | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u0 = w0 * inp
	MOVQ  BX, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u0
	MOVQ DX, 80(SP)

	// | 

/*                                         */

	// | j0

	// | w0 @ BX
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, BX
	ADCXQ R11, SI

	// | j1

	// | w1 @ SI
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI

	// | j2

	// | w2 @ DI
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j3

	// | w3 @ R10
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j4

	// | w4 @ R9
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j5

	// | w5 @ R8
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j6

	// | w6 @ CX
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j7

	// | w7 @ R15
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j8

	// | w8 @ R14
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, R13

	// | j9

	// | w9 @ R13
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, R12
	ADOXQ BX, R12
	ADCXQ BX, BX
	MOVQ  $0x00, AX
	ADOXQ AX, BX

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 1                                   */

	// | 
	// | W
	// | 0   -         | 1   SI        | 2   DI        | 3   R10       | 4   R9        | 5   R8        | 6   CX        | 7   R15       | 8   R14       | 9   R13       | 10  R12       | 11  184(SP)   
	// | 12  192(SP)   | 13  200(SP)   | 14  72(SP)    | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u1 = w1 * inp
	MOVQ  SI, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u1
	MOVQ DX, 88(SP)

	// | 

/*                                         */

	// | j0

	// | w1 @ SI
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI

	// | j1

	// | w2 @ DI
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j2

	// | w3 @ R10
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j3

	// | w4 @ R9
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j4

	// | w5 @ R8
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j5

	// | w6 @ CX
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j6

	// | w7 @ R15
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j7

	// | w8 @ R14
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, R13

	// | j8

	// | w9 @ R13
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, R12

	// | j9

	// | w10 @ R12
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, R12

	// | w11 @ 184(SP)
	// | move to temp register
	MOVQ  184(SP), AX
	ADCXQ R11, AX
	ADOXQ BX, AX

	// | move to an idle register
	// | w11 @ AX
	MOVQ  AX, BX
	ADCXQ SI, SI
	MOVQ  $0x00, AX
	ADOXQ AX, SI

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 2                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   DI        | 3   R10       | 4   R9        | 5   R8        | 6   CX        | 7   R15       | 8   R14       | 9   R13       | 10  R12       | 11  BX        
	// | 12  192(SP)   | 13  200(SP)   | 14  72(SP)    | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u2 = w2 * inp
	MOVQ  DI, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u2
	MOVQ DX, 96(SP)

	// | 

/*                                         */

	// | j0

	// | w2 @ DI
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j1

	// | w3 @ R10
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j2

	// | w4 @ R9
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j3

	// | w5 @ R8
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j4

	// | w6 @ CX
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j5

	// | w7 @ R15
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j6

	// | w8 @ R14
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, R13

	// | j7

	// | w9 @ R13
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, R12

	// | j8

	// | w10 @ R12
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, BX

	// | j9

	// | w11 @ BX
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, BX

	// | w12 @ 192(SP)
	// | move to temp register
	MOVQ  192(SP), AX
	ADCXQ R11, AX
	ADOXQ SI, AX

	// | move to an idle register
	// | w12 @ AX
	MOVQ  AX, SI
	ADCXQ DI, DI
	MOVQ  $0x00, AX
	ADOXQ AX, DI

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 3                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   R10       | 4   R9        | 5   R8        | 6   CX        | 7   R15       | 8   R14       | 9   R13       | 10  R12       | 11  BX        
	// | 12  SI        | 13  200(SP)   | 14  72(SP)    | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u3 = w3 * inp
	MOVQ  R10, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u3
	MOVQ DX, 104(SP)

	// | 

/*                                         */

	// | j0

	// | w3 @ R10
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j1

	// | w4 @ R9
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j2

	// | w5 @ R8
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j3

	// | w6 @ CX
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j4

	// | w7 @ R15
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j5

	// | w8 @ R14
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, R13

	// | j6

	// | w9 @ R13
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, R12

	// | j7

	// | w10 @ R12
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, BX

	// | j8

	// | w11 @ BX
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, BX
	ADCXQ R11, SI

	// | j9

	// | w12 @ SI
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, SI

	// | w13 @ 200(SP)
	// | move to temp register
	MOVQ  200(SP), AX
	ADCXQ R11, AX
	ADOXQ DI, AX

	// | move to an idle register
	// | w13 @ AX
	MOVQ  AX, DI
	ADCXQ R10, R10
	MOVQ  $0x00, AX
	ADOXQ AX, R10

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 4                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   R9        | 5   R8        | 6   CX        | 7   R15       | 8   R14       | 9   R13       | 10  R12       | 11  BX        
	// | 12  SI        | 13  DI        | 14  72(SP)    | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u4 = w4 * inp
	MOVQ  R9, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u4
	MOVQ DX, 112(SP)

	// | 

/*                                         */

	// | j0

	// | w4 @ R9
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j1

	// | w5 @ R8
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j2

	// | w6 @ CX
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j3

	// | w7 @ R15
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j4

	// | w8 @ R14
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, R13

	// | j5

	// | w9 @ R13
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, R12

	// | j6

	// | w10 @ R12
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, BX

	// | j7

	// | w11 @ BX
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, BX
	ADCXQ R11, SI

	// | j8

	// | w12 @ SI
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI

	// | j9

	// | w13 @ DI
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, DI

	// | w14 @ 72(SP)
	// | move to temp register
	MOVQ  72(SP), AX
	ADCXQ R11, AX
	ADOXQ R10, AX

	// | move to an idle register
	// | w14 @ AX
	MOVQ  AX, R10
	ADCXQ R9, R9
	MOVQ  $0x00, AX
	ADOXQ AX, R9

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 5                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   R8        | 6   CX        | 7   R15       | 8   R14       | 9   R13       | 10  R12       | 11  BX        
	// | 12  SI        | 13  DI        | 14  R10       | 15  64(SP)    | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u5 = w5 * inp
	MOVQ  R8, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u5
	MOVQ DX, 72(SP)

	// | 

/*                                         */

	// | j0

	// | w5 @ R8
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j1

	// | w6 @ CX
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j2

	// | w7 @ R15
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j3

	// | w8 @ R14
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, R13

	// | j4

	// | w9 @ R13
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, R12

	// | j5

	// | w10 @ R12
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, BX

	// | j6

	// | w11 @ BX
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, BX
	ADCXQ R11, SI

	// | j7

	// | w12 @ SI
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI

	// | j8

	// | w13 @ DI
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j9

	// | w14 @ R10
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, R10

	// | w15 @ 64(SP)
	// | move to temp register
	MOVQ  64(SP), AX
	ADCXQ R11, AX
	ADOXQ R9, AX

	// | move to an idle register
	// | w15 @ AX
	MOVQ  AX, R9
	ADCXQ R8, R8
	MOVQ  $0x00, AX
	ADOXQ AX, R8

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 6                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   CX        | 7   R15       | 8   R14       | 9   R13       | 10  R12       | 11  BX        
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  56(SP)    | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u6 = w6 * inp
	MOVQ  CX, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u6
	MOVQ DX, 64(SP)

	// | 

/*                                         */

	// | j0

	// | w6 @ CX
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j1

	// | w7 @ R15
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j2

	// | w8 @ R14
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, R13

	// | j3

	// | w9 @ R13
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, R12

	// | j4

	// | w10 @ R12
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, BX

	// | j5

	// | w11 @ BX
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, BX
	ADCXQ R11, SI

	// | j6

	// | w12 @ SI
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI

	// | j7

	// | w13 @ DI
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j8

	// | w14 @ R10
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j9

	// | w15 @ R9
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, R9

	// | w16 @ 56(SP)
	// | move to temp register
	MOVQ  56(SP), AX
	ADCXQ R11, AX
	ADOXQ R8, AX

	// | move to an idle register
	// | w16 @ AX
	MOVQ  AX, R8
	ADCXQ CX, CX
	MOVQ  $0x00, AX
	ADOXQ AX, CX

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 7                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   R15       | 8   R14       | 9   R13       | 10  R12       | 11  BX        
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  48(SP)    | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u7 = w7 * inp
	MOVQ  R15, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u7
	MOVQ DX, 56(SP)

	// | 

/*                                         */

	// | j0

	// | w7 @ R15
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j1

	// | w8 @ R14
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, R13

	// | j2

	// | w9 @ R13
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, R12

	// | j3

	// | w10 @ R12
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, BX

	// | j4

	// | w11 @ BX
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, BX
	ADCXQ R11, SI

	// | j5

	// | w12 @ SI
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI

	// | j6

	// | w13 @ DI
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j7

	// | w14 @ R10
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j8

	// | w15 @ R9
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j9

	// | w16 @ R8
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, R8

	// | w17 @ 48(SP)
	// | move to temp register
	MOVQ  48(SP), AX
	ADCXQ R11, AX
	ADOXQ CX, AX

	// | move to an idle register
	// | w17 @ AX
	MOVQ  AX, CX
	ADCXQ R15, R15
	MOVQ  $0x00, AX
	ADOXQ AX, R15

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 8                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   R14       | 9   R13       | 10  R12       | 11  BX        
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  40(SP)    | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u8 = w8 * inp
	MOVQ  R14, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u8
	MOVQ DX, 48(SP)

	// | 

/*                                         */

	// | j0

	// | w8 @ R14
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, R13

	// | j1

	// | w9 @ R13
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, R12

	// | j2

	// | w10 @ R12
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, BX

	// | j3

	// | w11 @ BX
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, BX
	ADCXQ R11, SI

	// | j4

	// | w12 @ SI
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI

	// | j5

	// | w13 @ DI
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j6

	// | w14 @ R10
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j7

	// | w15 @ R9
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j8

	// | w16 @ R8
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j9

	// | w17 @ CX
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, CX

	// | w18 @ 40(SP)
	// | move to temp register
	MOVQ  40(SP), AX
	ADCXQ R11, AX
	ADOXQ R15, AX

	// | move to an idle register
	// | w18 @ AX
	MOVQ  AX, R15
	ADCXQ R14, R14
	MOVQ  $0x00, AX
	ADOXQ AX, R14

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 9                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   R13       | 10  R12       | 11  BX        
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  32(SP)    | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u9 = w9 * inp
	MOVQ  R13, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u9
	MOVQ DX, 40(SP)

	// | 

/*                                         */

	// | j0

	// | w9 @ R13
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, R12

	// | j1

	// | w10 @ R12
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, BX

	// | j2

	// | w11 @ BX
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, BX
	ADCXQ R11, SI

	// | j3

	// | w12 @ SI
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI

	// | j4

	// | w13 @ DI
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j5

	// | w14 @ R10
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j6

	// | w15 @ R9
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j7

	// | w16 @ R8
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j8

	// | w17 @ CX
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j9

	// | w18 @ R15
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, R15

	// | w19 @ 32(SP)
	// | move to temp register
	MOVQ  32(SP), AX
	ADCXQ R11, AX
	ADOXQ R14, AX

	// | move to an idle register
	// | w19 @ AX
	MOVQ  AX, R14
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | 
	// | W montgomery reduction q1 ends
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  BX        
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | carry from q1 should be added to w20
	MOVQ R13, 32(SP)

	// | 

/* montgomerry reduction q2                */

	// | clear flags
	XORQ R13, R13

	// | 

/* i = 0                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  BX        
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | u0 @ 80(SP)
	MOVQ 80(SP), DX

	// | 

/*                                         */

	// | j10

	// | w10 @ R12
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, BX

	// | j11

	// | w11 @ BX
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, BX
	ADCXQ R11, SI
	ADOXQ R13, SI
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 1                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  BX        
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | u1 @ 88(SP)
	MOVQ 88(SP), DX

	// | 

/*                                         */

	// | j10

	// | w11 @ BX
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, BX
	MOVQ  BX, 80(SP)
	ADCXQ R11, SI

	// | j11

	// | w12 @ SI
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI
	ADOXQ R13, DI
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 2                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  80(SP)    
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | u2 @ 96(SP)
	MOVQ 96(SP), DX

	// | 

/*                                         */

	// | j10

	// | w12 @ SI
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, SI
	MOVQ  SI, 88(SP)
	ADCXQ R11, DI

	// | j11

	// | w13 @ DI
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10
	ADOXQ R13, R10
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 3                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  80(SP)    
	// | 12  88(SP)    | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | u3 @ 104(SP)
	MOVQ 104(SP), DX

	// | 

/*                                         */

	// | j10

	// | w13 @ DI
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j11

	// | w14 @ R10
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9
	ADOXQ R13, R9
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 4                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  80(SP)    
	// | 12  88(SP)    | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | u4 @ 112(SP)
	MOVQ 112(SP), DX

	// | 

/*                                         */

	// | j10

	// | w14 @ R10
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j11

	// | w15 @ R9
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8
	ADOXQ R13, R8
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 5                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  80(SP)    
	// | 12  88(SP)    | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | u5 @ 72(SP)
	MOVQ 72(SP), DX

	// | 

/*                                         */

	// | j10

	// | w15 @ R9
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j11

	// | w16 @ R8
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX
	ADOXQ R13, CX
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 6                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  80(SP)    
	// | 12  88(SP)    | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | u6 @ 64(SP)
	MOVQ 64(SP), DX

	// | 

/*                                         */

	// | j10

	// | w16 @ R8
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j11

	// | w17 @ CX
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15
	ADOXQ R13, R15
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 7                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  80(SP)    
	// | 12  88(SP)    | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | u7 @ 56(SP)
	MOVQ 56(SP), DX

	// | 

/*                                         */

	// | j10

	// | w17 @ CX
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j11

	// | w18 @ R15
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14
	ADOXQ R13, R14

	// | bring the carry from q1
	MOVQ  32(SP), R13
	MOVQ  $0x00, AX
	ADCXQ AX, R13
	ADOXQ AX, R13

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 8                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  80(SP)    
	// | 12  88(SP)    | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  24(SP)    | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | u8 @ 48(SP)
	MOVQ 48(SP), DX

	// | 

/*                                         */

	// | j10

	// | w18 @ R15
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j11

	// | w19 @ R14
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, R14

	// | w20 @ 24(SP)
	// | move to an idle register
	MOVQ 24(SP), BX

	// | w20 @ BX
	ADCXQ R11, BX
	ADOXQ R13, BX
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 9                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  80(SP)    
	// | 12  88(SP)    | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  BX        | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | u9 @ 40(SP)
	MOVQ 40(SP), DX

	// | 

/*                                         */

	// | j10

	// | w19 @ R14
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, BX

	// | j11

	// | w20 @ BX
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, BX

	// | w21 @ 16(SP)
	// | move to an idle register
	MOVQ 16(SP), SI

	// | w21 @ SI
	ADCXQ R11, SI
	ADOXQ R13, SI
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | 
	// | q2 ends
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  80(SP)    
	// | 12  88(SP)    | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  BX        | 21  SI        | 22  8(SP)     | 23  (SP)      


	// | save the carry from q2
	// | should be added to w22
	MOVQ R13, 32(SP)

	// | 

/* q2 q3 transition swap                   */

	MOVQ 80(SP), R13
	MOVQ SI, 16(SP)
	MOVQ 88(SP), SI

	// | 
	// | W q2 q3 transition
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  R13       
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  BX        | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | 

/* montgomery reduction q3                 */

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 10                                  */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  R12       | 11  R13       
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  BX        | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u10 = w10 * inp
	MOVQ  R12, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u10
	MOVQ DX, 24(SP)

	// | 

/*                                         */

	// | j0

	// | w10 @ R12
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, R13

	// | j1

	// | w11 @ R13
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, SI

	// | j2

	// | w12 @ SI
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI

	// | j3

	// | w13 @ DI
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j4

	// | w14 @ R10
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j5

	// | w15 @ R9
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j6

	// | w16 @ R8
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j7

	// | w17 @ CX
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j8

	// | w18 @ R15
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j9

	// | w19 @ R14
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, BX
	ADOXQ R12, BX
	ADCXQ R12, R12
	MOVQ  $0x00, AX
	ADOXQ AX, R12

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 11                                  */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  R13       
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  BX        | 21  16(SP)    | 22  8(SP)     | 23  (SP)      


	// | | u11 = w11 * inp
	MOVQ  R13, DX
	MULXQ ·inp+0(SB), DX, R11

	// | save u11
	MOVQ DX, 40(SP)

	// | 

/*                                         */

	// | j0

	// | w11 @ R13
	MULXQ ·modulus+0(SB), AX, R11
	ADOXQ AX, R13
	ADCXQ R11, SI

	// | j1

	// | w12 @ SI
	MULXQ ·modulus+8(SB), AX, R11
	ADOXQ AX, SI
	ADCXQ R11, DI

	// | j2

	// | w13 @ DI
	MULXQ ·modulus+16(SB), AX, R11
	ADOXQ AX, DI
	ADCXQ R11, R10

	// | j3

	// | w14 @ R10
	MULXQ ·modulus+24(SB), AX, R11
	ADOXQ AX, R10
	ADCXQ R11, R9

	// | j4

	// | w15 @ R9
	MULXQ ·modulus+32(SB), AX, R11
	ADOXQ AX, R9
	ADCXQ R11, R8

	// | j5

	// | w16 @ R8
	MULXQ ·modulus+40(SB), AX, R11
	ADOXQ AX, R8
	ADCXQ R11, CX

	// | j6

	// | w17 @ CX
	MULXQ ·modulus+48(SB), AX, R11
	ADOXQ AX, CX
	ADCXQ R11, R15

	// | j7

	// | w18 @ R15
	MULXQ ·modulus+56(SB), AX, R11
	ADOXQ AX, R15
	ADCXQ R11, R14

	// | j8

	// | w19 @ R14
	MULXQ ·modulus+64(SB), AX, R11
	ADOXQ AX, R14
	ADCXQ R11, BX

	// | j9

	// | w20 @ BX
	MULXQ ·modulus+72(SB), AX, R11
	ADOXQ AX, BX

	// | w21 @ 16(SP)
	// | move to temp register
	MOVQ  16(SP), AX
	ADCXQ R11, AX
	ADOXQ R12, AX

	// | move to an idle register
	// | w21 @ AX
	MOVQ  AX, R12
	ADCXQ R13, R13
	MOVQ  $0x00, AX
	ADOXQ AX, R13

	// | 
	// | W q3
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  -         
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  BX        | 21  R12       | 22  8(SP)     | 23  (SP)      


	// | aggregate carries from q2 & q3
	// | should be added to w22
	ADCQ 32(SP), R13

	// | 

/* montgomerry reduction q4                */

	// | clear flags
	XORQ AX, AX

	// | 

/* i = 0                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  -         
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  BX        | 21  R12       | 22  8(SP)     | 23  (SP)      


	// | u0 @ 24(SP)
	MOVQ 24(SP), DX

	// | 

/*                                         */

	// | j10

	// | w20 @ BX
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, BX
	ADCXQ R11, R12
	MOVQ  BX, 16(SP)

	// | j11

	// | w21 @ R12
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, R12

	// | w22 @ 8(SP)
	// | move to an idle register
	MOVQ  8(SP), BX
	ADCXQ R11, BX

	// | bring carry from q2 & q3
	// | w22 @ BX
	ADOXQ R13, BX
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, R11
	ADOXQ R11, R13

	// | 

/* i = 1                                   */

	// | 
	// | W
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  -         
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  16(SP)    | 21  R12       | 22  BX        | 23  (SP)      


	// | u1 @ 40(SP)
	MOVQ 40(SP), DX

	// | 

/*                                         */

	// | j10

	// | w21 @ R12
	MULXQ ·modulus+80(SB), AX, R11
	ADOXQ AX, R12
	ADCXQ R11, BX

	// | j11

	// | w22 @ BX
	MULXQ ·modulus+88(SB), AX, R11
	ADOXQ AX, BX

	// | w23 @ (SP)
	// | move to an idle register
	MOVQ  (SP), AX
	ADCXQ R11, AX

	// | w23 @ AX
	ADOXQ R13, AX
	MOVQ  $0x00, R13
	ADCXQ R13, R13
	MOVQ  $0x00, R11
	ADOXQ R11, R13

	// | 
	// | W q4
	// | 0   -         | 1   -         | 2   -         | 3   -         | 4   -         | 5   -         | 6   -         | 7   -         | 8   -         | 9   -         | 10  -         | 11  -         
	// | 12  SI        | 13  DI        | 14  R10       | 15  R9        | 16  R8        | 17  CX        | 18  R15       | 19  R14       | 20  16(SP)    | 21  R12       | 22  BX        | 23  AX        


	// | 

/* modular reduction                       */

	MOVQ SI, R11
	SUBQ ·modulus+0(SB), R11
	MOVQ DI, DX
	SBBQ ·modulus+8(SB), DX
	MOVQ DX, (SP)
	MOVQ R10, DX
	SBBQ ·modulus+16(SB), DX
	MOVQ DX, 8(SP)
	MOVQ R9, DX
	SBBQ ·modulus+24(SB), DX
	MOVQ DX, 24(SP)
	MOVQ R8, DX
	SBBQ ·modulus+32(SB), DX
	MOVQ DX, 32(SP)
	MOVQ CX, DX
	SBBQ ·modulus+40(SB), DX
	MOVQ DX, 40(SP)
	MOVQ R15, DX
	SBBQ ·modulus+48(SB), DX
	MOVQ DX, 48(SP)
	MOVQ R14, DX
	SBBQ ·modulus+56(SB), DX
	MOVQ DX, 56(SP)
	MOVQ 16(SP), DX
	SBBQ ·modulus+64(SB), DX
	MOVQ DX, 64(SP)
	MOVQ R12, DX
	SBBQ ·modulus+72(SB), DX
	MOVQ DX, 72(SP)
	MOVQ BX, DX
	SBBQ ·modulus+80(SB), DX
	MOVQ DX, 80(SP)
	MOVQ AX, DX
	SBBQ ·modulus+88(SB), DX
	MOVQ DX, 88(SP)
	SBBQ $0x00, R13

	// | 

/* out                                     */

	MOVQ    c+0(FP), R13
	CMOVQCC R11, SI
	MOVQ    SI, (R13)
	CMOVQCC (SP), DI
	MOVQ    DI, 8(R13)
	CMOVQCC 8(SP), R10
	MOVQ    R10, 16(R13)
	CMOVQCC 24(SP), R9
	MOVQ    R9, 24(R13)
	CMOVQCC 32(SP), R8
	MOVQ    R8, 32(R13)
	CMOVQCC 40(SP), CX
	MOVQ    CX, 40(R13)
	CMOVQCC 48(SP), R15
	MOVQ    R15, 48(R13)
	CMOVQCC 56(SP), R14
	MOVQ    R14, 56(R13)
	MOVQ    16(SP), DX
	CMOVQCC 64(SP), DX
	MOVQ    DX, 64(R13)
	CMOVQCC 72(SP), R12
	MOVQ    R12, 72(R13)
	CMOVQCC 80(SP), BX
	MOVQ    BX, 80(R13)
	CMOVQCC 88(SP), AX
	MOVQ    AX, 88(R13)
	RET

	// | 

/* end                                     */


