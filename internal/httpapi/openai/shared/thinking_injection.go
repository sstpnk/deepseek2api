package shared

import (
	"ds2api/internal/promptcompat"
)

func ApplyThinkingInjection(store ConfigReader, stdReq promptcompat.StandardRequest) promptcompat.StandardRequest {
	if store == nil || !store.ThinkingInjectionEnabled() || !stdReq.Thinking {
		return stdReq
	}
	defaultPrompt := promptcompat.DefaultThinkingInjectionPrompt
	messages, changed := promptcompat.AppendThinkingInjectionPromptToLatestUserWithDefault(stdReq.Messages, store.ThinkingInjectionPrompt(), defaultPrompt)
	if !changed {
		return stdReq
	}
	finalPrompt, toolNames := promptcompat.BuildOpenAIPromptWithOptions(messages, stdReq.ToolsRaw, "", stdReq.ToolChoice, stdReq.Thinking, promptcompat.PromptBuildOptions{SuppressToolPrompt: stdReq.SuppressToolPrompt})
	if len(toolNames) == 0 && len(stdReq.ToolNames) > 0 {
		toolNames = stdReq.ToolNames
	}
	stdReq.Messages = messages
	stdReq.FinalPrompt = finalPrompt
	stdReq.ToolNames = toolNames
	return stdReq
}
