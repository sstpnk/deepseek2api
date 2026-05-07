//go:build amd64 && !purego

package pow

import (
	"fmt"
	"os"

	"golang.org/x/sys/cpu"
)

func init() {
	if backendName != "" {
		return // user forced a specific backend
	}
	if cpu.X86.HasAVX512F && cpu.X86.HasAVX512VL {
		backendName = "avx512"
	} else {
		backendName = "generic"
	}
	validateBackend()
}

func keccakF23(s *[25]uint64) {
	switch backendName {
	case "avx512":
		keccakF23AVX512(s)
	default:
		keccakF23Generic(s)
	}
}

// validateBackend verifies the active backend against the pure Go reference
// using the standard test vector. Falls back to generic on mismatch.
func validateBackend() {
	// Standard test vector from TestDeepSeekHashV1
	input := []byte("testsalt_1700000000_42")
	want := DeepSeekHashV1(input)

	// Temporarily force generic to get reference result
	saved := backendName
	backendName = "generic"
	got := DeepSeekHashV1(input)
	backendName = saved

	if got == want {
		backendValidated = true
		return
	}

	fmt.Fprintf(os.Stderr, "[pow] ERROR: %s backend self-test FAILED — falling back to generic. Expected %x, got %x\n", saved, want, got)
	backendName = "generic"
}