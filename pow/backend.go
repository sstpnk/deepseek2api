package pow

import (
	"os"
	"strings"
)

var backendName string

// BackendName returns the active keccakF23 implementation name.
// Values: "generic", "avx512", "neon", "purego".
func BackendName() string {
	if backendName == "" {
		return "generic"
	}
	return backendName
}

// ForceBackend allows overriding the keccakF23 backend at runtime.
// Valid values: "generic", "avx512", "neon", "auto".
// Set via DS2API_POW_BACKEND env var or call directly.
func ForceBackend(name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	backendName = strings.TrimSpace(name)
}

func init() {
	if raw := strings.TrimSpace(os.Getenv("DS2API_POW_BACKEND")); raw != "" {
		ForceBackend(raw)
	}
}
