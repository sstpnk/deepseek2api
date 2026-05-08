// +build amd64,!purego

#include "textflag.h"

// vpternlogProbeD2: imm8=0xD2 computes a ^ (~b & c)
TEXT ·vpternlogProbeD2(SB), NOSPLIT, $0-32
    MOVQ a+0(FP), AX; VMOVQ AX, X0
    MOVQ b+8(FP), BX; VMOVQ BX, X1
    MOVQ c+16(FP), CX; VMOVQ CX, X2
    VPTERNLOGQ $0xD2, Z2, Z1, Z0
    VMOVQ X0, AX; MOVQ AX, ret+24(FP)
    RET
