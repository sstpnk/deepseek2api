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

// TestPowPlanEquivalence verifies that buildPowPlan produces padded states
// equivalent to the full hash path for every possible digit length.
func TestPowPlanEquivalence(t *testing.T) {
	salt := "testplan"
	expire := int64(1712345678)
	for answer := int64(0); answer < 10000; answer++ {
		// Reference: full DeepSeekHashV1
		fullInput := BuildPrefix(salt, expire) + strconv.FormatInt(answer, 10)
		want := DeepSeekHashV1([]byte(fullInput))

		// powPlan-based
		plan, err := buildPowPlan(hex.EncodeToString(want[:]), salt, expire, answer+1)
		if err != nil {
			t.Fatalf("buildPowPlan failed for answer %d: %v", answer, err)
		}
		numLen := numLenFor(answer)
		var digits [20]byte
		numStart, nLen := setDigitsTo(&digits, answer)
		if nLen != numLen {
			t.Fatalf("numLen mismatch: %d vs %d", nLen, numLen)
		}
		s := plan.paddedStates[numLen]
		xorDigitsIntoState(&s, digits[numStart:numStart+numLen], plan.tailLen)
		keccakF23(&s)

		if s[0] != plan.t0 || s[1] != plan.t1 || s[2] != plan.t2 || s[3] != plan.t3 {
			t.Fatalf("answer %d: powPlan result does not match target", answer)
		}
	}
}

// TestSolvePowProperty runs randomized property tests: for random salt/expire/answer,
// solving the generated challenge must find the original answer.
func TestSolvePowProperty(t *testing.T) {
	rng := mathrand.New(mathrand.NewSource(12345))

	// Also test concurrent solves
	t.Run("serial_random", func(t *testing.T) {
		// Force serial path
		t.Setenv("DS2API_POW_INTERNAL_PARALLEL", "1")
		for i := 0; i < 50; i++ {
			salt := fmt.Sprintf("salt%d", rng.Int63n(1000000))
			expire := int64(1700000000 + rng.Int63n(100000000))
			answer := rng.Int63n(10000)
			diff := answer + rng.Int63n(100) + 1
			if diff < answer+1 {
				diff = answer + 1
			}

			h := DeepSeekHashV1([]byte(BuildPrefix(salt, expire) + strconv.FormatInt(answer, 10)))
			ch := hex.EncodeToString(h[:])
			got, err := SolvePow(context.Background(), ch, salt, expire, diff)
			if err != nil {
				t.Fatalf("answer=%d salt=%s: %v", answer, salt, err)
			}
			if got != answer {
				t.Fatalf("expected %d, got %d", answer, got)
			}
		}
	})

	// Test that difficulty < answer+1 fails
	t.Run("unsolvable", func(t *testing.T) {
		t.Setenv("DS2API_POW_INTERNAL_PARALLEL", "1")
		salt := "unsolvable"
		expire := int64(1712345678)
		answer := int64(500)
		h := DeepSeekHashV1([]byte(BuildPrefix(salt, expire) + strconv.FormatInt(answer, 10)))
		ch := hex.EncodeToString(h[:])
		_, err := SolvePow(context.Background(), ch, salt, expire, answer) // diff < answer
		if err == nil {
			t.Fatal("expected error for unsolvable range")
		}
	})
}
