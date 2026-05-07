package pow

import (
	"fmt"
	"os"
	"strings"
)

var (
	backendName      string
	backendRequested string // what the user asked for (env or default "auto")
	backendValidated bool   // true after self-test passes
)

var validBackends = map[string]bool{
	"auto":    true,
	"generic": true,
	"avx512":  true,
	"neon":    true,
	"purego":  true,
}

// BackendName returns the active keccakF23 implementation.
func BackendName() string {
	if backendName == "" {
		return "generic"
	}
	return backendName
}

// BackendRequested returns what was configured (env var or "auto").
func BackendRequested() string {
	if backendRequested == "" {
		return "auto"
	}
	return backendRequested
}

// BackendValidated returns true if the active backend passed the startup self-test.
func BackendValidated() bool {
	return backendValidated
}

func init() {
	raw := strings.TrimSpace(os.Getenv("DS2API_POW_BACKEND"))
	if raw == "" {
		raw = "auto"
	}
	backendRequested = raw
	if !validBackends[raw] {
		fmt.Fprintf(os.Stderr, "[pow] WARNING: DS2API_POW_BACKEND=%q is invalid, falling back to auto. Valid: auto, generic, avx512, neon, purego\n", raw)
		raw = "auto"
	}
	if raw != "auto" {
		backendName = raw
	}
}