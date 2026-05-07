//go:build amd64 && !purego

package pow

// keccak_amd64.s is a placeholder for future SIMD implementations.
// Currently amd64 uses pure Go via dispatch_amd64.go → keccakF23Generic.
