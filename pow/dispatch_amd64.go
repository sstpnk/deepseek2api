//go:build amd64 && !purego

package pow

import "golang.org/x/sys/cpu"

func init() {
	if backendName != "" {
		return // forced by DS2API_POW_BACKEND
	}
	if cpu.X86.HasAVX512F && cpu.X86.HasAVX512VL {
		backendName = "avx512"
	} else {
		backendName = "generic"
	}
}

func keccakF23(s *[25]uint64) {
	switch backendName {
	case "avx512":
		keccakF23AVX512(s)
	default:
		keccakF23Generic(s)
	}
}
