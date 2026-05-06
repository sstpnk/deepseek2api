package gemini

import (
	"strings"
	"testing"
)

func TestNormalizeGeminiRequestNoThinkingModelForcesThinkingOff(t *testing.T) {
	req := map[string]any{
		"contents": []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "hello"}},
			},
		},
		"reasoning_effort": "high",
	}
	out, err := normalizeGeminiRequest(testGeminiConfig{}, "gemini-2.5-pro-nothinking", req, false)
	if err != nil {
		t.Fatalf("normalizeGeminiRequest error: %v", err)
	}
	if out.ResolvedModel != "deepseek-v4-pro-nothinking" {
		t.Fatalf("resolved model mismatch: got=%q", out.ResolvedModel)
	}
	if out.Thinking {
		t.Fatalf("expected nothinking model to force thinking off")
	}
	if out.Search {
		t.Fatalf("expected search=false, got=%v", out.Search)
	}
}

func TestNormalizeGeminiRequestReducedPromptOmitsToolInstructions(t *testing.T) {
	req := map[string]any{
		"contents": []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "hello"}},
			},
		},
		"tools": []any{
			map[string]any{
				"functionDeclarations": []any{
					map[string]any{
						"name":        "search",
						"description": "Search docs",
						"parameters": map[string]any{
							"type": "object",
						},
					},
				},
			},
		},
	}
	out, err := normalizeGeminiRequest(testGeminiConfig{}, "gemini-2.5-flash-rp", req, false)
	if err != nil {
		t.Fatalf("normalizeGeminiRequest error: %v", err)
	}
	if out.ResolvedModel != "deepseek-v4-flash-rp" {
		t.Fatalf("resolved model mismatch: got=%q", out.ResolvedModel)
	}
	if !out.SuppressToolPrompt {
		t.Fatal("expected reduced prompt model to suppress tool prompt injection")
	}
	if len(out.ToolNames) != 1 || out.ToolNames[0] != "search" {
		t.Fatalf("expected tool names to remain available, got %#v", out.ToolNames)
	}
	if strings.Contains(out.FinalPrompt, "You have access to these tools:") || strings.Contains(out.FinalPrompt, "TOOL CALL FORMAT") {
		t.Fatalf("reduced prompt should omit tool instructions, got %q", out.FinalPrompt)
	}
}
