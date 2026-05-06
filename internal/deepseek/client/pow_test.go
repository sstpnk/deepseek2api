package client

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	powpkg "ds2api/pow"

	"ds2api/internal/auth"
	dsprotocol "ds2api/internal/deepseek/protocol"
)

func TestPreloadPowNoOp(t *testing.T) {
	client := NewClient(nil, nil)
	if err := client.PreloadPow(context.Background()); err != nil {
		t.Fatalf("PreloadPow should be no-op, got error: %v", err)
	}
}

func TestComputePowUnsupportedAlgorithm(t *testing.T) {
	_, err := ComputePow(context.Background(), map[string]any{"algorithm": "unknown"})
	if err == nil {
		t.Fatal("expected error for unsupported algorithm")
	}
}

func TestSolvePowHeaderCachesByAccountAndTarget(t *testing.T) {
	challengeHash := powpkg.DeepSeekHashV1([]byte(powpkg.BuildPrefix("salt", 1712345678) + "42"))
	challenge := map[string]any{
		"algorithm":   "DeepSeekHashV1",
		"challenge":   hex.EncodeToString(challengeHash[:]),
		"salt":        "salt",
		"expire_at":   float64(1712345678),
		"difficulty":  float64(1000),
		"signature":   "sig",
		"target_path": dsprotocol.DeepSeekUploadTargetPath,
	}
	client := NewClient(nil, nil)
	authCtx := &auth.RequestAuth{DeepSeekToken: "token", AccountID: "account"}

	first, err := client.solvePowHeader(context.Background(), authCtx, dsprotocol.DeepSeekUploadTargetPath, challenge)
	if err != nil {
		t.Fatalf("first solve failed: %v", err)
	}
	cached, ok := client.cachedPowHeader(authCtx, dsprotocol.DeepSeekUploadTargetPath)
	if !ok || cached != first {
		t.Fatalf("expected cached pow header, ok=%v cached=%q first=%q", ok, cached, first)
	}

	client.invalidatePowHeader(authCtx, dsprotocol.DeepSeekUploadTargetPath)
	if _, ok := client.cachedPowHeader(authCtx, dsprotocol.DeepSeekUploadTargetPath); ok {
		t.Fatal("expected invalidated pow header to be absent")
	}
}

func TestInvalidPowResponseDetection(t *testing.T) {
	if !isInvalidPowResponse(40301, 0, "INVALID_POW_RESPONSE", "") {
		t.Fatal("expected code 40301 to be invalid pow")
	}
	if !isInvalidPowResponse(0, 0, "bad", "invalid pow response") {
		t.Fatal("expected message to be invalid pow")
	}
	if isInvalidPowResponse(0, 0, strings.Repeat("ok", 2), "") {
		t.Fatal("did not expect unrelated response to be invalid pow")
	}
}
