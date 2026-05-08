package shared

import (
	"fmt"
	"net/http"
	"strings"
)

func ShouldWriteUpstreamEmptyOutputError(text, thinking string) bool {
	return strings.TrimSpace(text) == ""
}

// PromoteThinkingWhenTextEmpty returns the text to use as the visible output
// when the upstream reasoner dumped the entire answer into the thinking
// channel. Returns (newText, promoted). Content-filter responses are left
// untouched so we still surface the filter error to the caller.
func PromoteThinkingWhenTextEmpty(text, thinking string, contentFilter bool) (string, bool) {
	if contentFilter {
		return text, false
	}
	if strings.TrimSpace(text) != "" {
		return text, false
	}
	if strings.TrimSpace(thinking) == "" {
		return text, false
	}
	return thinking, true
}

func UpstreamEmptyOutputDetail(contentFilter bool, text, thinking string) (int, string, string, map[string]string) {
	details := map[string]string{
		"has_thinking": fmt.Sprintf("%v", strings.TrimSpace(thinking) != ""),
		"has_text":     fmt.Sprintf("%v", strings.TrimSpace(text) != ""),
		"text_len":     fmt.Sprintf("%d", len(text)),
	}
	_ = text
	if contentFilter {
		details["reason"] = "content_filter"
		return http.StatusBadRequest, "Upstream content filtered the response and returned no output.", "content_filter", details
	}
	if strings.TrimSpace(thinking) != "" {
		details["reason"] = "thinking_only_no_visible_text"
		return http.StatusOK, "Upstream returned reasoning without visible output.", "empty_visible_output", details
	}
	details["reason"] = "truly_empty"
	return http.StatusTooManyRequests, "Upstream account hit a rate limit and returned empty output.", "upstream_empty_output", details
}

func WriteUpstreamEmptyOutputError(w http.ResponseWriter, text, thinking string, contentFilter bool) bool {
	if !ShouldWriteUpstreamEmptyOutputError(text, thinking) {
		return false
	}
	status, message, code, details := UpstreamEmptyOutputDetail(contentFilter, text, thinking)
	WriteOpenAIErrorWithDetails(w, status, message, code, details)
	return true
}
