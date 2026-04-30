package util

import (
	"strings"

	tiktoken "github.com/hupe1980/go-tiktoken"
)

const (
	defaultTokenizerModel = "gpt-4o"
	claudeTokenizerModel  = "claude"
)

func CountPromptTokens(text, model string) int {
	base := maxTokenCount(
		EstimateTokens(text),
		countWithTokenizer(text, model),
	)
	if base <= 0 {
		return 0
	}
	return base + conservativePromptPadding(base)
}

func CountOutputTokens(text, model string) int {
	base := maxTokenCount(
		EstimateTokens(text),
		countWithTokenizer(text, model),
	)
	if base <= 0 {
		return 0
	}
	return base
}

func countWithTokenizer(text, model string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	encoding, err := tiktoken.NewEncodingForModel(tokenizerModelForCount(model))
	if err != nil {
		return 0
	}
	ids, _, err := encoding.Encode(text, nil, nil)
	if err != nil {
		return 0
	}
	return len(ids)
}

func tokenizerModelForCount(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return defaultTokenizerModel
	}
	switch {
	case strings.HasPrefix(model, "claude"):
		return claudeTokenizerModel
	case strings.HasPrefix(model, "gpt-4"), strings.HasPrefix(model, "gpt-5"), strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "o4"):
		return defaultTokenizerModel
	case strings.HasPrefix(model, "deepseek-v4"):
		return defaultTokenizerModel
	case strings.HasPrefix(model, "deepseek"):
		return defaultTokenizerModel
	default:
		return defaultTokenizerModel
	}
}

func conservativePromptPadding(base int) int {
	padding := base / 50
	if padding < 4 {
		padding = 4
	}
	return padding
}

func maxTokenCount(values ...int) int {
	best := 0
	for _, v := range values {
		if v > best {
			best = v
		}
	}
	return best
}
