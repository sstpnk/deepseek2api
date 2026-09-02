package history

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/shared"
	"ds2api/internal/promptcompat"
)

const (
	currentInputFilename     = promptcompat.CurrentInputContextFilename
	currentInputContentType  = "text/plain; charset=utf-8"
	currentInputPurpose      = "assistants"
	roleplayLiveUserMaxRunes = 150000
)

type CurrentInputConfigReader interface {
	CurrentInputFileEnabled() bool
	CurrentInputFileMinChars() int
}

type ThinkingInjectionConfigReader interface {
	ThinkingInjectionEnabled() bool
	ThinkingInjectionPrompt() string
}

type CurrentInputUploader interface {
	UploadFile(ctx context.Context, a *auth.RequestAuth, req dsclient.UploadFileRequest, maxAttempts int) (*dsclient.UploadFileResult, error)
}

type Service struct {
	Store CurrentInputConfigReader
	DS    CurrentInputUploader
}

func (s Service) ApplyCurrentInputFile(ctx context.Context, a *auth.RequestAuth, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if stdReq.CurrentInputFileApplied || s.DS == nil || s.Store == nil || a == nil || !s.Store.CurrentInputFileEnabled() {
		return stdReq, nil
	}
	isRoleplayPrompt := isOpenAIRoleplayPromptRequest(stdReq)
	threshold := s.Store.CurrentInputFileMinChars()

	index, text := latestUserInputForFile(stdReq.Messages)
	if index < 0 {
		return stdReq, nil
	}
	if !isRoleplayPrompt && len([]rune(text)) < threshold {
		return stdReq, nil
	}
	liveLatestUserText := strings.TrimSpace(stdReq.LatestUserInputText)
	if liveLatestUserText == "" {
		liveLatestUserText = text
	}
	fileText := promptcompat.BuildOpenAICurrentInputContextTranscript(stdReq.Messages)
	if strings.TrimSpace(fileText) == "" {
		return stdReq, errors.New("current user input file produced empty transcript")
	}
	modelType := "default"
	if resolvedType, ok := config.GetModelType(stdReq.ResolvedModel); ok {
		modelType = resolvedType
	}
	result, err := s.DS.UploadFile(ctx, a, dsclient.UploadFileRequest{
		Filename:    currentInputFilename,
		ContentType: currentInputContentType,
		Purpose:     currentInputPurpose,
		ModelType:   modelType,
		Data:        []byte(fileText),
	}, 3)
	if err != nil {
		return stdReq, fmt.Errorf("upload current user input file: %w", err)
	}
	fileID := strings.TrimSpace(result.ID)
	if fileID == "" {
		return stdReq, errors.New("upload current user input file returned empty file id")
	}

	livePrompt := openAICurrentInputFilePrompt(stdReq, liveLatestUserText, roleplayLiveThinkingPrompt(s.Store, stdReq))
	messages := []any{
		map[string]any{
			"role":    "user",
			"content": livePrompt,
		},
	}

	stdReq.Messages = messages
	stdReq.HistoryText = fileText
	stdReq.CurrentInputFileApplied = true
	stdReq.RefFileIDs = prependUniqueRefFileID(stdReq.RefFileIDs, fileID)
	stdReq.FinalPrompt, stdReq.ToolNames = promptcompat.BuildOpenAIPromptWithOptions(messages, stdReq.ToolsRaw, "", stdReq.ToolChoice, stdReq.Thinking, promptcompat.PromptBuildOptions{
		SuppressToolPrompt:           stdReq.SuppressToolPrompt,
		SuppressOutputIntegrityGuard: isRoleplayPrompt,
	})
	// Token accounting must reflect the actual downstream context:
	// the uploaded DS2API_HISTORY.txt file content + the continuation live prompt.
	stdReq.PromptTokenText = fileText + "\n" + stdReq.FinalPrompt
	return stdReq, nil
}

func latestUserInputForFile(messages []any) (int, string) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
		if role != "user" {
			continue
		}
		text := promptcompat.NormalizeOpenAIContentForPrompt(msg["content"])
		if strings.TrimSpace(text) == "" {
			return -1, ""
		}
		return i, text
	}
	return -1, ""
}

func isOpenAIRoleplayPromptRequest(stdReq promptcompat.StandardRequest) bool {
	switch stdReq.Surface {
	case "openai_chat", "openai_responses":
		return config.IsRoleplayPromptModel(stdReq.ResolvedModel)
	default:
		return false
	}
}

func openAICurrentInputFilePrompt(stdReq promptcompat.StandardRequest, latestUserText, thinkingPrompt string) string {
	if isOpenAIRoleplayPromptRequest(stdReq) {
		return roleplayCurrentInputFilePrompt(latestUserText, thinkingPrompt)
	}
	return standardCurrentInputFilePrompt()
}

func standardCurrentInputFilePrompt() string {
	return "Continue from the latest state in the attached DS2API_HISTORY.txt context. Treat it as the current working state and answer the latest user request directly."
}

func roleplayCurrentInputFilePrompt(latestUserText, thinkingPrompt string) string {
	latestUserText = strings.TrimSpace(latestUserText)
	latestUserText = trailingRunes(latestUserText, roleplayLiveUserMaxRunes)
	parts := []string{
		"Context boundary: use the attached DS2API_HISTORY.txt as prior state, ignore corrupted fragments, and answer only the latest user input below.",
	}
	if latestUserText != "" {
		parts = append(parts, latestUserText)
	}
	parts = append(parts, "Continue the ongoing roleplay/story from the attached DS2API_HISTORY.txt context. Treat the attached file as the complete current state, including all SillyTavern presets, character cards, world/lore entries, author notes, injection-depth prompts, output format rules, prior dialogue, and the latest user turn.")
	if thinkingPrompt = strings.TrimSpace(thinkingPrompt); thinkingPrompt != "" {
		parts = append(parts, thinkingPrompt)
	}
	return strings.Join(parts, "\n\n")
}

func trailingRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for i := len(text); i > 0; {
		_, size := utf8.DecodeLastRuneInString(text[:i])
		if size <= 0 {
			break
		}
		i -= size
		count++
		if count == maxRunes {
			return text[i:]
		}
	}
	return text
}

func roleplayLiveThinkingPrompt(store CurrentInputConfigReader, stdReq promptcompat.StandardRequest) string {
	if !config.IsRoleplayPromptModel(stdReq.ResolvedModel) || !stdReq.Thinking {
		return ""
	}
	cfg, ok := store.(ThinkingInjectionConfigReader)
	if !ok || !cfg.ThinkingInjectionEnabled() {
		return ""
	}
	if prompt := strings.TrimSpace(cfg.ThinkingInjectionPrompt()); prompt != "" {
		return prompt
	}
	return promptcompat.DefaultThinkingInjectionPrompt
}

func prependUniqueRefFileID(existing []string, fileID string) []string {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return existing
	}
	out := make([]string, 0, len(existing)+1)
	out = append(out, fileID)
	for _, id := range existing {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" || strings.EqualFold(trimmed, fileID) {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
