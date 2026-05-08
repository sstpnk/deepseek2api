package cfworker

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

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

const workerModuleName = "cf-worker.js"

var (
	cfAPIBase    = "https://api.cloudflare.com/client/v4"
	cfHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

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

	config.Logger.Info("[cf_worker] deploy requested", "account_id", req.AccountID, "worker_name", req.WorkerName)

	body, contentType, err := buildWorkerUploadBody(workerScript)
	if err != nil {
		config.Logger.Warn("[cf_worker] build upload body failed", "worker_name", req.WorkerName, "error", err)
		writeJSON(w, 500, map[string]string{"error": "failed to build worker upload"})
		return
	}

	putReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPut,
		cfWorkerScriptURL(req.AccountID, req.WorkerName),
		bytes.NewReader(body))
	putReq.Header.Set("Authorization", "Bearer "+req.APIToken)
	putReq.Header.Set("Content-Type", contentType)
	putReq.Header.Set("CF-WORKER-MAIN-MODULE-PART", workerModuleName)

	resp, err := cfHTTPClient.Do(putReq)
	if err != nil {
		config.Logger.Warn("[cf_worker] upload request failed", "account_id", req.AccountID, "worker_name", req.WorkerName, "error", err)
		writeJSON(w, 502, map[string]string{"error": "CF API unreachable: " + err.Error()})
		return
	}
	respBody, readErr := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		config.Logger.Warn("[cf_worker] upload response close failed", "worker_name", req.WorkerName, "error", closeErr)
	}
	if readErr != nil {
		config.Logger.Warn("[cf_worker] upload response read failed", "worker_name", req.WorkerName, "error", readErr)
		writeJSON(w, 502, map[string]string{"error": "failed to read CF API response"})
		return
	}

	cfResp := parseCFResponse(respBody)
	if !cfResp.Success {
		msg := cfResp.messageOr("CF API error")
		config.Logger.Warn("[cf_worker] upload failed",
			"account_id", req.AccountID,
			"worker_name", req.WorkerName,
			"status", resp.StatusCode,
			"cf_error", msg,
			"cf_errors", cfResp.errorSummary(),
			"body", truncateForLog(string(respBody), 1000),
		)
		writeJSON(w, 502, map[string]string{"error": msg})
		return
	}
	config.Logger.Info("[cf_worker] upload succeeded", "account_id", req.AccountID, "worker_name", req.WorkerName, "status", resp.StatusCode)

	if err := enableWorkerSubdomain(r.Context(), req.APIToken, req.AccountID, req.WorkerName); err != nil {
		config.Logger.Warn("[cf_worker] enable workers.dev subdomain failed", "account_id", req.AccountID, "worker_name", req.WorkerName, "error", err)
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}

	accountWorkersDevHost, err := getSubdomain(req.APIToken, req.AccountID)
	if err != nil {
		config.Logger.Warn("[cf_worker] get account workers.dev subdomain failed", "account_id", req.AccountID, "worker_name", req.WorkerName, "error", err)
		writeJSON(w, 502, map[string]string{"error": "failed to resolve workers.dev subdomain: " + err.Error()})
		return
	}
	if accountWorkersDevHost == "" {
		config.Logger.Warn("[cf_worker] account workers.dev subdomain is empty", "account_id", req.AccountID, "worker_name", req.WorkerName)
		writeJSON(w, 502, map[string]string{"error": "failed to resolve workers.dev subdomain"})
		return
	}
	workerHost := req.WorkerName + "." + accountWorkersDevHost

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
		config.Logger.Warn("[cf_worker] save proxy failed", "worker_name", req.WorkerName, "worker_host", workerHost, "error", err)
		writeJSON(w, 500, map[string]string{"error": "failed to save proxy"})
		return
	}

	config.Logger.Info("[cf_worker] deploy completed", "worker_name", req.WorkerName, "worker_host", workerHost, "proxy_id", proxy.ID)

	writeJSON(w, 200, map[string]any{
		"worker_host": workerHost, "proxy_id": proxy.ID, "deployed": true,
	})
}

func buildWorkerUploadBody(script []byte) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	closed := false
	defer func() {
		if !closed {
			if closeErr := writer.Close(); closeErr != nil {
				config.Logger.Warn("[cf_worker] multipart writer close failed", "error", closeErr)
			}
		}
	}()

	meta, err := json.Marshal(map[string]any{
		"main_module":        workerModuleName,
		"compatibility_date": "2025-01-01",
	})
	if err != nil {
		return nil, "", err
	}
	metaHeader := make(textproto.MIMEHeader)
	metaHeader.Set("Content-Disposition", `form-data; name="metadata"`)
	metaHeader.Set("Content-Type", "application/json")
	metaPart, err := writer.CreatePart(metaHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := metaPart.Write(meta); err != nil {
		return nil, "", err
	}

	scriptHeader := make(textproto.MIMEHeader)
	scriptHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, workerModuleName, workerModuleName))
	scriptHeader.Set("Content-Type", "application/javascript+module")
	scriptPart, err := writer.CreatePart(scriptHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := scriptPart.Write(script); err != nil {
		return nil, "", err
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	closed = true
	return body.Bytes(), writer.FormDataContentType(), nil
}

func cfAccountURL(accountID, suffix string) string {
	return cfAPIBase + "/accounts/" + url.PathEscape(accountID) + suffix
}

func cfWorkerScriptURL(accountID, workerName string) string {
	return cfAccountURL(accountID, "/workers/scripts/"+url.PathEscape(workerName))
}

type cfAPIResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Messages []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"messages"`
}

func parseCFResponse(body []byte) cfAPIResponse {
	var cfResp cfAPIResponse
	_ = json.Unmarshal(body, &cfResp)
	return cfResp
}

func (r cfAPIResponse) messageOr(fallback string) string {
	if len(r.Errors) > 0 && strings.TrimSpace(r.Errors[0].Message) != "" {
		return strings.TrimSpace(r.Errors[0].Message)
	}
	if len(r.Messages) > 0 && strings.TrimSpace(r.Messages[0].Message) != "" {
		return strings.TrimSpace(r.Messages[0].Message)
	}
	return fallback
}

func (r cfAPIResponse) errorSummary() string {
	if len(r.Errors) == 0 {
		return ""
	}
	parts := make([]string, 0, len(r.Errors))
	for _, item := range r.Errors {
		msg := strings.TrimSpace(item.Message)
		if item.Code != 0 {
			parts = append(parts, fmt.Sprintf("%d:%s", item.Code, msg))
		} else {
			parts = append(parts, msg)
		}
	}
	return strings.Join(parts, " | ")
}

func enableWorkerSubdomain(ctx context.Context, apiToken, accountID, workerName string) error {
	body := bytes.NewReader([]byte(`{"enabled":true}`))
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		cfWorkerScriptURL(accountID, workerName)+"/subdomain", body)
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := cfHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("CF API unreachable while enabling workers.dev: %w", err)
	}
	respBody, readErr := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		config.Logger.Warn("[cf_worker] subdomain response close failed", "worker_name", workerName, "error", closeErr)
	}
	if readErr != nil {
		return fmt.Errorf("failed to read CF workers.dev response: %w", readErr)
	}
	cfResp := parseCFResponse(respBody)
	if !cfResp.Success {
		msg := cfResp.messageOr("failed to enable workers.dev subdomain")
		config.Logger.Warn("[cf_worker] workers.dev enable failed",
			"account_id", accountID,
			"worker_name", workerName,
			"status", resp.StatusCode,
			"cf_error", msg,
			"cf_errors", cfResp.errorSummary(),
			"body", truncateForLog(string(respBody), 1000),
		)
		return fmt.Errorf("failed to enable workers.dev subdomain: %s", msg)
	}
	config.Logger.Info("[cf_worker] workers.dev subdomain enabled", "account_id", accountID, "worker_name", workerName, "status", resp.StatusCode)
	return nil
}

func truncateForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func getSubdomain(apiToken, accountID string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet,
		cfAccountURL(accountID, "/workers/subdomain"), nil)
	req.Header.Set("Authorization", "Bearer "+apiToken)
	resp, err := cfHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	body, readErr := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		config.Logger.Warn("[cf_worker] subdomain response close failed", "account_id", accountID, "error", closeErr)
	}
	if readErr != nil {
		return "", readErr
	}
	var cfResp struct {
		Success bool `json:"success"`
		Result  struct {
			Subdomain string `json:"subdomain"`
		} `json:"result"`
	}
	json.Unmarshal(body, &cfResp)
	if !cfResp.Success {
		apiResp := parseCFResponse(body)
		return "", fmt.Errorf("%s", apiResp.messageOr("CF API error while reading workers.dev subdomain"))
	}
	if cfResp.Result.Subdomain != "" {
		subdomain := strings.TrimSpace(cfResp.Result.Subdomain)
		if strings.HasSuffix(subdomain, ".workers.dev") {
			return subdomain, nil
		}
		return subdomain + ".workers.dev", nil
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
		cfWorkerScriptURL(accountID, workerName), nil)
	delReq.Header.Set("Authorization", "Bearer "+apiToken)
	resp, err := cfHTTPClient.Do(delReq)
	if err != nil {
		config.Logger.Warn("[cf_worker] delete request failed", "account_id", accountID, "worker_name", workerName, "error", err)
	}
	if resp != nil {
		if _, copyErr := io.Copy(io.Discard, resp.Body); copyErr != nil {
			config.Logger.Warn("[cf_worker] delete response drain failed", "worker_name", workerName, "error", copyErr)
		}
		if closeErr := resp.Body.Close(); closeErr != nil {
			config.Logger.Warn("[cf_worker] delete response close failed", "worker_name", workerName, "error", closeErr)
		}
	}

	snap := h.Store.Snapshot()
	filtered := make([]config.Proxy, 0, len(snap.Proxies))
	for _, p := range snap.Proxies {
		if p.Type == "cloudflare" && strings.Contains(p.WorkerHost, workerName) {
			continue
		}
		filtered = append(filtered, p)
	}
	if err := h.Store.Update(func(c *config.Config) error {
		c.Proxies = filtered
		return nil
	}); err != nil {
		config.Logger.Warn("[cf_worker] remove local proxy failed", "worker_name", workerName, "error", err)
		writeJSON(w, 500, map[string]string{"error": "failed to remove local proxy"})
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": true})
}
