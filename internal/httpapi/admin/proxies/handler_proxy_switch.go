package proxies

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"ds2api/internal/config"
	adminshared "ds2api/internal/httpapi/admin/shared"
)

const proxySwitchModeResin = "resin"
const proxySwitchModeCloudflare = "cloudflare"

func (h *Handler) proxySwitchStatus(w http.ResponseWriter, _ *http.Request) {
	snap := h.Store.Snapshot()
	status := buildProxySwitchStatus(snap)
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) switchProxyMode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode      string `json:"mode"`
		CFProxyID string `json:"cf_proxy_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	targetMode := strings.ToLower(strings.TrimSpace(req.Mode))
	if targetMode == "" || targetMode == "toggle" {
		current := buildProxySwitchStatus(h.Store.Snapshot()).Mode
		if current == proxySwitchModeCloudflare {
			targetMode = proxySwitchModeResin
		} else {
			targetMode = proxySwitchModeCloudflare
		}
	}
	if targetMode != proxySwitchModeResin && targetMode != proxySwitchModeCloudflare {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "mode must be resin or cloudflare"})
		return
	}

	before := h.Store.Snapshot()
	config.Logger.Info("[proxy_switch] requested",
		"target_mode", targetMode,
		"current_mode", buildProxySwitchStatus(before).Mode,
		"requested_cf_proxy_id", strings.TrimSpace(req.CFProxyID),
		"accounts", len(before.Accounts),
	)

	var status proxySwitchStatusPayload
	err := h.Store.Update(func(c *config.Config) error {
		var err error
		switch targetMode {
		case proxySwitchModeCloudflare:
			status, err = applyCloudflareProxyMode(c, req.CFProxyID)
		case proxySwitchModeResin:
			status, err = applyResinProxyMode(c)
		}
		if err != nil {
			return err
		}
		return validateProxyMutation(c)
	})
	if err != nil {
		config.Logger.Warn("[proxy_switch] failed",
			"target_mode", targetMode,
			"requested_cf_proxy_id", strings.TrimSpace(req.CFProxyID),
			"error", err,
		)
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	h.Pool.Reset()
	config.Logger.Info("[proxy_switch] completed",
		"mode", status.Mode,
		"cf_proxy_id", status.CFProxyID,
		"cf_proxy_host", status.CFProxyHost,
		"accounts", status.AccountTotal,
		"cloudflare_bound", status.CloudflareBound,
		"resin_snapshot_count", status.ResinSnapshotCount,
		"missing_resin_restores", status.MissingResinRestores,
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"status":  status,
	})
}

type proxySwitchStatusPayload struct {
	Mode                 string `json:"mode"`
	CFProxyID            string `json:"cf_proxy_id,omitempty"`
	CFProxyName          string `json:"cf_proxy_name,omitempty"`
	CFProxyHost          string `json:"cf_proxy_host,omitempty"`
	CFProxyAvailable     bool   `json:"cf_proxy_available"`
	AccountTotal         int    `json:"account_total"`
	CloudflareBound      int    `json:"cloudflare_bound"`
	ResinSnapshotCount   int    `json:"resin_snapshot_count"`
	ResinTemplateReady   bool   `json:"resin_template_ready"`
	ResinTemplateName    string `json:"resin_template_name,omitempty"`
	ResinTemplateHost    string `json:"resin_template_host,omitempty"`
	ResinTemplatePort    int    `json:"resin_template_port,omitempty"`
	ResinTemplateType    string `json:"resin_template_type,omitempty"`
	ResinTemplateUser    string `json:"resin_template_username,omitempty"`
	MissingResinRestores int    `json:"missing_resin_restores,omitempty"`
}

func buildProxySwitchStatus(c config.Config) proxySwitchStatusPayload {
	raw := adminshared.ProxySwitchStatus(c)
	mode, _ := raw["mode"].(string)

	status := proxySwitchStatusPayload{
		Mode:               mode,
		CFProxyAvailable:   boolFromAny(raw["cf_proxy_available"]),
		AccountTotal:       len(c.Accounts),
		ResinSnapshotCount: len(c.ProxySwitch.ResinAssignments),
		ResinTemplateReady: adminshared.ResinTemplateReady(c.ProxySwitch.ResinProxyTemplate),
	}
	status.CFProxyID, _ = raw["cf_proxy_id"].(string)
	status.CFProxyName, _ = raw["cf_proxy_name"].(string)
	status.CFProxyHost, _ = raw["cf_proxy_host"].(string)
	status.CloudflareBound = intFromAny(raw["cloudflare_bound"])
	tpl := config.NormalizeProxy(c.ProxySwitch.ResinProxyTemplate)
	if status.ResinTemplateReady {
		status.ResinTemplateName = tpl.Name
		status.ResinTemplateHost = tpl.Host
		status.ResinTemplatePort = tpl.Port
		status.ResinTemplateType = tpl.Type
		status.ResinTemplateUser = tpl.Username
	}
	return status
}

func applyCloudflareProxyMode(c *config.Config, requestedCFProxyID string) (proxySwitchStatusPayload, error) {
	cfProxy, ok := adminshared.ResolveCloudflareProxy(*c, requestedCFProxyID)
	if !ok {
		return proxySwitchStatusPayload{}, fmt.Errorf("未找到可用的 Cloudflare 代理，请先部署 CF Worker")
	}
	currentMode := adminshared.ProxySwitchMode(*c)
	assignments := make(map[string]string, len(c.Accounts))
	if currentMode == proxySwitchModeCloudflare {
		assignments = cloneStringMap(c.ProxySwitch.ResinAssignments)
		if assignments == nil {
			assignments = make(map[string]string, len(c.Accounts))
		}
	}
	for _, acc := range c.Accounts {
		id := acc.Identifier()
		if id == "" {
			continue
		}
		if currentMode != proxySwitchModeCloudflare {
			assignments[id] = strings.TrimSpace(acc.ProxyID)
		} else if _, exists := assignments[id]; !exists {
			assignments[id] = strings.TrimSpace(acc.ProxyID)
		}
		if shouldUseResinTemplate(c.ProxySwitch.ResinProxyTemplate, acc.ProxyID, c.Proxies) {
			c.ProxySwitch.ResinProxyTemplate = proxyTemplateForID(acc.ProxyID, c.Proxies)
		}
	}
	if !adminshared.ResinTemplateReady(c.ProxySwitch.ResinProxyTemplate) {
		if tpl, ok := inferResinTemplate(c.Proxies); ok {
			c.ProxySwitch.ResinProxyTemplate = tpl
		}
	}
	for i := range c.Accounts {
		c.Accounts[i].ProxyID = cfProxy.ID
	}
	c.ProxySwitch.Mode = proxySwitchModeCloudflare
	c.ProxySwitch.CFProxyID = cfProxy.ID
	c.ProxySwitch.ResinAssignments = assignments
	return buildProxySwitchStatus(*c), nil
}

func applyResinProxyMode(c *config.Config) (proxySwitchStatusPayload, error) {
	assignments := cloneStringMap(c.ProxySwitch.ResinAssignments)
	if assignments == nil {
		assignments = map[string]string{}
	}
	proxyIDs := proxyIDSet(c.Proxies)
	missingRestores := 0
	for i, acc := range c.Accounts {
		id := acc.Identifier()
		proxyID, hasSnapshot := assignments[id]
		proxyID = strings.TrimSpace(proxyID)
		if hasSnapshot && proxyID != "" && proxyIDs[proxyID] && !proxyIsCloudflare(proxyID, c.Proxies) {
			c.Accounts[i].ProxyID = proxyID
			if id != "" {
				assignments[id] = proxyID
			}
			continue
		}
		c.Accounts[i].ProxyID = adminshared.EnsureResinProxyForAccount(c, acc)
		if c.Accounts[i].ProxyID == "" {
			missingRestores++
			if id != "" {
				delete(assignments, id)
			}
		} else if id != "" {
			assignments[id] = c.Accounts[i].ProxyID
		}
	}
	c.ProxySwitch.Mode = proxySwitchModeResin
	c.ProxySwitch.ResinAssignments = assignments
	status := buildProxySwitchStatus(*c)
	status.MissingResinRestores = missingRestores
	return status, nil
}

func inferResinTemplate(proxies []config.Proxy) (config.Proxy, bool) {
	for _, proxy := range proxies {
		proxy = config.NormalizeProxy(proxy)
		if proxy.Type != "socks5" && proxy.Type != "socks5h" {
			continue
		}
		if !strings.Contains(strings.ToLower(proxy.Name), "resin") && !strings.Contains(strings.ToLower(proxy.Username), "default.") {
			continue
		}
		proxy.ID = ""
		proxy.Name = resinTemplateName(proxy.Name)
		proxy.Username = resinTemplateUsername(proxy.Username)
		return proxy, true
	}
	return config.Proxy{}, false
}

func shouldUseResinTemplate(current config.Proxy, proxyID string, proxies []config.Proxy) bool {
	if adminshared.ResinTemplateReady(current) || strings.TrimSpace(proxyID) == "" {
		return false
	}
	proxy := proxyTemplateForID(proxyID, proxies)
	return proxy.Host != ""
}

func proxyTemplateForID(proxyID string, proxies []config.Proxy) config.Proxy {
	proxyID = strings.TrimSpace(proxyID)
	for _, proxy := range proxies {
		proxy = config.NormalizeProxy(proxy)
		if proxy.ID != proxyID || (proxy.Type != "socks5" && proxy.Type != "socks5h") {
			continue
		}
		proxy.ID = ""
		proxy.Name = resinTemplateName(proxy.Name)
		proxy.Username = resinTemplateUsername(proxy.Username)
		return proxy
	}
	return config.Proxy{}
}

func resinTemplateName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "resin-{local}"
	}
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		return name[:idx+1] + "{local}"
	}
	return name + "-{local}"
}

func resinTemplateUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	if idx := strings.LastIndex(username, "."); idx > 0 {
		return username[:idx+1] + "{local}"
	}
	return username + ".{local}"
}

func proxyIDSet(proxies []config.Proxy) map[string]bool {
	out := make(map[string]bool, len(proxies))
	for _, proxy := range proxies {
		proxy = config.NormalizeProxy(proxy)
		out[proxy.ID] = true
	}
	return out
}

func proxyIsCloudflare(proxyID string, proxies []config.Proxy) bool {
	proxyID = strings.TrimSpace(proxyID)
	if proxyID == "" {
		return false
	}
	for _, proxy := range proxies {
		proxy = config.NormalizeProxy(proxy)
		if proxy.ID == proxyID {
			return proxy.Type == "cloudflare"
		}
	}
	return false
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func boolFromAny(v any) bool {
	b, _ := v.(bool)
	return b
}

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}
