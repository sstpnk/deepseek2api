//go:build (!amd64 && !arm64) || purego

package pow

func init() {
	if backendName == "" {
		backendName = "generic"
	}
}

func keccakF23(s *[25]uint64) {
	keccakF23Generic(s)
}
