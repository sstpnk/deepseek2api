package cfworker

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"ds2api/internal/chathistory"
	"ds2api/internal/config"
	adminshared "ds2api/internal/httpapi/admin/shared"
)

//go:embed cf-worker.js
var workerScript []byte

type Handler struct {
	Store       adminshared.ConfigStore
	Pool        adminshared.PoolController
	DS          adminshared.DeepSeekCaller
	OpenAI      adminshared.OpenAIChatCaller
	ChatHistory *chathistory.Store
}

const cfAPIBase = "https://api.cloudflare.com/client/v4"

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) deploy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIToken   string `json:"api_token"`
		AccountID  string `json:"account_id"`
		WorkerName string `json:"worker_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	req.APIToken = strings.TrimSpace(req.APIToken)
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.WorkerName = strings.TrimSpace(req.WorkerName)
	if req.APIToken == "" || req.AccountID == "" || req.WorkerName == "" {
		writeJSON(w, 400, map[string]string{"error": "api_token, account_id, worker_name required"})
		return
	}

	putReq, _ := http.NewRequest(http.MethodPut,
		cfAPIBase+"/accounts/"+req.AccountID+"/workers/scripts/"+req.WorkerName,
		bytes.NewReader(workerScript))
	putReq.Header.Set("Authorization", "Bearer "+req.APIToken)
	putReq.Header.Set("Content-Type", "application/javascript+module")

	resp, err := (&http.Client{}).Do(putReq)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "CF API unreachable: " + err.Error()})
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var cfResp struct {
		Success bool `json:"success"`
		Errors  []struct{ Message string `json:"message"` } `json:"errors"`
	}
	json.Unmarshal(body, &cfResp)
	if !cfResp.Success {
		msg := "CF API error"
		if len(cfResp.Errors) > 0 {
			msg = cfResp.Errors[0].Message
		}
		writeJSON(w, 502, map[string]string{"error": msg})
		return
	}

	workerHost := req.WorkerName + ".workers.dev"
	if s, _ := getSubdomain(req.APIToken, req.AccountID); s != "" {
		workerHost = req.WorkerName + "." + s
	}

	proxy := config.NormalizeProxy(config.Proxy{
		Type: "cloudflare", Host: workerHost, Port: 443,
		WorkerHost: workerHost, Name: "CF Worker: " + req.WorkerName,
	})

	snap := h.Store.Snapshot()
	proxies := snap.Proxies
	found := false
	for i, p := range proxies {
		if p.ID == proxy.ID {
			proxies[i] = proxy
			found = true
			break
		}
	}
	if !found {
		proxies = append(proxies, proxy)
	}
	finalProxies := proxies
	if err := h.Store.Update(func(c *config.Config) error {
		c.Proxies = finalProxies
		return nil
	}); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to save proxy"})
		return
	}

	writeJSON(w, 200, map[string]any{
		"worker_host": workerHost, "proxy_id": proxy.ID, "deployed": true,
	})
}

func getSubdomain(apiToken, accountID string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet,
		cfAPIBase+"/accounts/"+accountID+"/workers/subdomain", nil)
	req.Header.Set("Authorization", "Bearer "+apiToken)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var cfResp struct {
		Success bool `json:"success"`
		Result  struct{ Subdomain string `json:"subdomain"` } `json:"result"`
	}
	json.Unmarshal(body, &cfResp)
	if cfResp.Result.Subdomain != "" {
		return cfResp.Result.Subdomain + ".workers.dev", nil
	}
	return "", nil
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	workerName := strings.TrimSpace(r.URL.Query().Get("worker_name"))
	if workerName == "" {
		writeJSON(w, 400, map[string]string{"error": "worker_name required"})
		return
	}
	snap := h.Store.Snapshot()
	inUse := 0
	for _, acc := range snap.Accounts {
		for _, p := range snap.Proxies {
			if p.Type == "cloudflare" && strings.Contains(p.WorkerHost, workerName) && acc.ProxyID == p.ID {
				inUse++
			}
		}
	}
	for _, p := range snap.Proxies {
		if p.Type == "cloudflare" && strings.Contains(p.WorkerHost, workerName) {
			writeJSON(w, 200, map[string]any{
				"deployed": true, "worker_host": p.WorkerHost,
				"proxy_id": p.ID, "proxy_in_use": inUse,
			})
			return
		}
	}
	writeJSON(w, 200, map[string]any{"deployed": false})
}

func (h *Handler) deleteWorker(w http.ResponseWriter, r *http.Request) {
	apiToken := strings.TrimSpace(r.Header.Get("X-CF-API-Token"))
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	workerName := strings.TrimSpace(r.URL.Query().Get("worker_name"))
	if apiToken == "" || accountID == "" || workerName == "" {
		writeJSON(w, 400, map[string]string{"error": "X-CF-API-Token, account_id, worker_name required"})
		return
	}
	delReq, _ := http.NewRequest(http.MethodDelete,
		cfAPIBase+"/accounts/"+accountID+"/workers/scripts/"+workerName, nil)
	delReq.Header.Set("Authorization", "Bearer "+apiToken)
	resp, _ := (&http.Client{}).Do(delReq)
	if resp != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	snap := h.Store.Snapshot()
	filtered := make([]config.Proxy, 0, len(snap.Proxies))
	for _, p := range snap.Proxies {
		if p.Type == "cloudflare" && strings.Contains(p.WorkerHost, workerName) {
			continue
		}
		filtered = append(filtered, p)
	}
	_ = h.Store.Update(func(c *config.Config) error {
		c.Proxies = filtered
		return nil
	})
	writeJSON(w, 200, map[string]any{"deleted": true})
}
