// Package chat exposes the OpenAI-compatible /v1/chat/completions handler.
package chat

import (
	"encoding/json"
	"errors"
	"net/http"

	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/shared"
)

// Handler is the /v1/chat/completions handler.
type Handler struct {
	Deps *shared.Deps
}

// NewHandler returns a new chat handler.
func NewHandler(deps *shared.Deps) *Handler { return &Handler{Deps: deps} }

// ServeHTTP handles the request.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Model    string           `json:"model"`
		Messages []map[string]any `json:"messages"`
		Stream   bool             `json:"stream"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, shared.GeneralMaxSize)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	a, _ := h.Deps.Auth.Determine(r.Context())
	ctx := auth.WithAuth(r.Context(), a)
	resp, err := h.Deps.DS.CallCompletion(ctx, a, map[string]any{
		"model":    body.Model,
		"messages": body.Messages,
		"stream":   body.Stream,
	}, "", 1)
	if err != nil {
		if errors.Is(err, auth.ErrNoAccount) {
			http.Error(w, "no account available", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_ = http.NewResponseController(w)
}
