package shared

import (
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

type thinkingInjectionStore struct {
	enabled bool
	prompt  string
}

func (s thinkingInjectionStore) ModelAliases() map[string]string     { return nil }
func (s thinkingInjectionStore) ToolcallMode() string                { return "" }
func (s thinkingInjectionStore) ToolcallEarlyEmitConfidence() string { return "" }
func (s thinkingInjectionStore) ResponsesStoreTTLSeconds() int       { return 0 }
func (s thinkingInjectionStore) EmbeddingsProvider() string          { return "" }
func (s thinkingInjectionStore) AutoDeleteMode() string              { return "none" }
func (s thinkingInjectionStore) AutoDeleteSessions() bool            { return false }
func (s thinkingInjectionStore) CurrentInputFileEnabled() bool       { return false }
func (s thinkingInjectionStore) CurrentInputFileMinChars() int       { return 0 }
func (s thinkingInjectionStore) ThinkingInjectionEnabled() bool      { return s.enabled }
func (s thinkingInjectionStore) ThinkingInjectionPrompt() string     { return s.prompt }

func TestApplyThinkingInjectionRoleplayUsesStandardDefaultPrompt(t *testing.T) {
	stdReq := promptcompat.StandardRequest{
		ResolvedModel: "deepseek-v4-flash-rp",
		Messages:      []any{map[string]any{"role": "user", "content": "continue"}},
		FinalPrompt:   "continue",
		Thinking:      true,
	}

	out := ApplyThinkingInjection(thinkingInjectionStore{enabled: true}, stdReq)

	if !strings.Contains(out.FinalPrompt, promptcompat.ThinkingInjectionMarker) {
		t.Fatalf("expected standard thinking injection, got %q", out.FinalPrompt)
	}
	for _, forbidden := range []string{"SillyTavern", "roleplay presets"} {
		if strings.Contains(out.FinalPrompt, forbidden) {
			t.Fatalf("expected RP thinking injection to use standard default and omit %q, got %q", forbidden, out.FinalPrompt)
		}
	}
}

func TestApplyThinkingInjectionNonRoleplayUsesStandardDefaultPrompt(t *testing.T) {
	stdReq := promptcompat.StandardRequest{
		ResolvedModel: "deepseek-v4-flash",
		Messages:      []any{map[string]any{"role": "user", "content": "continue"}},
		FinalPrompt:   "continue",
		Thinking:      true,
	}

	out := ApplyThinkingInjection(thinkingInjectionStore{enabled: true}, stdReq)

	if !strings.Contains(out.FinalPrompt, promptcompat.ThinkingInjectionMarker) {
		t.Fatalf("expected standard thinking injection, got %q", out.FinalPrompt)
	}
	for _, forbidden := range []string{"SillyTavern", "roleplay presets"} {
		if strings.Contains(out.FinalPrompt, forbidden) {
			t.Fatalf("expected non-RP default prompt to omit %q, got %q", forbidden, out.FinalPrompt)
		}
	}
}

func TestApplyThinkingInjectionRoleplayCustomPromptWins(t *testing.T) {
	stdReq := promptcompat.StandardRequest{
		ResolvedModel: "deepseek-v4-pro-rp",
		Messages:      []any{map[string]any{"role": "user", "content": "continue"}},
		FinalPrompt:   "continue",
		Thinking:      true,
	}

	out := ApplyThinkingInjection(thinkingInjectionStore{enabled: true, prompt: "custom thinking prompt"}, stdReq)

	if !strings.Contains(out.FinalPrompt, "custom thinking prompt") {
		t.Fatalf("expected custom prompt, got %q", out.FinalPrompt)
	}
	if strings.Contains(out.FinalPrompt, promptcompat.ThinkingInjectionMarker) {
		t.Fatalf("expected custom prompt to replace default, got %q", out.FinalPrompt)
	}
}

func TestApplyThinkingInjectionRoleplayNoThinkingSkipsPrompt(t *testing.T) {
	stdReq := promptcompat.StandardRequest{
		ResolvedModel: "deepseek-v4-flash-nothinking-rp",
		Messages:      []any{map[string]any{"role": "user", "content": "continue"}},
		FinalPrompt:   "continue",
		Thinking:      false,
	}

	out := ApplyThinkingInjection(thinkingInjectionStore{enabled: true}, stdReq)

	if out.FinalPrompt != stdReq.FinalPrompt {
		t.Fatalf("expected no thinking injection when thinking is disabled, got %q", out.FinalPrompt)
	}
}
