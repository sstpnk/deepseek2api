//go:build (!amd64 && !arm64) || purego

package pow

// keccakF23 on non-amd64 or purego builds uses the pure Go implementation.
func keccakF23(s *[25]uint64) {
	keccakF23Generic(s)
}
