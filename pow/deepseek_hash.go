// Package pow 提供 DeepSeekHashV1 纯 Go 实现。
// DeepSeekHashV1 = SHA3-256 但跳过 Keccak-f[1600] round 0 (只做 rounds 1..23)。
//
// keccakF23 在 dispatch_*.go 和 keccak_generic.go / keccak_amd64.s 中实现,
// 根据平台和指令集自动选择最优实现。
package pow

import "encoding/binary"

// DeepSeekHashV1 返回 data 的 32 字节摘要,与 WASM wasm_deepseek_hash_v1 等价。
func DeepSeekHashV1(data []byte) [32]byte {
	const rate = 136
	var s [25]uint64

	off := 0
	for off+rate <= len(data) {
		for i := 0; i < rate/8; i++ {
			s[i] ^= binary.LittleEndian.Uint64(data[off+i*8:])
		}
		keccakF23(&s)
		off += rate
	}

	var final [rate]byte
	copy(final[:], data[off:])
	final[len(data)-off] = 0x06
	final[rate-1] |= 0x80
	for i := 0; i < rate/8; i++ {
		s[i] ^= binary.LittleEndian.Uint64(final[i*8:])
	}
	keccakF23(&s)

	var out [32]byte
	binary.LittleEndian.PutUint64(out[0:], s[0])
	binary.LittleEndian.PutUint64(out[8:], s[1])
	binary.LittleEndian.PutUint64(out[16:], s[2])
	binary.LittleEndian.PutUint64(out[24:], s[3])
	return out
}
