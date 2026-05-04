package shared

import (
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

func UpstreamEmptyOutputDetail(contentFilter bool, text, thinking string) (int, string, string) {
	_ = text
	if contentFilter {
		return http.StatusBadRequest, "Upstream content filtered the response and returned no output.", "content_filter"
	}
	if thinking != "" {
		return http.StatusTooManyRequests, "Upstream account hit a rate limit and returned reasoning without visible output.", "upstream_empty_output"
	}
	return http.StatusTooManyRequests, "Upstream account hit a rate limit and returned empty output.", "upstream_empty_output"
}

func WriteUpstreamEmptyOutputError(w http.ResponseWriter, text, thinking string, contentFilter bool) bool {
	if !ShouldWriteUpstreamEmptyOutputError(text, thinking) {
		return false
	}
	status, message, code := UpstreamEmptyOutputDetail(contentFilter, text, thinking)
	WriteOpenAIErrorWithCode(w, status, message, code)
	return true
}
