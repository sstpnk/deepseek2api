package pow

import (
	"context"
	"encoding/hex"
	"fmt"
	mathrand "math/rand"
	"strconv"
	"strings"
	"testing"
)

// TestKeccakF23CrossCheck verifies that the platform-specific keccakF23
// implementation produces identical results to the pure Go reference.
func TestKeccakF23CrossCheck(t *testing.T) {
	rng := mathrand.New(mathrand.NewSource(42))

	for i := 0; i < 100; i++ {
		var state [25]uint64
		for j := range state {
			state[j] = rng.Uint64()
		}

		var goState [25]uint64
		copy(goState[:], state[:])
		keccakF23Generic(&goState)

		var activeState [25]uint64
		copy(activeState[:], state[:])
		keccakF23(&activeState)

		if goState != activeState {
			t.Fatalf("iter %d: keccakF23 and keccakF23Generic differ\ngeneric: %v\nactive: %v", i, goState, activeState)
		}
	}
}

// TestDeepSeekHashV1Deterministic verifies DeepSeekHashV1 is deterministic.
func TestDeepSeekHashV1Deterministic(t *testing.T) {
	rng := mathrand.New(mathrand.NewSource(99))
	inputs := [][]byte{
		{},
		[]byte("hello"),
		[]byte("testsalt_1700000000_42"),
		make([]byte, 200),
		make([]byte, 136),
		make([]byte, 135),
	}
	for _, b := range inputs[3:] {
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
	}
	for _, input := range inputs {
		h1 := DeepSeekHashV1(input)
		h2 := DeepSeekHashV1(input)
		if h1 != h2 {
			t.Fatalf("DeepSeekHashV1 not deterministic for input len=%d", len(input))
		}
	}
}

// TestSolvePowBoundaries exercises edge cases for the solver.
func TestSolvePowBoundaries(t *testing.T) {
	t.Run("context_canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := SolvePow(ctx, "00"+strings.Repeat("0", 62), "salt", 1712345678, 100000)
		if err == nil {
			t.Fatal("expected error from canceled context")
		}
	})

	t.Run("long_salt", func(t *testing.T) {
		// Use salt length that doesn't hit the exact rate boundary
		salt := strings.Repeat("x", 100)
		h := DeepSeekHashV1([]byte(BuildPrefix(salt, 1712345678) + "777"))
		ch := hex.EncodeToString(h[:])
		answer, err := SolvePow(context.Background(), ch, salt, 1712345678, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if answer != 777 {
			t.Fatalf("expected 777, got %d", answer)
		}
	})

	// Test salt that hits exactly rate-1 boundary (data + padding collide at last byte)
	t.Run("salt_rate_edge", func(t *testing.T) {
		// 120 x's + "_1712345678_" = 132 bytes. With 3-digit nonce = 135 bytes.
		// pad byte and 0x80 both at position 135 → 0x86.
		salt := strings.Repeat("x", 120)
		prefix := BuildPrefix(salt, 1712345678)
		h := DeepSeekHashV1([]byte(prefix + "42"))
		ch := hex.EncodeToString(h[:])
		answer, err := SolvePow(context.Background(), ch, salt, 1712345678, 100)
		if err != nil {
			t.Fatal(err)
		}
		if answer != 42 {
			t.Fatalf("expected 42, got %d", answer)
		}
	})

	// Test digit boundary transitions (incremental counter edge cases)
	for _, answer := range []int64{0, 1, 9, 10, 99, 100, 999, 1000, 9999} {
		t.Run(fmt.Sprintf("answer_%d", answer), func(t *testing.T) {
			salt := "test"
			expire := int64(1712345678)
			h := DeepSeekHashV1([]byte(BuildPrefix(salt, expire) + strconv.FormatInt(answer, 10)))
			ch := hex.EncodeToString(h[:])
			got, err := SolvePow(context.Background(), ch, salt, expire, answer+1)
			if err != nil {
				t.Fatal(err)
			}
			if got != answer {
				t.Fatalf("expected %d, got %d", answer, got)
			}
		})
	}

	// Test difficulty=1 finds answer 0
	t.Run("difficulty_1", func(t *testing.T) {
		h := DeepSeekHashV1([]byte("s_1712345678_0"))
		ch := hex.EncodeToString(h[:])
		got, err := SolvePow(context.Background(), ch, "s", 1712345678, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got != 0 {
			t.Fatalf("expected 0, got %d", got)
		}
	})
}
