//go:build amd64 && !purego

package pow

import "golang.org/x/sys/cpu"

func keccakF23(s *[25]uint64) {
	if cpu.X86.HasAVX512F && cpu.X86.HasAVX512VL {
		keccakF23AVX512(s)
	} else {
		keccakF23Generic(s)
	}
}
