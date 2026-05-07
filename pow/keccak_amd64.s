// +build amd64,!purego

#include "textflag.h"

// keccak_amd64.s — placeholder for future SIMD implementations.
// Currently amd64 uses pure Go (dispatch_amd64.go → keccakF23Generic).
// TODO: AVX-512 VPTERNLOGQ + VPROLQ when Go assembler supports EVEX encoding.
