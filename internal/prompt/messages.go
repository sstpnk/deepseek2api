package prompt

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var markdownImagePattern = regexp.MustCompile(`!\[(.*?)\]\((.*?)\)`)

const (
	beginSentenceMarker   = "<｜begin▁of▁sentence｜>"
	systemMarker          = "<｜System｜>"
	userMarker            = "<｜User｜>"
	assistantMarker       = "<｜Assistant｜>"
	toolMarker            = "<｜Tool｜>"
	endSentenceMarker     = "<｜end▁of▁sentence｜>"
	endToolResultsMarker  = "<｜end▁of▁toolresults｜>"
	endInstructionsMarker = "<｜end▁of▁instructions｜>"
)

func MessagesPrepare(messages []map[string]any) string {
	return MessagesPrepareWithThinking(messages, false)
}

func MessagesPrepareWithThinking(messages []map[string]any, thinkingEnabled bool) string {
	type block struct {
		Role string
		Text string
	}
	processed := make([]block, 0, len(messages))
	for _, m := range messages {
		role, _ := m["role"].(string)
		text := NormalizeContent(m["content"])
		processed = append(processed, block{Role: role, Text: text})
	}
	if len(processed) == 0 {
		return ""
	}
	merged := make([]block, 0, len(processed))
	for _, msg := range processed {
		if len(merged) > 0 && merged[len(merged)-1].Role == msg.Role {
			merged[len(merged)-1].Text += "\n\n" + msg.Text
			continue
		}
		merged = append(merged, msg)
	}
	parts := make([]string, 0, len(merged)+2)
	parts = append(parts, beginSentenceMarker)
	lastRole := ""
	lastIdx := len(merged) - 1
	for i, m := range merged {
		lastRole = m.Role
		switch m.Role {
		case "assistant":
			// The final assistant block is treated as a prefill/continuation
			// prefix: the model should continue writing from this text, so we
			// leave the turn half-open (no end-of-sentence marker). Closing it
			// causes the upstream reasoner to open a brand-new turn and emit
			// the entire visible answer inside the thinking channel.
			suffix := endSentenceMarker
			if i == lastIdx {
				suffix = ""
			}
			parts = append(parts, formatRoleBlock(assistantMarker, m.Text, suffix))
		case "tool":
			if strings.TrimSpace(m.Text) != "" {
				parts = append(parts, formatRoleBlock(toolMarker, m.Text, endToolResultsMarker))
			}
		case "system":
			if text := strings.TrimSpace(m.Text); text != "" {
				parts = append(parts, formatRoleBlock(systemMarker, text, endInstructionsMarker))
			}
		case "user":
			parts = append(parts, formatRoleBlock(userMarker, m.Text, ""))
		default:
			if strings.TrimSpace(m.Text) != "" {
				parts = append(parts, m.Text)
			}
		}
	}
	if lastRole != "assistant" {
		parts = append(parts, assistantMarker)
	}
	out := strings.Join(parts, "")
	return markdownImagePattern.ReplaceAllString(out, `[${1}](${2})`)
}

// formatRoleBlock produces a single concatenated block: marker + text + endMarker.
// No whitespace is inserted between marker and text so role boundaries stay
// compact and predictable for downstream parsers.
func formatRoleBlock(marker, text, endMarker string) string {
	out := marker + text
	if strings.TrimSpace(endMarker) != "" {
		out += endMarker
	}
	return out
}

func NormalizeContent(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			typeStr, _ := m["type"].(string)
			typeStr = strings.ToLower(strings.TrimSpace(typeStr))
			if typeStr == "text" || typeStr == "output_text" || typeStr == "input_text" {
				if txt, ok := m["text"].(string); ok && txt != "" {
					parts = append(parts, txt)
					continue
				}
				if txt, ok := m["content"].(string); ok && txt != "" {
					parts = append(parts, txt)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
