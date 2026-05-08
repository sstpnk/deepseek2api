// +build amd64,!purego

#include "textflag.h"

// func vpternlogProbe(a, b, c uint64) uint64
// Computes a ^ (~b & c) using VPTERNLOGQ with imm8=$0xA4.
// Returns the result so Go can verify against scalar computation.
TEXT ·vpternlogProbe(SB), NOSPLIT, $0-32
    MOVQ a+0(FP), AX
    MOVQ b+8(FP), BX
    MOVQ c+16(FP), CX
    VMOVQ AX, X0
    VMOVQ BX, X1
    VMOVQ CX, X2
    VPTERNLOGQ $0xA4, Z2, Z1, Z0
    VMOVQ X0, AX
    MOVQ AX, ret+24(FP)
    RET
