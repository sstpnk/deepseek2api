//go:build amd64 && !purego

package pow

// keccakF23 on amd64 uses pure Go.
// Go compiler's register allocator beats hand-written scalar assembly for
// single-instance keccak. AVX-512 path pending Go assembler EVEX support.
func keccakF23(s *[25]uint64) {
	keccakF23Generic(s)
}
