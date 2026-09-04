package pow

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Challenge 对应 /api/v0/chat/create_pow_challenge 返回 dem data.biz_data.challenge。
type Challenge struct {
	Algorithm  string `json:"algorithm"`
	Challenge  string `json:"challenge"`
	Salt       string `json:"salt"`
	ExpireAt   int64  `json:"expire_at"`
	Difficulty int64  `json:"difficulty"`
	Signature  string `json:"signature"`
	TargetPath string `json:"target_path"`
}

// BuildPrefix: "<salt>_<expire_at>_" (对应 pow.go:89)
func BuildPrefix(salt string, expireAt int64) string {
	return salt + "_" + strconv.FormatInt(expireAt, 10) + "_"
}

// powPlan holds precomputed values shared across all workers for a single challenge.
type powPlan struct {
	t0, t1, t2, t3 uint64 // target hash as 4 uint64
	tailLen        int
	paddedStates   [][25]uint64 // index by numLen (1..maxNumLen)
}

// buildPowPlan parses the challenge and precomputes the padded states.
// This is the shared preprocessing step — call once, then pass to all workers.
func buildPowPlan(challengeHex, salt string, expireAt, difficulty int64) (*powPlan, error) {
	if len(challengeHex) != 64 {
		return nil, errors.New("pow: challenge must be 64 hex chars")
	}
	target, err := hex.DecodeString(challengeHex)
	if err != nil {
		return nil, err
	}

	plan := &powPlan{}
	plan.t0 = binary.LittleEndian.Uint64(target[0:])
	plan.t1 = binary.LittleEndian.Uint64(target[8:])
	plan.t2 = binary.LittleEndian.Uint64(target[16:])
	plan.t3 = binary.LittleEndian.Uint64(target[24:])

	prefix := []byte(BuildPrefix(salt, expireAt))
	const rate = 136
	var baseState [25]uint64
	off := 0
	for off+rate <= len(prefix) {
		for i := 0; i < rate/8; i++ {
			baseState[i] ^= binary.LittleEndian.Uint64(prefix[off+i*8:])
		}
		keccakF23(&baseState)
		off += rate
	}
	plan.tailLen = len(prefix) - off
	var tail [rate]byte
	copy(tail[:], prefix[off:])

	maxNumLen := numLenFor(difficulty - 1)
	if maxNumLen < 1 {
		maxNumLen = 1
	}
	if plan.tailLen+maxNumLen >= rate {
		return nil, errors.New("pow: prefix tail too long")
	}

	plan.paddedStates = make([][25]uint64, maxNumLen+1) // index by numLen (1..maxNumLen)
	for nl := 1; nl <= maxNumLen; nl++ {
		s := baseState
		var buf [rate]byte
		copy(buf[:plan.tailLen], tail[:plan.tailLen])
		buf[plan.tailLen+nl] = 0x06
		buf[rate-1] = 0x80
		for i := 0; i < rate/8; i++ {
			s[i] ^= binary.LittleEndian.Uint64(buf[i*8:])
		}
		plan.paddedStates[nl] = s
	}
	return plan, nil
}

// SolvePow 搜索 nonce ∈ [0, difficulty) 使得 DeepSeekHashV1(prefix+str(nonce)) == challenge。
// prefix 预吸收进 state,循环内增量十进制计数 + 预计算按位长填充状态,零分配。
//
// 默认使用多核并行搜索（工作线程数 = GOMAXPROCS）。
// 设置 DS2API_POW_INTERNAL_PARALLEL=0/1 禁用并行,用 N 固定线程数,或 -1 显式全核。
func SolvePow(ctx context.Context, challengeHex, salt string, expireAt, difficulty int64) (int64, error) {
	plan, err := buildPowPlan(challengeHex, salt, expireAt, difficulty)
	if err != nil {
		return 0, err
	}

	workers := PowInternalParallel()
	if workers > 1 && difficulty > 100 {
		return solvePowParallel(ctx, plan, difficulty, workers)
	}
	return solvePowSerial(ctx, plan, difficulty)
}

// solvePowSerial performs a sequential nonce search using the precomputed plan.
func solvePowSerial(ctx context.Context, plan *powPlan, difficulty int64) (int64, error) {
	// Incremental decimal counter — avoids division in the hot loop.
	var digits [20]byte
	digits[19] = '0'
	numStart := 19
	numLen := 1

	for n := int64(0); n < difficulty; n++ {
		if n&0x3FF == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}

		s := plan.paddedStates[numLen]
		xorDigitsIntoState(&s, digits[numStart:numStart+numLen], plan.tailLen)
		keccakF23(&s)

		if s[0] == plan.t0 && s[1] == plan.t1 && s[2] == plan.t2 && s[3] == plan.t3 {
			return n, nil
		}

		// Increment decimal counter for next iteration
		i := 19
		for i >= numStart {
			if digits[i] < '9' {
				digits[i]++
				break
			}
			digits[i] = '0'
			i--
		}
		if i < numStart {
			numStart--
			numLen++
			digits[numStart] = '1'
		}
	}
	return 0, errors.New("pow: no solution within difficulty")
}

// PowInternalParallel 返回 SolvePow 内部并行度。
// 0/1 → 串行, -1 → runtime.GOMAXPROCS(0), N → 固定 N 线程。
// 可由环境变量 DS2API_POW_INTERNAL_PARALLEL 覆盖。
func PowInternalParallel() int {
	if raw := strings.TrimSpace(os.Getenv("DS2API_POW_INTERNAL_PARALLEL")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			if n == -1 {
				return runtime.GOMAXPROCS(0)
			}
			if n >= 0 {
				return n
			}
		}
	}
	return runtime.GOMAXPROCS(0)
}

// setDigitsTo sets digits[numStart:] to the decimal representation of v.
// Returns (numStart, numLen).
func setDigitsTo(digits *[20]byte, v int64) (numStart, numLen int) {
	if v == 0 {
		digits[19] = '0'
		return 19, 1
	}
	pos := 20
	for v > 0 {
		pos--
		digits[pos] = byte('0' + v%10)
		v /= 10
	}
	return pos, 20 - pos
}

// solvePowParallel splits the difficulty range across workers goroutines.
func solvePowParallel(ctx context.Context, plan *powPlan, difficulty int64, workers int) (int64, error) {
	if workers < 2 {
		workers = 2
	}
	chunkSize := difficulty / int64(workers)
	if chunkSize < 1 {
		chunkSize = 1
	}

	searchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultCh := make(chan int64, 1)
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		start := int64(w) * chunkSize
		end := start + chunkSize
		if w == workers-1 {
			end = difficulty
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(start, end int64) {
			defer wg.Done()
			if ans, ok := searchRange(searchCtx, plan, start, end); ok {
				select {
				case resultCh <- ans:
				default:
				}
				cancel()
			}
		}(start, end)
	}

	wg.Wait()
	close(resultCh)

	if ans, ok := <-resultCh; ok {
		return ans, nil
	}
	return 0, errors.New("pow: no solution within difficulty")
}

// searchRange searches for a matching nonce in [start, end) using the precomputed plan.
func searchRange(ctx context.Context, plan *powPlan, start, end int64) (int64, bool) {
	var digits [20]byte
	numStart, numLen := setDigitsTo(&digits, start)

	for n := start; n < end; n++ {
		select {
		case <-ctx.Done():
			return 0, false
		default:
		}

		s := plan.paddedStates[numLen]
		xorDigitsIntoState(&s, digits[numStart:numStart+numLen], plan.tailLen)
		keccakF23(&s)

		if s[0] == plan.t0 && s[1] == plan.t1 && s[2] == plan.t2 && s[3] == plan.t3 {
			return n, true
		}

		// Increment decimal counter
		i := 19
		for i >= numStart {
			if digits[i] < '9' {
				digits[i]++
				break
			}
			digits[i] = '0'
			i--
		}
		if i < numStart {
			numStart--
			numLen++
			digits[numStart] = '1'
		}
	}
	return 0, false
}
func numLenFor(n int64) int {
	if n < 10 {
		return 1
	}
	if n < 100 {
		return 2
	}
	if n < 1000 {
		return 3
	}
	if n < 10000 {
		return 4
	}
	if n < 100000 {
		return 5
	}
	if n < 1000000 {
		return 6
	}
	if n < 10000000 {
		return 7
	}
	// Beyond 7 digits: use strconv for correctness, though difficulty
	// this high would likely time out before reaching here.
	return len(strconv.FormatInt(n, 10))
}

// xorDigitsIntoState XORs the decimal digit bytes into the keccak state at the
// correct byte positions within the rate block (offset = tailLen).
func xorDigitsIntoState(s *[25]uint64, digits []byte, tailLen int) {
	for i, d := range digits {
		pos := tailLen + i
		s[pos/8] ^= uint64(d) << ((pos % 8) * 8)
	}
}

// BuildPowHeader 序列化 {algorithm,challenge,salt,answer,signature,target_path} 为 base64(JSON)。
// 不含 difficulty/expire_at (对应 pow.go:218)。
func BuildPowHeader(c *Challenge, answer int64) (string, error) {
	b, err := json.Marshal(map[string]any{
		"algorithm": c.Algorithm, "challenge": c.Challenge, "salt": c.Salt,
		"answer": answer, "signature": c.Signature, "target_path": c.TargetPath,
	})
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// SolveAndBuildHeader 端到端: Challenge → x-ds-pow-response header string。
func SolveAndBuildHeader(ctx context.Context, c *Challenge) (string, error) {
	if c.Algorithm != "DeepSeekHashV1" {
		return "", errors.New("pow: unsupported algorithm: " + c.Algorithm)
	}
	d := c.Difficulty
	if d == 0 {
		d = 144000
	}
	answer, err := SolvePow(ctx, c.Challenge, c.Salt, c.ExpireAt, d)
	if err != nil {
		return "", err
	}
	return BuildPowHeader(c, answer)
}
