// +build amd64,!purego

#include "textflag.h"

// Probe 0: imm8=0x00 → should return 0 regardless of inputs
TEXT ·vpternlogProbe00(SB), NOSPLIT, $0-32
    MOVQ a+0(FP), AX; VMOVQ AX, X0
    MOVQ b+8(FP), BX; VMOVQ BX, X1
    MOVQ c+16(FP), CX; VMOVQ CX, X2
    VPTERNLOGQ $0x00, Z2, Z1, Z0
    VMOVQ X0, AX; MOVQ AX, ret+24(FP)
    RET

// Probe FF: imm8=0xFF → should return all 1s regardless of inputs
TEXT ·vpternlogProbeFF(SB), NOSPLIT, $0-32
    MOVQ a+0(FP), AX; VMOVQ AX, X0
    MOVQ b+8(FP), BX; VMOVQ BX, X1
    MOVQ c+16(FP), CX; VMOVQ CX, X2
    VPTERNLOGQ $0xFF, Z2, Z1, Z0
    VMOVQ X0, AX; MOVQ AX, ret+24(FP)
    RET

// Probe F0: imm8=0xF0 → should return c
TEXT ·vpternlogProbeF0(SB), NOSPLIT, $0-32
    MOVQ a+0(FP), AX; VMOVQ AX, X0
    MOVQ b+8(FP), BX; VMOVQ BX, X1
    MOVQ c+16(FP), CX; VMOVQ CX, X2
    VPTERNLOGQ $0xF0, Z2, Z1, Z0
    VMOVQ X0, AX; MOVQ AX, ret+24(FP)
    RET

// Probe 96: imm8=0x96 → f(a,b,c)=a^(b&c) → easy to verify
TEXT ·vpternlogProbe96(SB), NOSPLIT, $0-32
    MOVQ a+0(FP), AX; VMOVQ AX, X0
    MOVQ b+8(FP), BX; VMOVQ BX, X1
    MOVQ c+16(FP), CX; VMOVQ CX, X2
    VPTERNLOGQ $0x96, Z2, Z1, Z0
    VMOVQ X0, AX; MOVQ AX, ret+24(FP)
    RET

// Probe CC: imm8=0xCC → f(a,b,c)=a?b:c → if a then b else c
TEXT ·vpternlogProbeCC(SB), NOSPLIT, $0-32
    MOVQ a+0(FP), AX; VMOVQ AX, X0
    MOVQ b+8(FP), BX; VMOVQ BX, X1
    MOVQ c+16(FP), CX; VMOVQ CX, X2
    VPTERNLOGQ $0xCC, Z2, Z1, Z0
    VMOVQ X0, AX; MOVQ AX, ret+24(FP)
    RET
