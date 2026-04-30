package accounts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"ds2api/internal/config"
)

// bulkImportAccounts accepts a list of `email:password` lines (or an explicit
// accounts array) and creates ds2api accounts in one transaction. When
// auto_proxy.enabled is true, each new account is bound to a per-account proxy
// derived from the supplied template (host/port/credentials common to the
// pool, username unique per account local-part — typical Resin sticky-session
// pattern).
func (h *Handler) bulkImportAccounts(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lines     string           `json:"lines"`
		Accounts  []map[string]any `json:"accounts"`
		AutoProxy struct {
			Enabled          bool   `json:"enabled"`
			Type             string `json:"type"`
			Host             string `json:"host"`
			Port             int    `json:"port"`
			UsernameTemplate string `json:"username_template"`
			Password         string `json:"password"`
			NameTemplate     string `json:"name_template"`
		} `json:"auto_proxy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "无效的 JSON 格式"})
		return
	}

	entries := parseBulkImportLines(req.Lines)
	for _, m := range req.Accounts {
		email := strings.TrimSpace(fieldString(m, "email"))
		password := strings.TrimSpace(fieldString(m, "password"))
		if email == "" || password == "" {
			continue
		}
		entries = append(entries, bulkImportEntry{Email: email, Password: password, Source: email})
	}
	if len(entries) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "未解析到任何有效账号(格式:email:password 一行一个)"})
		return
	}

	tpl := req.AutoProxy
	if tpl.Type == "" {
		tpl.Type = "socks5"
	}
	if tpl.UsernameTemplate == "" {
		tpl.UsernameTemplate = "Default.{local}"
	}
	if tpl.NameTemplate == "" {
		tpl.NameTemplate = "resin-{local}"
	}
	if tpl.Enabled {
		if strings.TrimSpace(tpl.Host) == "" || tpl.Port <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "auto_proxy.enabled 时必须提供 host 与 port"})
			return
		}
	}

	importedAccounts := 0
	importedProxies := 0
	skipped := []map[string]string{}
	errs := []map[string]string{}

	err := h.Store.Update(func(c *config.Config) error {
		existingEmails := make(map[string]bool, len(c.Accounts))
		for _, a := range c.Accounts {
			email := strings.ToLower(strings.TrimSpace(a.Email))
			if email != "" {
				existingEmails[email] = true
			}
		}
		existingProxyIDs := make(map[string]bool, len(c.Proxies))
		existingProxyByName := make(map[string]string, len(c.Proxies))
		for _, p := range c.Proxies {
			p = config.NormalizeProxy(p)
			existingProxyIDs[p.ID] = true
			if p.Name != "" {
				existingProxyByName[p.Name] = p.ID
			}
		}

		for _, e := range entries {
			emailKey := strings.ToLower(e.Email)
			if existingEmails[emailKey] {
				skipped = append(skipped, map[string]string{"email": e.Email, "reason": "邮箱已存在"})
				continue
			}
			local := emailLocalPart(e.Email)
			if local == "" {
				errs = append(errs, map[string]string{"line": e.Source, "error": "无法解析邮箱本地部分"})
				continue
			}

			var proxyID string
			if tpl.Enabled {
				proxyName := strings.ReplaceAll(tpl.NameTemplate, "{local}", local)
				if existingID, ok := existingProxyByName[proxyName]; ok {
					proxyID = existingID
				} else {
					p := config.NormalizeProxy(config.Proxy{
						Name:     proxyName,
						Type:     tpl.Type,
						Host:     strings.TrimSpace(tpl.Host),
						Port:     tpl.Port,
						Username: strings.ReplaceAll(tpl.UsernameTemplate, "{local}", local),
						Password: tpl.Password,
					})
					if existingProxyIDs[p.ID] {
						proxyID = p.ID
					} else {
						c.Proxies = append(c.Proxies, p)
						existingProxyIDs[p.ID] = true
						existingProxyByName[p.Name] = p.ID
						proxyID = p.ID
						importedProxies++
					}
				}
			}

			c.Accounts = append(c.Accounts, config.Account{
				Email:    e.Email,
				Password: e.Password,
				ProxyID:  proxyID,
			})
			existingEmails[emailKey] = true
			importedAccounts++
		}

		if err := config.ValidateProxyConfig(c.Proxies); err != nil {
			return fmt.Errorf("代理配置校验失败:%w", err)
		}
		if err := config.ValidateAccountProxyReferences(c.Accounts, c.Proxies); err != nil {
			return fmt.Errorf("账号代理引用校验失败:%w", err)
		}
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}

	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"imported_accounts": importedAccounts,
		"imported_proxies":  importedProxies,
		"skipped":           skipped,
		"errors":            errs,
	})
}

type bulkImportEntry struct {
	Email    string
	Password string
	Source   string
}

func parseBulkImportLines(blob string) []bulkImportEntry {
	if strings.TrimSpace(blob) == "" {
		return nil
	}
	out := make([]bulkImportEntry, 0)
	for _, raw := range strings.Split(blob, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Accept colon, tab, or whitespace runs as the separator. Use the
		// first colon to allow passwords that contain colons (rare but
		// possible) — split only once.
		var email, password string
		if idx := strings.Index(line, ":"); idx > 0 {
			email = strings.TrimSpace(line[:idx])
			password = strings.TrimSpace(line[idx+1:])
		} else if idx := strings.Index(line, "\t"); idx > 0 {
			email = strings.TrimSpace(line[:idx])
			password = strings.TrimSpace(line[idx+1:])
		} else if fields := strings.Fields(line); len(fields) >= 2 {
			email = fields[0]
			password = strings.Join(fields[1:], " ")
		} else {
			continue
		}
		if email == "" || password == "" {
			continue
		}
		out = append(out, bulkImportEntry{Email: email, Password: password, Source: line})
	}
	return out
}

func emailLocalPart(email string) string {
	email = strings.TrimSpace(email)
	at := strings.Index(email, "@")
	if at <= 0 {
		return ""
	}
	return strings.TrimSpace(email[:at])
}
