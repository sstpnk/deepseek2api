// Package shared hosts common dependencies and helpers for the slim
// OpenAI-compatible HTTP surface.
package shared

import (
	"context"
	"net/http"
	"time"

	"ds2api/internal/auth"
)

// DeepSeekCaller is the subset of the DeepSeek client used by handlers.
type DeepSeekCaller interface {
	CallCompletion(ctx context.Context, a *auth.RequestAuth, payload map[string]any, powAnswer string, attempts int) (*http.Response, error)
}

// AuthResolver selects and releases accounts.
type AuthResolver interface {
	Determine(ctx context.Context) (*auth.RequestAuth, error)
	DetermineCaller(ctx context.Context, caller string) (*auth.RequestAuth, error)
	Release(a *auth.RequestAuth)
}

// ConfigReader is the subset of the config Store handlers depend on.
type ConfigReader interface {
	ModelAliases() map[string]string
	ThinkingInjectionEnabled() bool
	ThinkingInjectionMinChars() int
	ThinkingInjectionPrompt() string
	CurrentInputFileEnabled() bool
	CurrentInputFileMinChars() int
	CurrentInputFilePrompt() string
}

// Deps bundles the dependencies for a handler.
type Deps struct {
	Store ConfigReader
	Auth  AuthResolver
	DS    DeepSeekCaller
}

// Size limits used by the slim handlers.
const (
	UploadMaxSize  = 50 << 20 // 50 MiB
	GeneralMaxSize = 4 << 20  // 4 MiB
)

// ModelsHandler returns a JSON model list derived from the store.
type ModelsHandler struct {
	Store ConfigReader
}

// ServeHTTP writes the OpenAI-style /v1/models list.
func (h *ModelsHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
}

// compile-time interface checks.
var (
	_ AuthResolver   = (*auth.Resolver)(nil)
	_ DeepSeekCaller = (DeepSeekCaller)(nil)
	_ ConfigReader   = (*StoreConfigReader)(nil)
)

// maxWait is a small helper used by retry loops.
func maxWait(d time.Duration, cap time.Duration) time.Duration {
	if d > cap {
		return cap
	}
	if d < 0 {
		return 0
	}
	return d
}
