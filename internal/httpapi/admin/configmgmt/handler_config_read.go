package configmgmt

import (
	"net/http"
	"strings"

	"ds2api/internal/config"
)

func (h *Handler) getConfig(w http.ResponseWriter, _ *http.Request) {
	accountCount := h.Store.RuntimeAccountCount()
	vercelConfig := h.Store.VercelConfig()
	snap := h.Store.Snapshot()
	safe := map[string]any{
		"keys":                  h.Store.Keys(),
		"api_keys":              h.Store.APIKeys(),
		"accounts":              []map[string]any{},
		"account_total":         accountCount,
		"proxies":               []map[string]any{},
		"env_backed":            h.Store.IsEnvBacked(),
		"env_source_present":    h.Store.HasEnvConfigSource(),
		"env_writeback_enabled": h.Store.IsEnvWritebackEnabled(),
		"config_path":           h.Store.ConfigPath(),
		"model_aliases":         h.Store.ConfiguredModelAliases(),
		"proxy_switch_status":   proxySwitchStatus(snap),
		"vercel": map[string]any{
			"has_token":     strings.TrimSpace(vercelConfig.Token) != "",
			"token_preview": maskSecretPreview(vercelConfig.Token),
			"project_id":    vercelConfig.ProjectID,
			"team_id":       vercelConfig.TeamID,
		},
	}
	if accountCount <= 200 {
		storeAccounts := h.Store.Accounts()
		accountItems := make([]map[string]any, 0, accountCount)
		for _, acc := range storeAccounts {
			token := strings.TrimSpace(acc.Token)
			accountItems = append(accountItems, map[string]any{
				"identifier":    acc.Identifier(),
				"name":          acc.Name,
				"remark":        acc.Remark,
				"email":         acc.Email,
				"mobile":        acc.Mobile,
				"proxy_id":      acc.ProxyID,
				"has_password":  strings.TrimSpace(acc.Password) != "",
				"has_token":     token != "",
				"token_preview": maskSecretPreview(token),
			})
		}
		safe["accounts"] = accountItems
	}
	storeProxies := h.Store.Proxies()
	proxies := make([]map[string]any, 0, len(storeProxies))
	for _, proxy := range storeProxies {
		proxy = config.NormalizeProxy(proxy)
		proxies = append(proxies, map[string]any{
			"id":           proxy.ID,
			"name":         proxy.Name,
			"type":         proxy.Type,
			"host":         proxy.Host,
			"port":         proxy.Port,
			"username":     proxy.Username,
			"worker_host":  proxy.WorkerHost,
			"has_password": strings.TrimSpace(proxy.Password) != "",
		})
	}
	safe["proxies"] = proxies
	writeJSON(w, http.StatusOK, safe)
}

func (h *Handler) exportConfig(w http.ResponseWriter, _ *http.Request) {
	h.configExport(w, nil)
}

func (h *Handler) configExport(w http.ResponseWriter, _ *http.Request) {
	snap := h.Store.Snapshot()
	jsonStr, b64, err := h.Store.ExportJSONAndBase64()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"config":  snap,
		"json":    jsonStr,
		"base64":  b64,
	})
}
