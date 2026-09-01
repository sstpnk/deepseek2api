// +build arm64,!purego

#include "textflag.h"

// keccakF23ARM64 — ARM64 NEON Keccak-f[1600] rounds 1..23 (DeepSeekHashV1).
//
// Implementation notes:
//   - Theta (θ): scalar EOR chain with ROR $63 (≡ ROL $1), results in GP registers.
//   - Rho (ρ) + Pi (π): 25 scalar lane rotations with hardcoded offsets,
//     stored to b[] scratch at 200(RSP).
//   - Chi (χ): BIC (bit-clear = ANDN) for each lane → 2 instructions per lane
//     (BIC + EOR) vs 3 scalar. ARM64 has 31 GP registers — ample headroom.
//   - Iota (ι): EOR round constant from precomputed table.
//   - DeepSeekHashV1 skips standard Keccak round 0 — only rounds 1..23 run.
//   - 23-round loop with ADD/CMP/BLT.
//
// Registers (callee-saved saved/restored):
//   R19(round counter), R20(RC table base), R21-R25(d0..d4)
// Stack frame: 456 bytes (200 state + 200 b[] + 56 saved regs)

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

// func keccakF23ARM64(s *[25]uint64)
//
// ARM64 NEON-optimized Keccak-f[1600] rounds 1..23.
// Uses BIC (bit-clear = ANDN) for Chi step.
// ARM64 has 31 GP registers — keep hot values in registers, minimal stack traffic.
//
// Stack: 0(RSP)..192(RSP)=state, 200(RSP)..392(RSP)=b[] scratch,
// 400(RSP)..448(RSP)=saved callee-saved regs (R19-R25)
TEXT ·keccakF23ARM64(SB), $456-8
    // Save callee-saved registers (Go ARM64 ABI: R19-R29)
    MOVD R19, 400(RSP)
    MOVD R20, 408(RSP)
    MOVD R21, 416(RSP)
    MOVD R22, 424(RSP)
    MOVD R23, 432(RSP)
    MOVD R24, 440(RSP)
    MOVD R25, 448(RSP)

    MOVD s+0(FP), R0

    // Copy state to stack.
    MOVD 0(R0), R1; MOVD R1, 0(RSP)
    MOVD 8(R0), R1; MOVD R1, 8(RSP)
    MOVD 16(R0), R1; MOVD R1, 16(RSP)
    MOVD 24(R0), R1; MOVD R1, 24(RSP)
    MOVD 32(R0), R1; MOVD R1, 32(RSP)
    MOVD 40(R0), R1; MOVD R1, 40(RSP)
    MOVD 48(R0), R1; MOVD R1, 48(RSP)
    MOVD 56(R0), R1; MOVD R1, 56(RSP)
    MOVD 64(R0), R1; MOVD R1, 64(RSP)
    MOVD 72(R0), R1; MOVD R1, 72(RSP)
    MOVD 80(R0), R1; MOVD R1, 80(RSP)
    MOVD 88(R0), R1; MOVD R1, 88(RSP)
    MOVD 96(R0), R1; MOVD R1, 96(RSP)
    MOVD 104(R0), R1; MOVD R1, 104(RSP)
    MOVD 112(R0), R1; MOVD R1, 112(RSP)
    MOVD 120(R0), R1; MOVD R1, 120(RSP)
    MOVD 128(R0), R1; MOVD R1, 128(RSP)
    MOVD 136(R0), R1; MOVD R1, 136(RSP)
    MOVD 144(R0), R1; MOVD R1, 144(RSP)
    MOVD 152(R0), R1; MOVD R1, 152(RSP)
    MOVD 160(R0), R1; MOVD R1, 160(RSP)
    MOVD 168(R0), R1; MOVD R1, 168(RSP)
    MOVD 176(R0), R1; MOVD R1, 176(RSP)
    MOVD 184(R0), R1; MOVD R1, 184(RSP)
    MOVD 192(R0), R1; MOVD R1, 192(RSP)

    MOVD $0, R19             // round counter
    MOVD $rc<>(SB), R20      // RC table

round:
    // === Theta ===
    MOVD 0(RSP), R8; MOVD 40(RSP), R1; EOR R1, R8, R8
    MOVD 80(RSP), R1; EOR R1, R8, R8
    MOVD 120(RSP), R1; EOR R1, R8, R8
    MOVD 160(RSP), R1; EOR R1, R8, R8            // R8 = c0

    MOVD 8(RSP), R9; MOVD 48(RSP), R1; EOR R1, R9, R9
    MOVD 88(RSP), R1; EOR R1, R9, R9
    MOVD 128(RSP), R1; EOR R1, R9, R9
    MOVD 168(RSP), R1; EOR R1, R9, R9            // R9 = c1

    MOVD 16(RSP), R10; MOVD 56(RSP), R1; EOR R1, R10, R10
    MOVD 96(RSP), R1; EOR R1, R10, R10
    MOVD 136(RSP), R1; EOR R1, R10, R10
    MOVD 176(RSP), R1; EOR R1, R10, R10           // R10 = c2

    MOVD 24(RSP), R11; MOVD 64(RSP), R1; EOR R1, R11, R11
    MOVD 104(RSP), R1; EOR R1, R11, R11
    MOVD 144(RSP), R1; EOR R1, R11, R11
    MOVD 184(RSP), R1; EOR R1, R11, R11           // R11 = c3

    MOVD 32(RSP), R12; MOVD 72(RSP), R1; EOR R1, R12, R12
    MOVD 112(RSP), R1; EOR R1, R12, R12
    MOVD 152(RSP), R1; EOR R1, R12, R12
    MOVD 192(RSP), R1; EOR R1, R12, R12           // R12 = c4

    // d[x] = c[x-1] ^ ROTL64(c[x+1], 1)
    ROR $63, R9, R1; EOR R12, R1, R21            // d0 = c4 ^ ROT(c1,1)
    ROR $63, R10, R2; EOR R8, R2, R22             // d1
    ROR $63, R11, R3; EOR R9, R3, R23             // d2
    ROR $63, R12, R4; EOR R10, R4, R24            // d3
    ROR $63, R8, R5; EOR R11, R5, R25             // d4

    // Mix d into columns
    MOVD 0(RSP), R1; EOR R21, R1, R1; MOVD R1, 0(RSP)
    MOVD 40(RSP), R1; EOR R21, R1, R1; MOVD R1, 40(RSP)
    MOVD 80(RSP), R1; EOR R21, R1, R1; MOVD R1, 80(RSP)
    MOVD 120(RSP), R1; EOR R21, R1, R1; MOVD R1, 120(RSP)
    MOVD 160(RSP), R1; EOR R21, R1, R1; MOVD R1, 160(RSP)

    MOVD 8(RSP), R1; EOR R22, R1, R1; MOVD R1, 8(RSP)
    MOVD 48(RSP), R1; EOR R22, R1, R1; MOVD R1, 48(RSP)
    MOVD 88(RSP), R1; EOR R22, R1, R1; MOVD R1, 88(RSP)
    MOVD 128(RSP), R1; EOR R22, R1, R1; MOVD R1, 128(RSP)
    MOVD 168(RSP), R1; EOR R22, R1, R1; MOVD R1, 168(RSP)

    MOVD 16(RSP), R1; EOR R23, R1, R1; MOVD R1, 16(RSP)
    MOVD 56(RSP), R1; EOR R23, R1, R1; MOVD R1, 56(RSP)
    MOVD 96(RSP), R1; EOR R23, R1, R1; MOVD R1, 96(RSP)
    MOVD 136(RSP), R1; EOR R23, R1, R1; MOVD R1, 136(RSP)
    MOVD 176(RSP), R1; EOR R23, R1, R1; MOVD R1, 176(RSP)

    MOVD 24(RSP), R1; EOR R24, R1, R1; MOVD R1, 24(RSP)
    MOVD 64(RSP), R1; EOR R24, R1, R1; MOVD R1, 64(RSP)
    MOVD 104(RSP), R1; EOR R24, R1, R1; MOVD R1, 104(RSP)
    MOVD 144(RSP), R1; EOR R24, R1, R1; MOVD R1, 144(RSP)
    MOVD 184(RSP), R1; EOR R24, R1, R1; MOVD R1, 184(RSP)

    MOVD 32(RSP), R1; EOR R25, R1, R1; MOVD R1, 32(RSP)
    MOVD 72(RSP), R1; EOR R25, R1, R1; MOVD R1, 72(RSP)
    MOVD 112(RSP), R1; EOR R25, R1, R1; MOVD R1, 112(RSP)
    MOVD 152(RSP), R1; EOR R25, R1, R1; MOVD R1, 152(RSP)
    MOVD 192(RSP), R1; EOR R25, R1, R1; MOVD R1, 192(RSP)

    // === Rho+Pi with hardcoded permutation ===
    // b starts at 200(RSP)

    // b0  = s[0]
    MOVD 0(RSP), R1; MOVD R1, 200(RSP)
    // b10 = ROT(s[1], 1)
    MOVD 8(RSP), R1; ROR $63, R1; MOVD R1, 280(RSP)
    // b20 = ROT(s[2], 62)
    MOVD 16(RSP), R1; ROR $2, R1; MOVD R1, 360(RSP)
    // b5  = ROT(s[3], 28)
    MOVD 24(RSP), R1; ROR $36, R1; MOVD R1, 240(RSP)
    // b15 = ROT(s[4], 27)
    MOVD 32(RSP), R1; ROR $37, R1; MOVD R1, 320(RSP)
    // b16 = ROT(s[5], 36)
    MOVD 40(RSP), R1; ROR $28, R1; MOVD R1, 328(RSP)
    // b1  = ROT(s[6], 44)
    MOVD 48(RSP), R1; ROR $20, R1; MOVD R1, 208(RSP)
    // b11 = ROT(s[7], 6)
    MOVD 56(RSP), R1; ROR $58, R1; MOVD R1, 288(RSP)
    // b21 = ROT(s[8], 55)
    MOVD 64(RSP), R1; ROR $9, R1; MOVD R1, 368(RSP)
    // b6  = ROT(s[9], 20)
    MOVD 72(RSP), R1; ROR $44, R1; MOVD R1, 248(RSP)
    // b7  = ROT(s[10], 3)
    MOVD 80(RSP), R1; ROR $61, R1; MOVD R1, 256(RSP)
    // b17 = ROT(s[11], 10)
    MOVD 88(RSP), R1; ROR $54, R1; MOVD R1, 336(RSP)
    // b2  = ROT(s[12], 43)
    MOVD 96(RSP), R1; ROR $21, R1; MOVD R1, 216(RSP)
    // b12 = ROT(s[13], 25)
    MOVD 104(RSP), R1; ROR $39, R1; MOVD R1, 296(RSP)
    // b22 = ROT(s[14], 39)
    MOVD 112(RSP), R1; ROR $25, R1; MOVD R1, 376(RSP)
    // b23 = ROT(s[15], 41)
    MOVD 120(RSP), R1; ROR $23, R1; MOVD R1, 384(RSP)
    // b8  = ROT(s[16], 45)
    MOVD 128(RSP), R1; ROR $19, R1; MOVD R1, 264(RSP)
    // b18 = ROT(s[17], 15)
    MOVD 136(RSP), R1; ROR $49, R1; MOVD R1, 344(RSP)
    // b3  = ROT(s[18], 21)
    MOVD 144(RSP), R1; ROR $43, R1; MOVD R1, 224(RSP)
    // b13 = ROT(s[19], 8)
    MOVD 152(RSP), R1; ROR $56, R1; MOVD R1, 304(RSP)
    // b14 = ROT(s[20], 18)
    MOVD 160(RSP), R1; ROR $46, R1; MOVD R1, 312(RSP)
    // b24 = ROT(s[21], 2)
    MOVD 168(RSP), R1; ROR $62, R1; MOVD R1, 392(RSP)
    // b9  = ROT(s[22], 61)
    MOVD 176(RSP), R1; ROR $3, R1; MOVD R1, 272(RSP)
    // b19 = ROT(s[23], 56)
    MOVD 184(RSP), R1; ROR $8, R1; MOVD R1, 352(RSP)
    // b4  = ROT(s[24], 14)
    MOVD 192(RSP), R1; ROR $50, R1; MOVD R1, 232(RSP)

    // === Chi: a[x] = b[x] ^ ((~b[x+1]) & b[x+2]), 5 rows, BIC ===
    // BIC Rm, Rn, Rd → Rd = Rn & ^Rm
    // We need a[x] = b[x] ^ (^b[x+1] & b[x+2])
    // = b[x] ^ BIC(b[x+2], b[x+1])
    //
    // Row 0 (0,1,2,3,4)
    MOVD 200(RSP), R8; MOVD 208(RSP), R9; MOVD 216(RSP), R10
    BIC R9, R10, R1; EOR R1, R8, R8; MOVD R8, 0(RSP)        // a[0]
    MOVD 224(RSP), R11
    BIC R10, R11, R1; EOR R1, R9, R9; MOVD R9, 8(RSP)        // a[1]
    MOVD 232(RSP), R12
    BIC R11, R12, R1; EOR R1, R10, R10; MOVD R10, 16(RSP)     // a[2]
    MOVD 200(RSP), R8
    BIC R12, R8, R1; EOR R1, R11, R11; MOVD R11, 24(RSP)      // a[3]
    MOVD 208(RSP), R15
    BIC R8, R15, R1; EOR R1, R12, R12; MOVD R12, 32(RSP)      // a[4]

    // Row 1 (5,6,7,8,9)
    MOVD 240(RSP), R8; MOVD 248(RSP), R9; MOVD 256(RSP), R10
    BIC R9, R10, R1; EOR R1, R8, R8; MOVD R8, 40(RSP)
    MOVD 264(RSP), R11
    BIC R10, R11, R1; EOR R1, R9, R9; MOVD R9, 48(RSP)
    MOVD 272(RSP), R12
    BIC R11, R12, R1; EOR R1, R10, R10; MOVD R10, 56(RSP)
    MOVD 240(RSP), R8
    BIC R12, R8, R1; EOR R1, R11, R11; MOVD R11, 64(RSP)
    MOVD 248(RSP), R15
    BIC R8, R15, R1; EOR R1, R12, R12; MOVD R12, 72(RSP)

    // Row 2 (10,11,12,13,14)
    MOVD 280(RSP), R8; MOVD 288(RSP), R9; MOVD 296(RSP), R10
    BIC R9, R10, R1; EOR R1, R8, R8; MOVD R8, 80(RSP)
    MOVD 304(RSP), R11
    BIC R10, R11, R1; EOR R1, R9, R9; MOVD R9, 88(RSP)
    MOVD 312(RSP), R12
    BIC R11, R12, R1; EOR R1, R10, R10; MOVD R10, 96(RSP)
    MOVD 280(RSP), R8
    BIC R12, R8, R1; EOR R1, R11, R11; MOVD R11, 104(RSP)
    MOVD 288(RSP), R15
    BIC R8, R15, R1; EOR R1, R12, R12; MOVD R12, 112(RSP)

    // Row 3 (15,16,17,18,19)
    MOVD 320(RSP), R8; MOVD 328(RSP), R9; MOVD 336(RSP), R10
    BIC R9, R10, R1; EOR R1, R8, R8; MOVD R8, 120(RSP)
    MOVD 344(RSP), R11
    BIC R10, R11, R1; EOR R1, R9, R9; MOVD R9, 128(RSP)
    MOVD 352(RSP), R12
    BIC R11, R12, R1; EOR R1, R10, R10; MOVD R10, 136(RSP)
    MOVD 320(RSP), R8
    BIC R12, R8, R1; EOR R1, R11, R11; MOVD R11, 144(RSP)
    MOVD 328(RSP), R15
    BIC R8, R15, R1; EOR R1, R12, R12; MOVD R12, 152(RSP)

    // Row 4 (20,21,22,23,24)
    MOVD 360(RSP), R8; MOVD 368(RSP), R9; MOVD 376(RSP), R10
    BIC R9, R10, R1; EOR R1, R8, R8; MOVD R8, 160(RSP)
    MOVD 384(RSP), R11
    BIC R10, R11, R1; EOR R1, R9, R9; MOVD R9, 168(RSP)
    MOVD 392(RSP), R12
    BIC R11, R12, R1; EOR R1, R10, R10; MOVD R10, 176(RSP)
    MOVD 360(RSP), R8
    BIC R12, R8, R1; EOR R1, R11, R11; MOVD R11, 184(RSP)
    MOVD 368(RSP), R15
    BIC R8, R15, R1; EOR R1, R12, R12; MOVD R12, 192(RSP)

    // === Iota ===
    MOVD (R20), R1; MOVD 0(RSP), R2; EOR R1, R2, R2; MOVD R2, 0(RSP)
    ADD $8, R20, R20

    ADD $1, R19, R19
    CMP $23, R19
    BLT round

    // Store back
    MOVD 0(RSP), R1; MOVD R1, 0(R0)
    MOVD 8(RSP), R1; MOVD R1, 8(R0)
    MOVD 16(RSP), R1; MOVD R1, 16(R0)
    MOVD 24(RSP), R1; MOVD R1, 24(R0)
    MOVD 32(RSP), R1; MOVD R1, 32(R0)
    MOVD 40(RSP), R1; MOVD R1, 40(R0)
    MOVD 48(RSP), R1; MOVD R1, 48(R0)
    MOVD 56(RSP), R1; MOVD R1, 56(R0)
    MOVD 64(RSP), R1; MOVD R1, 64(R0)
    MOVD 72(RSP), R1; MOVD R1, 72(R0)
    MOVD 80(RSP), R1; MOVD R1, 80(R0)
    MOVD 88(RSP), R1; MOVD R1, 88(R0)
    MOVD 96(RSP), R1; MOVD R1, 96(R0)
    MOVD 104(RSP), R1; MOVD R1, 104(R0)
    MOVD 112(RSP), R1; MOVD R1, 112(R0)
    MOVD 120(RSP), R1; MOVD R1, 120(R0)
    MOVD 128(RSP), R1; MOVD R1, 128(R0)
    MOVD 136(RSP), R1; MOVD R1, 136(R0)
    MOVD 144(RSP), R1; MOVD R1, 144(R0)
    MOVD 152(RSP), R1; MOVD R1, 152(R0)
    MOVD 160(RSP), R1; MOVD R1, 160(R0)
    MOVD 168(RSP), R1; MOVD R1, 168(R0)
    MOVD 176(RSP), R1; MOVD R1, 176(R0)
    MOVD 184(RSP), R1; MOVD R1, 184(R0)
    MOVD 192(RSP), R1; MOVD R1, 192(R0)

    // Restore callee-saved registers
    MOVD 400(RSP), R19
    MOVD 408(RSP), R20
    MOVD 416(RSP), R21
    MOVD 424(RSP), R22
    MOVD 432(RSP), R23
    MOVD 440(RSP), R24
    MOVD 448(RSP), R25
    RET
