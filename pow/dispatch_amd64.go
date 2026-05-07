//go:build amd64 && !purego

package pow

// keccakF23 on amd64: the pure Go unrolled implementation keeps enough values
// in registers that hand-written scalar assembly doesn't beat it on most CPUs.
// AVX-512 and NEON paths will use dedicated SIMD assembly.
func keccakF23(s *[25]uint64) {
	keccakF23Generic(s)
}
