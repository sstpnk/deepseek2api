// +build amd64,!purego

#include "textflag.h"

// keccakF23AVX512 — AVX-512 Keccak-f[1600] rounds 1..23 (DeepSeekHashV1).
//
// Implementation notes:
//   - Theta (θ): scalar XOR chain + ROLQ $1, results in GP registers.
//   - Rho (ρ) + Pi (π): 25 scalar lane rotations with hardcoded offsets,
//     stored to b[] scratch at 200(SP).
//   - Chi (χ): VPTERNLOGQ $0xA4 on ZMM registers → 1 instruction per lane
//     instead of 3 scalar (ANDN + XOR). Data loaded via XMM (SSE2 MOVQ)
//     into shared XMM/ZMM physical registers.
//   - Iota (ι): XOR round constant from precomputed table.
//   - DeepSeekHashV1 skips standard Keccak round 0 — only rounds 1..23 run.
//   - 23-round loop with INCQ/CMPQ/JL; loop overhead <0.1% of per-round work.
//
// Registers (callee-saved saved/restored):
//   R12(d4), R13(d3), BP(RC table base)
// Stack frame: 424 bytes (200 state + 200 b[] + 24 saved regs)

// RC constants for rounds 1..23.
DATA rc<>+0(SB)/8, $0x0000000000008082
DATA rc<>+8(SB)/8, $0x800000000000808A
DATA rc<>+16(SB)/8, $0x8000000080008000
DATA rc<>+24(SB)/8, $0x000000000000808B
DATA rc<>+32(SB)/8, $0x0000000080000001
DATA rc<>+40(SB)/8, $0x8000000080008081
DATA rc<>+48(SB)/8, $0x8000000000008009
DATA rc<>+56(SB)/8, $0x000000000000008A
DATA rc<>+64(SB)/8, $0x0000000000000088
DATA rc<>+72(SB)/8, $0x0000000080008009
DATA rc<>+80(SB)/8, $0x000000008000000A
DATA rc<>+88(SB)/8, $0x000000008000808B
DATA rc<>+96(SB)/8, $0x800000000000008B
DATA rc<>+104(SB)/8, $0x8000000000008089
DATA rc<>+112(SB)/8, $0x8000000000008003
DATA rc<>+120(SB)/8, $0x8000000000008002
DATA rc<>+128(SB)/8, $0x8000000000000080
DATA rc<>+136(SB)/8, $0x000000000000800A
DATA rc<>+144(SB)/8, $0x800000008000000A
DATA rc<>+152(SB)/8, $0x8000000080008081
DATA rc<>+160(SB)/8, $0x8000000000008080
DATA rc<>+168(SB)/8, $0x0000000080000001
DATA rc<>+176(SB)/8, $0x8000000080008008
GLOBL rc<>(SB), RODATA, $184

// func keccakF23AVX512(s *[25]uint64)
//
// AVX-512 Keccak-f[1600] rounds 1..23.
// Chi step uses VPTERNLOGQ (1 instruction vs 3 scalar).
// Theta and Rho+Pi remain scalar; state on stack.
//
// Stack: 0(SP)..192(SP)=state, 200(SP)..392(SP)=b[] scratch,
// 400(SP)..416(SP)=saved callee-saved regs (R12,R13,BP)
TEXT ·keccakF23AVX512(SB), $424-8
    // Save callee-saved registers (Go amd64 ABI: BP, R12-R15)
    MOVQ R12, 400(SP)
    MOVQ R13, 408(SP)
    MOVQ BP, 416(SP)

    MOVQ s+0(FP), DI

    // Copy state to stack.
    MOVQ 0(DI), AX; MOVQ AX, 0(SP)
    MOVQ 8(DI), AX; MOVQ AX, 8(SP)
    MOVQ 16(DI), AX; MOVQ AX, 16(SP)
    MOVQ 24(DI), AX; MOVQ AX, 24(SP)
    MOVQ 32(DI), AX; MOVQ AX, 32(SP)
    MOVQ 40(DI), AX; MOVQ AX, 40(SP)
    MOVQ 48(DI), AX; MOVQ AX, 48(SP)
    MOVQ 56(DI), AX; MOVQ AX, 56(SP)
    MOVQ 64(DI), AX; MOVQ AX, 64(SP)
    MOVQ 72(DI), AX; MOVQ AX, 72(SP)
    MOVQ 80(DI), AX; MOVQ AX, 80(SP)
    MOVQ 88(DI), AX; MOVQ AX, 88(SP)
    MOVQ 96(DI), AX; MOVQ AX, 96(SP)
    MOVQ 104(DI), AX; MOVQ AX, 104(SP)
    MOVQ 112(DI), AX; MOVQ AX, 112(SP)
    MOVQ 120(DI), AX; MOVQ AX, 120(SP)
    MOVQ 128(DI), AX; MOVQ AX, 128(SP)
    MOVQ 136(DI), AX; MOVQ AX, 136(SP)
    MOVQ 144(DI), AX; MOVQ AX, 144(SP)
    MOVQ 152(DI), AX; MOVQ AX, 152(SP)
    MOVQ 160(DI), AX; MOVQ AX, 160(SP)
    MOVQ 168(DI), AX; MOVQ AX, 168(SP)
    MOVQ 176(DI), AX; MOVQ AX, 176(SP)
    MOVQ 184(DI), AX; MOVQ AX, 184(SP)
    MOVQ 192(DI), AX; MOVQ AX, 192(SP)

    XORQ SI, SI          // round counter
    LEAQ rc<>(SB), BP    // RC table

round:
    // === Theta (scalar) ===
    MOVQ 0(SP), R8; MOVQ 40(SP), AX; XORQ AX, R8
    MOVQ 80(SP), AX; XORQ AX, R8
    MOVQ 120(SP), AX; XORQ AX, R8
    MOVQ 160(SP), AX; XORQ AX, R8

    MOVQ 8(SP), R9; MOVQ 48(SP), AX; XORQ AX, R9
    MOVQ 88(SP), AX; XORQ AX, R9
    MOVQ 128(SP), AX; XORQ AX, R9
    MOVQ 168(SP), AX; XORQ AX, R9

    MOVQ 16(SP), R10; MOVQ 56(SP), AX; XORQ AX, R10
    MOVQ 96(SP), AX; XORQ AX, R10
    MOVQ 136(SP), AX; XORQ AX, R10
    MOVQ 176(SP), AX; XORQ AX, R10

    MOVQ 24(SP), R11; MOVQ 64(SP), AX; XORQ AX, R11
    MOVQ 104(SP), AX; XORQ AX, R11
    MOVQ 144(SP), AX; XORQ AX, R11
    MOVQ 184(SP), AX; XORQ AX, R11

    MOVQ 32(SP), R12; MOVQ 72(SP), AX; XORQ AX, R12
    MOVQ 112(SP), AX; XORQ AX, R12
    MOVQ 152(SP), AX; XORQ AX, R12
    MOVQ 192(SP), AX; XORQ AX, R12

    // d[x] = c[x-1] ^ ROTL64(c[x+1], 1)
    MOVQ R9, AX; ROLQ $1, AX; XORQ R12, AX; MOVQ AX, BX  // d0
    MOVQ R10, AX; ROLQ $1, AX; XORQ R8, AX; MOVQ AX, CX  // d1
    MOVQ R11, AX; ROLQ $1, AX; XORQ R9, AX; MOVQ AX, DX  // d2
    MOVQ R12, AX; ROLQ $1, AX; XORQ R10, AX; MOVQ AX, R13 // d3
    MOVQ R8, AX; ROLQ $1, AX; XORQ R11, AX; MOVQ AX, R12 // d4

    // Mix d into columns (d0=BX, d1=CX, d2=DX, d3=R13, d4=R12)
    MOVQ 0(SP), AX; XORQ BX, AX; MOVQ AX, 0(SP)
    MOVQ 40(SP), AX; XORQ BX, AX; MOVQ AX, 40(SP)
    MOVQ 80(SP), AX; XORQ BX, AX; MOVQ AX, 80(SP)
    MOVQ 120(SP), AX; XORQ BX, AX; MOVQ AX, 120(SP)
    MOVQ 160(SP), AX; XORQ BX, AX; MOVQ AX, 160(SP)

    MOVQ 8(SP), AX; XORQ CX, AX; MOVQ AX, 8(SP)
    MOVQ 48(SP), AX; XORQ CX, AX; MOVQ AX, 48(SP)
    MOVQ 88(SP), AX; XORQ CX, AX; MOVQ AX, 88(SP)
    MOVQ 128(SP), AX; XORQ CX, AX; MOVQ AX, 128(SP)
    MOVQ 168(SP), AX; XORQ CX, AX; MOVQ AX, 168(SP)

    MOVQ 16(SP), AX; XORQ DX, AX; MOVQ AX, 16(SP)
    MOVQ 56(SP), AX; XORQ DX, AX; MOVQ AX, 56(SP)
    MOVQ 96(SP), AX; XORQ DX, AX; MOVQ AX, 96(SP)
    MOVQ 136(SP), AX; XORQ DX, AX; MOVQ AX, 136(SP)
    MOVQ 176(SP), AX; XORQ DX, AX; MOVQ AX, 176(SP)

    MOVQ 24(SP), AX; XORQ R13, AX; MOVQ AX, 24(SP)
    MOVQ 64(SP), AX; XORQ R13, AX; MOVQ AX, 64(SP)
    MOVQ 104(SP), AX; XORQ R13, AX; MOVQ AX, 104(SP)
    MOVQ 144(SP), AX; XORQ R13, AX; MOVQ AX, 144(SP)
    MOVQ 184(SP), AX; XORQ R13, AX; MOVQ AX, 184(SP)

    MOVQ 32(SP), AX; XORQ R12, AX; MOVQ AX, 32(SP)
    MOVQ 72(SP), AX; XORQ R12, AX; MOVQ AX, 72(SP)
    MOVQ 112(SP), AX; XORQ R12, AX; MOVQ AX, 112(SP)
    MOVQ 152(SP), AX; XORQ R12, AX; MOVQ AX, 152(SP)
    MOVQ 192(SP), AX; XORQ R12, AX; MOVQ AX, 192(SP)

    // === Rho+Pi (scalar — same as amd64) ===
    MOVQ 0(SP), AX; MOVQ AX, 200(SP)
    MOVQ 8(SP), AX; ROLQ $1, AX; MOVQ AX, 280(SP)
    MOVQ 16(SP), AX; ROLQ $62, AX; MOVQ AX, 360(SP)
    MOVQ 24(SP), AX; ROLQ $28, AX; MOVQ AX, 240(SP)
    MOVQ 32(SP), AX; ROLQ $27, AX; MOVQ AX, 320(SP)
    MOVQ 40(SP), AX; ROLQ $36, AX; MOVQ AX, 328(SP)
    MOVQ 48(SP), AX; ROLQ $44, AX; MOVQ AX, 208(SP)
    MOVQ 56(SP), AX; ROLQ $6, AX; MOVQ AX, 288(SP)
    MOVQ 64(SP), AX; ROLQ $55, AX; MOVQ AX, 368(SP)
    MOVQ 72(SP), AX; ROLQ $20, AX; MOVQ AX, 248(SP)
    MOVQ 80(SP), AX; ROLQ $3, AX; MOVQ AX, 256(SP)
    MOVQ 88(SP), AX; ROLQ $10, AX; MOVQ AX, 336(SP)
    MOVQ 96(SP), AX; ROLQ $43, AX; MOVQ AX, 216(SP)
    MOVQ 104(SP), AX; ROLQ $25, AX; MOVQ AX, 296(SP)
    MOVQ 112(SP), AX; ROLQ $39, AX; MOVQ AX, 376(SP)
    MOVQ 120(SP), AX; ROLQ $41, AX; MOVQ AX, 384(SP)
    MOVQ 128(SP), AX; ROLQ $45, AX; MOVQ AX, 264(SP)
    MOVQ 136(SP), AX; ROLQ $15, AX; MOVQ AX, 344(SP)
    MOVQ 144(SP), AX; ROLQ $21, AX; MOVQ AX, 224(SP)
    MOVQ 152(SP), AX; ROLQ $8, AX; MOVQ AX, 304(SP)
    MOVQ 160(SP), AX; ROLQ $18, AX; MOVQ AX, 312(SP)
    MOVQ 168(SP), AX; ROLQ $2, AX; MOVQ AX, 392(SP)
    MOVQ 176(SP), AX; ROLQ $61, AX; MOVQ AX, 272(SP)
    MOVQ 184(SP), AX; ROLQ $56, AX; MOVQ AX, 352(SP)
    MOVQ 192(SP), AX; ROLQ $14, AX; MOVQ AX, 232(SP)

    // === Chi: a[x] = b[x] ^ ((~b[x+1]) & b[x+2]) with VPTERNLOGQ 0xD2 ===
    // Load via XMM (SSE2 MOVQ), compute via ZMM VPTERNLOGQ, store via XMM.
    // 0xD2 computes a^(~b&c) with Go asm's (dst,src1,src2) bit ordering.
    //
    // Row 0 (0,1,2,3,4)
    MOVQ 200(SP), X0; MOVQ 208(SP), X1; MOVQ 216(SP), X2
    VPTERNLOGQ $0xD2, Z2, Z1, Z0; MOVQ X0, 0(SP)         // a[0]
    MOVQ 224(SP), X3
    VPTERNLOGQ $0xD2, Z3, Z2, Z1; MOVQ X1, 8(SP)         // a[1]
    MOVQ 232(SP), X4
    VPTERNLOGQ $0xD2, Z4, Z3, Z2; MOVQ X2, 16(SP)        // a[2]
    MOVQ 200(SP), X0
    VPTERNLOGQ $0xD2, Z0, Z4, Z3; MOVQ X3, 24(SP)        // a[3]
    MOVQ 208(SP), X1
    VPTERNLOGQ $0xD2, Z1, Z0, Z4; MOVQ X4, 32(SP)        // a[4]

    // Row 1 (5,6,7,8,9)
    MOVQ 240(SP), X0; MOVQ 248(SP), X1; MOVQ 256(SP), X2
    VPTERNLOGQ $0xD2, Z2, Z1, Z0; MOVQ X0, 40(SP)
    MOVQ 264(SP), X3
    VPTERNLOGQ $0xD2, Z3, Z2, Z1; MOVQ X1, 48(SP)
    MOVQ 272(SP), X4
    VPTERNLOGQ $0xD2, Z4, Z3, Z2; MOVQ X2, 56(SP)
    MOVQ 240(SP), X0
    VPTERNLOGQ $0xD2, Z0, Z4, Z3; MOVQ X3, 64(SP)
    MOVQ 248(SP), X1
    VPTERNLOGQ $0xD2, Z1, Z0, Z4; MOVQ X4, 72(SP)

    // Row 2 (10,11,12,13,14)
    MOVQ 280(SP), X0; MOVQ 288(SP), X1; MOVQ 296(SP), X2
    VPTERNLOGQ $0xD2, Z2, Z1, Z0; MOVQ X0, 80(SP)
    MOVQ 304(SP), X3
    VPTERNLOGQ $0xD2, Z3, Z2, Z1; MOVQ X1, 88(SP)
    MOVQ 312(SP), X4
    VPTERNLOGQ $0xD2, Z4, Z3, Z2; MOVQ X2, 96(SP)
    MOVQ 280(SP), X0
    VPTERNLOGQ $0xD2, Z0, Z4, Z3; MOVQ X3, 104(SP)
    MOVQ 288(SP), X1
    VPTERNLOGQ $0xD2, Z1, Z0, Z4; MOVQ X4, 112(SP)

    // Row 3 (15,16,17,18,19)
    MOVQ 320(SP), X0; MOVQ 328(SP), X1; MOVQ 336(SP), X2
    VPTERNLOGQ $0xD2, Z2, Z1, Z0; MOVQ X0, 120(SP)
    MOVQ 344(SP), X3
    VPTERNLOGQ $0xD2, Z3, Z2, Z1; MOVQ X1, 128(SP)
    MOVQ 352(SP), X4
    VPTERNLOGQ $0xD2, Z4, Z3, Z2; MOVQ X2, 136(SP)
    MOVQ 320(SP), X0
    VPTERNLOGQ $0xD2, Z0, Z4, Z3; MOVQ X3, 144(SP)
    MOVQ 328(SP), X1
    VPTERNLOGQ $0xD2, Z1, Z0, Z4; MOVQ X4, 152(SP)

    // Row 4 (20,21,22,23,24)
    MOVQ 360(SP), X0; MOVQ 368(SP), X1; MOVQ 376(SP), X2
    VPTERNLOGQ $0xD2, Z2, Z1, Z0; MOVQ X0, 160(SP)
    MOVQ 384(SP), X3
    VPTERNLOGQ $0xD2, Z3, Z2, Z1; MOVQ X1, 168(SP)
    MOVQ 392(SP), X4
    VPTERNLOGQ $0xD2, Z4, Z3, Z2; MOVQ X2, 176(SP)
    MOVQ 360(SP), X0
    VPTERNLOGQ $0xD2, Z0, Z4, Z3; MOVQ X3, 184(SP)
    MOVQ 368(SP), X1
    VPTERNLOGQ $0xD2, Z1, Z0, Z4; MOVQ X4, 192(SP)

    // === Iota ===
    MOVQ 0(BP)(SI*8), AX; XORQ AX, 0(SP)

    INCQ SI
    CMPQ SI, $23
    JL round

    // Store back
    MOVQ 0(SP), AX; MOVQ AX, 0(DI)
    MOVQ 8(SP), AX; MOVQ AX, 8(DI)
    MOVQ 16(SP), AX; MOVQ AX, 16(DI)
    MOVQ 24(SP), AX; MOVQ AX, 24(DI)
    MOVQ 32(SP), AX; MOVQ AX, 32(DI)
    MOVQ 40(SP), AX; MOVQ AX, 40(DI)
    MOVQ 48(SP), AX; MOVQ AX, 48(DI)
    MOVQ 56(SP), AX; MOVQ AX, 56(DI)
    MOVQ 64(SP), AX; MOVQ AX, 64(DI)
    MOVQ 72(SP), AX; MOVQ AX, 72(DI)
    MOVQ 80(SP), AX; MOVQ AX, 80(DI)
    MOVQ 88(SP), AX; MOVQ AX, 88(DI)
    MOVQ 96(SP), AX; MOVQ AX, 96(DI)
    MOVQ 104(SP), AX; MOVQ AX, 104(DI)
    MOVQ 112(SP), AX; MOVQ AX, 112(DI)
    MOVQ 120(SP), AX; MOVQ AX, 120(DI)
    MOVQ 128(SP), AX; MOVQ AX, 128(DI)
    MOVQ 136(SP), AX; MOVQ AX, 136(DI)
    MOVQ 144(SP), AX; MOVQ AX, 144(DI)
    MOVQ 152(SP), AX; MOVQ AX, 152(DI)
    MOVQ 160(SP), AX; MOVQ AX, 160(DI)
    MOVQ 168(SP), AX; MOVQ AX, 168(DI)
    MOVQ 176(SP), AX; MOVQ AX, 176(DI)
    MOVQ 184(SP), AX; MOVQ AX, 184(DI)
    MOVQ 192(SP), AX; MOVQ AX, 192(DI)

    // Restore callee-saved registers
    MOVQ 400(SP), R12
    MOVQ 408(SP), R13
    MOVQ 416(SP), BP
    RET
