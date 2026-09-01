// Package server wires the slim OpenAI-compatible HTTP surface.
package server

import (
	"context"
	"net/http"
	"os"
	"strings"

	"ds2api/internal/config"
	"ds2api/internal/deepseek/client"
	"ds2api/internal/httpapi/openai/chat"
	"ds2api/internal/httpapi/openai/embeddings"
	"ds2api/internal/httpapi/openai/files"
	"ds2api/internal/httpapi/openai/responses"
	"ds2api/internal/httpapi/openai/shared"
)

// App bundles the HTTP application state.
type App struct {
	Store *config.Store
	DS    *client.Client
	mux   *http.ServeMux
}

// NewApp constructs the slim HTTP application.
func NewApp(_ context.Context, store *config.Store) *App {
	ds := client.NewClient(store, nil)
	deps := &shared.Deps{
		Store: shared.StoreConfigReader{Store: store},
		Auth:  ds.Auth(),
		DS:    ds,
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", chat.NewHandler(deps))
	mux.Handle("/v1/responses", responses.NewHandler(deps))
	mux.Handle("/v1/embeddings", embeddings.NewHandler(deps))
	mux.Handle("/v1/files", files.NewHandler(deps))
	mux.Handle("/v1/models", &shared.ModelsHandler{Store: shared.StoreConfigReader{Store: store}})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return &App{Store: store, DS: ds, mux: mux}
}

// Handler returns the http.Handler for the application with CORS pre-installed.
func (a *App) Handler() http.Handler {
	return corsMiddleware(splitCSV(os.Getenv("DS2API_CORS_ALLOWED_ORIGINS")))(a.mux)
}

// corsMiddleware wraps an http.Handler with permissive CORS for the slim
// OpenAI-compatible surface. By default the request's Origin is echoed
// (development-friendly). To restrict to a known set, pass allowed origins
// via corsAllowlist in the constructor.
func corsMiddleware(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				echo := origin
				if len(allowed) > 0 {
					echo = matchOrigin(origin, allowed)
				}
				if echo != "" {
					w.Header().Set("Access-Control-Allow-Origin", echo)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Requested-With")
					w.Header().Set("Access-Control-Max-Age", "86400")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func matchOrigin(origin string, allowed []string) string {
	for _, a := range allowed {
		if a == "*" || strings.EqualFold(a, origin) {
			return origin
		}
	}
	return ""
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
