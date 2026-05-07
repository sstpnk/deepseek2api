//go:build arm64 && !purego

package pow

func keccakF23(s *[25]uint64) {
	keccakF23ARM64(s)
}
