//go:build amd64 && !purego

package pow

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/cpu"
)

func init() {
	if backendRequested == "avx512" {
		backendName = "avx512"
	} else if backendName == "" && cpu.X86.HasAVX512F && cpu.X86.HasAVX512VL {
		// AVX-512 is experimental — don't auto-select. User must opt in via
		// DS2API_POW_BACKEND=avx512. Remove this guard once validated on real hardware.
		backendName = "generic"
	} else if backendName == "" {
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

// validateBackend verifies the active backend produces identical results to
// the pure Go reference. On mismatch, falls back to generic. If strict mode
// is enabled (DS2API_POW_BACKEND_STRICT=1), self-test failure causes a fatal exit.
func validateBackend() {
	// Use the standard test vector: DeepSeekHashV1("testsalt_1700000000_42")
	input := []byte("testsalt_1700000000_42")

	// Reference: pure Go generic backend
	saved := backendName
	backendName = "generic"
	expected := DeepSeekHashV1(input)

	// Actual: active backend
	backendName = saved
	actual := DeepSeekHashV1(input)
	// restore in case backendName was modified during the call
	backendName = saved

	if expected == actual {
		backendValidated = true
		return
	}

	fmt.Fprintf(os.Stderr, "[pow] ERROR: %s backend self-test FAILED — expected (generic) != actual (%s)\n", saved, saved)
	fmt.Fprintf(os.Stderr, "[pow]   expected_generic: %x\n", expected)
	fmt.Fprintf(os.Stderr, "[pow]   actual_%s: %x\n", saved, actual)
	fmt.Fprintf(os.Stderr, "[pow]   falling back to generic backend\n")

	backendName = "generic"
	backendValidated = false

	if strings.TrimSpace(os.Getenv("DS2API_POW_BACKEND_STRICT")) == "1" {
		fmt.Fprintf(os.Stderr, "[pow] FATAL: DS2API_POW_BACKEND_STRICT=1 and backend self-test failed\n")
		os.Exit(1)
	}
}