//go:build arm64 && !purego

package pow

func init() {
	if backendName == "" {
		backendName = "neon"
	}
}

func keccakF23(s *[25]uint64) {
	switch backendName {
	case "generic":
		keccakF23Generic(s)
	default:
		keccakF23ARM64(s)
	}
}
