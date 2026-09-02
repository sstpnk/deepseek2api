package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"ds2api/pow"

	"ds2api/internal/auth"
	dsprotocol "ds2api/internal/deepseek/protocol"
)

// ComputePow 使用纯 Go 实现求解 PoW challenge (DeepSeekHashV1)。
func ComputePow(ctx context.Context, challenge map[string]any) (int64, error) {
	algo, _ := challenge["algorithm"].(string)
	if algo != "DeepSeekHashV1" {
		return 0, errors.New("unsupported algorithm")
	}
	challengeStr, _ := challenge["challenge"].(string)
	salt, _ := challenge["salt"].(string)
	expireAt := toInt64(challenge["expire_at"], 1680000000)
	difficulty := toInt64FromFloat(challenge["difficulty"], 144000)

	return pow.SolvePow(ctx, challengeStr, salt, expireAt, difficulty)
}

// BuildPowHeader 序列化 {algorithm,challenge,salt,answer,signature,target_path} 为 base64(JSON)。
func BuildPowHeader(challenge map[string]any, answer int64) (string, error) {
	payload := map[string]any{
		"algorithm":   challenge["algorithm"],
		"challenge":   challenge["challenge"],
		"salt":        challenge["salt"],
		"answer":      answer,
		"signature":   challenge["signature"],
		"target_path": challenge["target_path"],
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func (c *Client) solvePowHeader(ctx context.Context, a *auth.RequestAuth, targetPath string, challenge map[string]any) (string, error) {
	if c == nil || c.pow == nil {
		return solvePowHeaderDirect(ctx, challenge)
	}
	if strings.TrimSpace(targetPath) != dsprotocol.DeepSeekUploadTargetPath {
		return c.pow.computeUncached(ctx, challenge)
	}
	return c.pow.compute(ctx, powCachePrefix(a, targetPath), challenge)
}

func (c *Client) invalidatePowHeader(a *auth.RequestAuth, targetPath string) {
	if c == nil || c.pow == nil {
		return
	}
	c.pow.invalidatePrefix(powCachePrefix(a, targetPath))
}

func (c *Client) cachedPowHeader(a *auth.RequestAuth, targetPath string) (string, bool) {
	if c == nil || c.pow == nil {
		return "", false
	}
	return c.pow.getCached(powCachePrefix(a, targetPath))
}

func powCachePrefix(a *auth.RequestAuth, targetPath string) string {
	token := ""
	accountID := ""
	if a != nil {
		token = strings.TrimSpace(a.DeepSeekToken)
		accountID = strings.TrimSpace(a.AccountID)
	}
	targetPath = strings.TrimSpace(targetPath)
	if token != "" {
		return token + "|" + targetPath
	}
	if accountID != "" {
		return accountID + "|" + targetPath
	}
	return targetPath
}

func isInvalidPowResponse(code, bizCode int, msg, bizMsg string) bool {
	if code == 40301 || bizCode == 40301 {
		return true
	}
	combined := strings.ToLower(strings.TrimSpace(msg) + " " + strings.TrimSpace(bizMsg))
	return strings.Contains(combined, "invalid_pow") ||
		strings.Contains(combined, "invalid pow") ||
		strings.Contains(combined, "pow_response")
}

func toFloat64(v any, d float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return d
	}
}

func toInt64(v any, d int64) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	default:
		return d
	}
}

// toInt64FromFloat 与 toInt64 等价，仅名称区分用途。
func toInt64FromFloat(v any, d int64) int64 {
	return toInt64(v, d)
}
