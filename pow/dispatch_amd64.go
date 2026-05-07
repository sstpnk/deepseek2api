//go:build amd64 && !purego

package pow

// keccakF23 on amd64 uses the pure Go implementation.
// Hand-written scalar assembly doesn't beat Go's register allocator
// for single-instance keccak. The real wins are SIMD paths:
// AVX-512 (VPROLQ + VPTERNLOG) and batch-lane processing.
func keccakF23(s *[25]uint64) {
	keccakF23Generic(s)
}
