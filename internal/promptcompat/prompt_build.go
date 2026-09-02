package promptcompat

import (
	"ds2api/internal/prompt"
)

type PromptBuildOptions struct {
	SuppressToolPrompt           bool
	SuppressOutputIntegrityGuard bool
}

func buildOpenAIFinalPrompt(messagesRaw []any, toolsRaw any, traceID string, thinkingEnabled bool) (string, []string) {
	return BuildOpenAIPrompt(messagesRaw, toolsRaw, traceID, DefaultToolChoicePolicy(), thinkingEnabled)
}

func BuildOpenAIPrompt(messagesRaw []any, toolsRaw any, traceID string, toolPolicy ToolChoicePolicy, thinkingEnabled bool) (string, []string) {
	return BuildOpenAIPromptWithOptions(messagesRaw, toolsRaw, traceID, toolPolicy, thinkingEnabled, PromptBuildOptions{})
}

func BuildOpenAIPromptWithOptions(messagesRaw []any, toolsRaw any, traceID string, toolPolicy ToolChoicePolicy, thinkingEnabled bool, opts PromptBuildOptions) (string, []string) {
	messages := NormalizeOpenAIMessagesForPrompt(messagesRaw, traceID)
	toolNames := []string{}
	if tools, ok := toolsRaw.([]any); ok && len(tools) > 0 {
		if !opts.SuppressToolPrompt {
			messages, toolNames = injectToolPrompt(messages, tools, toolPolicy)
		} else if !toolPolicy.IsNone() {
			toolNames = extractDeclaredToolNamesForPolicy(tools, toolPolicy)
		}
	}
	return prompt.MessagesPrepareWithOptions(messages, prompt.PrepareOptions{
		SuppressOutputIntegrityGuard: opts.SuppressOutputIntegrityGuard,
	}), toolNames
}

// BuildOpenAIPromptForAdapter exposes the OpenAI-compatible prompt building flow so
// other protocol adapters (for example Gemini) can reuse the same tool/history
// normalization logic and remain behavior-compatible with chat/completions.
func BuildOpenAIPromptForAdapter(messagesRaw []any, toolsRaw any, traceID string, thinkingEnabled bool) (string, []string) {
	return buildOpenAIFinalPrompt(messagesRaw, toolsRaw, traceID, thinkingEnabled)
}
