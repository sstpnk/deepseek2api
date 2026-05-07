//go:build amd64 && !purego

package pow

func keccakF23AVX512(s *[25]uint64)
