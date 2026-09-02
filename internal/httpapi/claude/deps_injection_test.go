package claude

import (
	"strings"
	"testing"
)

type mockClaudeConfig struct {
	aliases map[string]string
}

func (m mockClaudeConfig) ModelAliases() map[string]string { return m.aliases }
func (mockClaudeConfig) CurrentInputFileEnabled() bool     { return true }
func (mockClaudeConfig) CurrentInputFileMinChars() int     { return 0 }

func TestNormalizeClaudeRequestUsesGlobalAliasMapping(t *testing.T) {
	req := map[string]any{
		"model": "claude-opus-4-6",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	out, err := normalizeClaudeRequest(mockClaudeConfig{
		aliases: map[string]string{
			"claude-opus-4-6": "deepseek-v4-pro-search",
		},
	}, req)
	if err != nil {
		t.Fatalf("normalizeClaudeRequest error: %v", err)
	}
	if out.Standard.ResolvedModel != "deepseek-v4-pro-search" {
		t.Fatalf("resolved model mismatch: got=%q", out.Standard.ResolvedModel)
	}
	if !out.Standard.Thinking || !out.Standard.Search {
		t.Fatalf("unexpected flags: thinking=%v search=%v", out.Standard.Thinking, out.Standard.Search)
	}
}

func TestNormalizeClaudeRequestDisablesThinkingWhenRequested(t *testing.T) {
	req := map[string]any{
		"model": "claude-opus-4-6",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"thinking": map[string]any{"type": "disabled"},
	}
	out, err := normalizeClaudeRequest(mockClaudeConfig{
		aliases: map[string]string{
			"claude-opus-4-6": "deepseek-v4-pro",
		},
	}, req)
	if err != nil {
		t.Fatalf("normalizeClaudeRequest error: %v", err)
	}
	if out.Standard.Thinking {
		t.Fatalf("expected explicit Claude thinking disable to win")
	}
}

func TestNormalizeClaudeRequestEnablesThinkingWhenRequested(t *testing.T) {
	req := map[string]any{
		"model": "claude-opus-4-6",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"thinking": map[string]any{"type": "enabled", "budget_tokens": 1024},
	}
	out, err := normalizeClaudeRequest(mockClaudeConfig{
		aliases: map[string]string{
			"claude-opus-4-6": "deepseek-v4-pro",
		},
	}, req)
	if err != nil {
		t.Fatalf("normalizeClaudeRequest error: %v", err)
	}
	if !out.Standard.Thinking {
		t.Fatalf("expected explicit Claude thinking request to enable downstream thinking")
	}
}

func TestNormalizeClaudeRequestNoThinkingAliasForcesThinkingOff(t *testing.T) {
	req := map[string]any{
		"model": "claude-opus-4-6-nothinking",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"thinking": map[string]any{"type": "enabled", "budget_tokens": 1024},
	}
	out, err := normalizeClaudeRequest(mockClaudeConfig{}, req)
	if err != nil {
		t.Fatalf("normalizeClaudeRequest error: %v", err)
	}
	if out.Standard.ResolvedModel != "deepseek-v4-pro-nothinking" {
		t.Fatalf("resolved model mismatch: got=%q", out.Standard.ResolvedModel)
	}
	if out.Standard.Thinking {
		t.Fatalf("expected nothinking alias to force downstream thinking off")
	}
}

func TestNormalizeClaudeRequestPrefersGlobalAliasMapping(t *testing.T) {
	req := map[string]any{
		"model": "claude-sonnet-4-6",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	out, err := normalizeClaudeRequest(mockClaudeConfig{
		aliases: map[string]string{
			"claude-sonnet-4-6": "deepseek-v4-flash",
		},
	}, req)
	if err != nil {
		t.Fatalf("normalizeClaudeRequest error: %v", err)
	}
	if out.Standard.ResolvedModel != "deepseek-v4-flash" {
		t.Fatalf("expected global alias to win for explicit model, got=%q", out.Standard.ResolvedModel)
	}
}

func TestNormalizeClaudeRequestRejectsReducedPromptModel(t *testing.T) {
	req := map[string]any{
		"model": "claude-sonnet-4-6-rp",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"tools": []any{
			map[string]any{
				"name":        "search",
				"description": "Search docs",
				"input_schema": map[string]any{
					"type": "object",
				},
			},
		},
	}
	_, err := normalizeClaudeRequest(mockClaudeConfig{}, req)
	if err == nil || !strings.Contains(err.Error(), "OpenAI-compatible endpoint for -rp models") {
		t.Fatalf("expected Claude endpoint to reject -rp model, got %v", err)
	}
}
