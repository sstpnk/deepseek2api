//go:build amd64 && !purego

package pow

// keccakF23AVX2 is implemented in keccak_amd64.s.
// It performs 23 rounds of Keccak-f[1600] (DeepSeekHashV1 variant: skip round 0).
func keccakF23AVX2(s *[25]uint64)
