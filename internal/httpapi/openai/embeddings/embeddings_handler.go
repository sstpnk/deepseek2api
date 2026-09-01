// Package embeddings exposes the OpenAI-compatible /v1/embeddings handler.
package embeddings

import (
	"encoding/json"
	"net/http"

	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/shared"
)

// Handler is the /v1/embeddings handler.
type Handler struct {
	Deps *shared.Deps
}

// NewHandler returns a new embeddings handler.
func NewHandler(deps *shared.Deps) *Handler { return &Handler{Deps: deps} }

// ServeHTTP handles the request.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Model string `json:"model"`
		Input any    `json:"input"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, shared.GeneralMaxSize)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	_ = body
	a, _ := h.Deps.Auth.Determine(r.Context())
	ctx := auth.WithAuth(r.Context(), a)
	_ = ctx
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"object":"list","data":[],"model":"","usage":{"prompt_tokens":0,"total_tokens":0}}`))
}
