package config

import "strings"

type ModelInfo struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	Created    int64  `json:"created"`
	OwnedBy    string `json:"owned_by"`
	Permission []any  `json:"permission,omitempty"`
}

type ModelAliasReader interface {
	ModelAliases() map[string]string
}

const (
	noThinkingModelSuffix    = "-nothinking"
	reducedPromptModelSuffix = "-rp"
)

var deepSeekBaseModels = []ModelInfo{
	{ID: "deepseek-v4-flash", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
	{ID: "deepseek-v4-pro", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
	{ID: "deepseek-v4-flash-search", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
	{ID: "deepseek-v4-pro-search", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
	{ID: "deepseek-v4-vision", Object: "model", Created: 1677610602, OwnedBy: "deepseek", Permission: []any{}},
}

var DeepSeekModels = appendDeepSeekVariants(deepSeekBaseModels)

func GetModelConfig(model string) (thinking bool, search bool, ok bool) {
	baseModel, noThinking, _ := splitDeepSeekVariant(model)
	if baseModel == "" {
		return false, false, false
	}
	if IsReducedPromptModel(model) && !supportsReducedPromptBaseModel(baseModel) {
		return false, false, false
	}
	switch baseModel {
	case "deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v4-vision":
		return !noThinking, false, true
	case "deepseek-v4-flash-search", "deepseek-v4-pro-search":
		return !noThinking, true, true
	default:
		return false, false, false
	}
}

func GetModelType(model string) (modelType string, ok bool) {
	baseModel, _, _ := splitDeepSeekVariant(model)
	if IsReducedPromptModel(model) && !supportsReducedPromptBaseModel(baseModel) {
		return "", false
	}
	switch baseModel {
	case "deepseek-v4-flash", "deepseek-v4-flash-search":
		return "default", true
	case "deepseek-v4-pro", "deepseek-v4-pro-search":
		return "expert", true
	case "deepseek-v4-vision":
		return "vision", true
	default:
		return "", false
	}
}

func IsSupportedDeepSeekModel(model string) bool {
	_, _, ok := GetModelConfig(model)
	return ok
}

func IsNoThinkingModel(model string) bool {
	_, noThinking, _ := splitDeepSeekVariant(model)
	return noThinking
}

func IsReducedPromptModel(model string) bool {
	_, _, reducedPrompt := splitDeepSeekVariant(model)
	return reducedPrompt
}

func IsRoleplayPromptModel(model string) bool {
	baseModel, _, reducedPrompt := splitDeepSeekVariant(model)
	return reducedPrompt && supportsReducedPromptBaseModel(baseModel)
}

func supportsReducedPromptBaseModel(baseModel string) bool {
	switch baseModel {
	case "deepseek-v4-flash", "deepseek-v4-pro":
		return true
	default:
		return false
	}
}

func DefaultModelAliases() map[string]string {
	return map[string]string{
		// OpenAI GPT / ChatGPT families
		"chatgpt-4o":          "deepseek-v4-flash",
		"gpt-4":               "deepseek-v4-flash",
		"gpt-4-turbo":         "deepseek-v4-flash",
		"gpt-4-turbo-preview": "deepseek-v4-flash",
		"gpt-4.5-preview":     "deepseek-v4-flash",
		"gpt-4o":              "deepseek-v4-flash",
		"gpt-4o-mini":         "deepseek-v4-flash",
		"gpt-4.1":             "deepseek-v4-flash",
		"gpt-4.1-mini":        "deepseek-v4-flash",
		"gpt-4.1-nano":        "deepseek-v4-flash",
		"gpt-5":               "deepseek-v4-flash",
		"gpt-5-chat":          "deepseek-v4-flash",
		"gpt-5.1":             "deepseek-v4-flash",
		"gpt-5.1-chat":        "deepseek-v4-flash",
		"gpt-5.2":             "deepseek-v4-flash",
		"gpt-5.2-chat":        "deepseek-v4-flash",
		"gpt-5.3-chat":        "deepseek-v4-flash",
		"gpt-5.4":             "deepseek-v4-flash",
		"gpt-5.5":             "deepseek-v4-flash",
		"gpt-5-mini":          "deepseek-v4-flash",
		"gpt-5-nano":          "deepseek-v4-flash",
		"gpt-5.4-mini":        "deepseek-v4-flash",
		"gpt-5.4-nano":        "deepseek-v4-flash",
		"gpt-5-pro":           "deepseek-v4-pro",
		"gpt-5.2-pro":         "deepseek-v4-pro",
		"gpt-5.4-pro":         "deepseek-v4-pro",
		"gpt-5.5-pro":         "deepseek-v4-pro",
		"gpt-5-codex":         "deepseek-v4-pro",
		"gpt-5.1-codex":       "deepseek-v4-pro",
		"gpt-5.1-codex-mini":  "deepseek-v4-pro",
		"gpt-5.1-codex-max":   "deepseek-v4-pro",
		"gpt-5.2-codex":       "deepseek-v4-pro",
		"gpt-5.3-codex":       "deepseek-v4-pro",
		"codex-mini-latest":   "deepseek-v4-pro",

		// OpenAI reasoning / research families
		"o1":                    "deepseek-v4-pro",
		"o1-preview":            "deepseek-v4-pro",
		"o1-mini":               "deepseek-v4-pro",
		"o1-pro":                "deepseek-v4-pro",
		"o3":                    "deepseek-v4-pro",
		"o3-mini":               "deepseek-v4-pro",
		"o3-pro":                "deepseek-v4-pro",
		"o3-deep-research":      "deepseek-v4-pro-search",
		"o4-mini":               "deepseek-v4-pro",
		"o4-mini-deep-research": "deepseek-v4-pro-search",

		"llama-3.1-70b-instruct": "deepseek-v4-flash",
		"qwen-max":               "deepseek-v4-flash",
	}
}

func ResolveModel(store ModelAliasReader, requested string) (string, bool) {
	model := lower(strings.TrimSpace(requested))
	if model == "" {
		return "", false
	}
	aliases := loadModelAliases(store)
	if IsSupportedDeepSeekModel(model) {
		return model, true
	}
	if mapped, ok := aliases[model]; ok && IsSupportedDeepSeekModel(mapped) {
		return lower(strings.TrimSpace(mapped)), true
	}
	baseModel, noThinking, reducedPrompt := splitDeepSeekVariant(model)
	if mapped, ok := aliases[baseModel]; ok && IsSupportedDeepSeekModel(mapped) {
		candidate := withDeepSeekVariant(mapped, noThinking, reducedPrompt)
		if IsSupportedDeepSeekModel(candidate) {
			return lower(strings.TrimSpace(candidate)), true
		}
	}
	return "", false
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func OpenAIModelsResponse() map[string]any {
	return map[string]any{"object": "list", "data": DeepSeekModels}
}

func OpenAIModelByID(store ModelAliasReader, id string) (ModelInfo, bool) {
	canonical, ok := ResolveModel(store, id)
	if !ok {
		return ModelInfo{}, false
	}
	for _, model := range DeepSeekModels {
		if model.ID == canonical {
			return model, true
		}
	}
	return ModelInfo{}, false
}

func appendDeepSeekVariants(models []ModelInfo) []ModelInfo {
	out := make([]ModelInfo, 0, len(models)*2+4)
	for _, model := range models {
		out = append(out, model)
		noThinking := model
		noThinking.ID = withDeepSeekVariant(model.ID, true, false)
		out = append(out, noThinking)
		if supportsReducedPromptBaseModel(model.ID) {
			reducedPrompt := model
			reducedPrompt.ID = withDeepSeekVariant(model.ID, false, true)
			out = append(out, reducedPrompt)
			noThinkingReducedPrompt := model
			noThinkingReducedPrompt.ID = withDeepSeekVariant(model.ID, true, true)
			out = append(out, noThinkingReducedPrompt)
		}
	}
	return out
}

func splitDeepSeekVariant(model string) (baseModel string, noThinking bool, reducedPrompt bool) {
	model = lower(strings.TrimSpace(model))
	if strings.HasSuffix(model, reducedPromptModelSuffix) {
		reducedPrompt = true
		model = strings.TrimSuffix(model, reducedPromptModelSuffix)
	}
	if strings.HasSuffix(model, noThinkingModelSuffix) {
		noThinking = true
		model = strings.TrimSuffix(model, noThinkingModelSuffix)
	}
	return model, noThinking, reducedPrompt
}

func withDeepSeekVariant(model string, noThinking, reducedPrompt bool) string {
	baseModel, _, _ := splitDeepSeekVariant(model)
	if baseModel == "" {
		return ""
	}
	out := baseModel
	if noThinking {
		out += noThinkingModelSuffix
	}
	if reducedPrompt {
		out += reducedPromptModelSuffix
	}
	return out
}

func loadModelAliases(store ModelAliasReader) map[string]string {
	aliases := DefaultModelAliases()
	if store != nil {
		for k, v := range store.ModelAliases() {
			key := lower(strings.TrimSpace(k))
			val := lower(strings.TrimSpace(v))
			if key == "" || val == "" {
				continue
			}
			aliases[key] = val
		}
	}
	return aliases
}
