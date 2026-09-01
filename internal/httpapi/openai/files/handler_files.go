// Package files exposes a minimal /v1/files handler in the slim build.
package files

import (
	"encoding/json"
	"net/http"

	"ds2api/internal/auth"
	"ds2api/internal/httpapi/openai/shared"
)

// Handler is the /v1/files handler.
type Handler struct {
	Deps *shared.Deps
}

// NewHandler returns a new files handler.
func NewHandler(deps *shared.Deps) *Handler { return &Handler{Deps: deps} }

// ServeHTTP handles the request. The slim build only returns the OpenAI
// list-files response; uploads are not yet wired up.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data":   []any{},
		})
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_, _ = h.Deps.Auth.Determine(r.Context())
	_ = auth.WithAuth(r.Context(), nil)
	http.Error(w, "uploads not yet implemented in slim build", http.StatusNotImplemented)
}
