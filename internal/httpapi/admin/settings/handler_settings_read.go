package settings

import (
	"net/http"
	"strings"

	authn "ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
)

func (h *Handler) getSettings(w http.ResponseWriter, _ *http.Request) {
	recommended := defaultRuntimeRecommended(h.Store.RuntimeAccountCount(), h.Store.RuntimeAccountMaxInflight())
	needsSync := false
	if config.IsVercel() {
		snap := h.Store.Snapshot()
		needsSync = snap.VercelSyncHash != "" && snap.VercelSyncHash != h.computeSyncHash()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"admin": map[string]any{
			"has_password_hash":        strings.TrimSpace(h.Store.AdminPasswordHash()) != "",
			"jwt_expire_hours":         h.Store.AdminJWTExpireHours(),
			"jwt_valid_after_unix":     h.Store.AdminJWTValidAfterUnix(),
			"default_password_warning": authn.UsingDefaultAdminKey(h.Store),
		},
		"runtime": map[string]any{
			"account_max_inflight":         h.Store.RuntimeAccountMaxInflight(),
			"account_max_queue":            h.Store.RuntimeAccountMaxQueue(recommended),
			"global_max_inflight":          h.Store.RuntimeGlobalMaxInflight(recommended),
			"pow_max_concurrency":          h.Store.RuntimePowMaxConcurrency(),
			"token_refresh_interval_hours": h.Store.RuntimeTokenRefreshIntervalHours(),
		},
		"responses":   h.Store.ResponsesConfig(),
		"embeddings":  h.Store.EmbeddingsConfig(),
		"auto_delete": h.Store.AutoDeleteConfig(),
		"current_input_file": map[string]any{
			"enabled":   h.Store.CurrentInputFileEnabled(),
			"min_chars": h.Store.CurrentInputFileMinChars(),
		},
		"thinking_injection": map[string]any{
			"enabled":        h.Store.ThinkingInjectionEnabled(),
			"prompt":         h.Store.ThinkingInjectionPrompt(),
			"default_prompt": promptcompat.DefaultThinkingInjectionPrompt,
		},
		"model_aliases":     h.Store.ConfiguredModelAliases(),
		"env_backed":        h.Store.IsEnvBacked(),
		"needs_vercel_sync": needsSync,
	})
}
