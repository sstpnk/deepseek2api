package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	authn "ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/prompt"
	"ds2api/internal/promptcompat"
	"ds2api/internal/sse"
)

type modelAliasSnapshotReader struct {
	aliases map[string]string
}

func (m modelAliasSnapshotReader) ModelAliases() map[string]string {
	return m.aliases
}

func (h *Handler) testSingleAccount(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	identifier, _ := req["identifier"].(string)
	if strings.TrimSpace(identifier) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "需要账号标识（identifier / email / mobile）"})
		return
	}
	acc, ok := findAccountByIdentifier(h.Store, identifier)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": "账号不存在"})
		return
	}
	model, _ := req["model"].(string)
	if model == "" {
		model = "deepseek-v4-flash"
	}
	message, _ := req["message"].(string)
	autoDelete, _ := req["auto_delete"].(bool)
	result := h.testAccount(r.Context(), acc, model, message)
	if autoDelete && h.maybeQuarantineFailedAccount(r.Context(), acc, result) {
		// Quarantining is a removal from the pool — refresh the queue once
		// per request, same contract as direct deletion used to have.
		h.Pool.Reset()
	}
	writeJSON(w, http.StatusOK, result)
}

// maybeQuarantineFailedAccount inspects a failed test result and, when the
// caller opted into auto-deletion, runs a two-strike Login verification. If
// both probes fail the account is moved into the quarantine list (NOT deleted
// outright) — the background sweeper will give it three more chances over six
// hours before the actual delete. Mutates result in place to record:
//   - result["verified_unusable"] = true   if both verification logins failed
//   - result["quarantined"]       = true   if the account was moved
//   - result["quarantine_error"]  = "..."  if quarantine bookkeeping failed
//
// Returns true when an account was actually moved. The caller is responsible
// for calling Pool.Reset() once after any batch of moves so the pool's queue
// reflects the smaller account set; see testAllAccounts and testSingleAccount
// for the two call sites.
//
// Only call this when result["success"] is false; success accounts are not
// touched even if auto_delete is enabled.
func (h *Handler) maybeQuarantineFailedAccount(ctx context.Context, acc config.Account, result map[string]any) bool {
	if ok, _ := result["success"].(bool); ok {
		return false
	}
	verifyErr := h.verifyAccountUnusable(ctx, acc)
	if verifyErr == "" {
		return false
	}
	result["verified_unusable"] = true
	reason, _ := result["message"].(string)
	if err := h.quarantineAccountByIdentifier(acc.Identifier(), verifyErr, reason); err != nil {
		result["quarantine_error"] = err.Error()
		return false
	}
	result["quarantined"] = true
	return true
}

// verifyAccountUnusable performs two consecutive Login attempts against the
// upstream. Both must fail for the account to be considered confirmed-broken.
// This filters most transient network blips: a single failed attempt is not
// enough to trigger quarantine. Returns the joined error string when both
// fail (empty string means at least one Login succeeded, account is fine).
//
// The two attempts go through the same per-account proxy as the regular
// login path, so a flaky proxy will register as a failure — the user opts
// into this risk by enabling auto_delete. The quarantine sweeper gives the
// proxy three more chances at two-hour intervals before the real delete.
func (h *Handler) verifyAccountUnusable(ctx context.Context, acc config.Account) string {
	if _, err := h.DS.Login(ctx, acc); err == nil {
		return ""
	} else {
		first := err.Error()
		if _, err2 := h.DS.Login(ctx, acc); err2 == nil {
			return ""
		} else {
			return joinErrors(first, err2.Error())
		}
	}
}

// quarantineAccountByIdentifier moves a single account from the active list
// into the quarantine list under the store mutex. Mirrors
// deleteAccountByIdentifierInternal in shape — the caller is responsible for
// Pool.Reset() once per batch.
func (h *Handler) quarantineAccountByIdentifier(identifier, lastError, reason string) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return fmt.Errorf("identifier 为空")
	}
	return h.Store.Update(func(c *config.Config) error {
		return quarantineAccountLocked(c, identifier, lastError, reason)
	})
}

func (h *Handler) testAllAccounts(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	model, _ := req["model"].(string)
	if model == "" {
		model = "deepseek-v4-flash"
	}
	autoDelete, _ := req["auto_delete"].(bool)
	concurrency := clampConcurrency(intFromRequest(req, "concurrency", defaultRefreshConcurrency))

	accounts := h.Store.Snapshot().Accounts
	if len(accounts) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"total": 0, "success": 0, "failed": 0, "quarantined": 0, "deleted": 0, "results": []any{}, "quarantined_accounts": []any{}, "deleted_accounts": []any{}})
		return
	}

	results := runAccountTestsConcurrently(accounts, concurrency, func(_ int, account config.Account) map[string]any {
		res := h.testAccount(r.Context(), account, model, "")
		if autoDelete {
			h.maybeQuarantineFailedAccount(r.Context(), account, res)
		}
		return res
	})

	success := 0
	quarantinedIDs := make([]string, 0)
	for _, res := range results {
		if ok, _ := res["success"].(bool); ok {
			success++
		}
		if q, _ := res["quarantined"].(bool); q {
			id, _ := res["account"].(string)
			if id != "" {
				quarantinedIDs = append(quarantinedIDs, id)
			}
		}
	}
	// Single Pool.Reset for the whole batch — the per-test goroutines mutate
	// store.Accounts via quarantineAccountByIdentifier under the store mutex,
	// but Pool.queue is rebuilt only here so live traffic doesn't see the
	// queue churned 1×N times for a refresh-all run.
	if len(quarantinedIDs) > 0 {
		h.Pool.Reset()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":                len(accounts),
		"success":              success,
		"failed":               len(accounts) - success,
		"quarantined":          len(quarantinedIDs),
		"quarantined_accounts": quarantinedIDs,
		// Kept for backwards-compatible API consumers; quarantine *replaces*
		// the immediate-delete path so this number is always 0 for the
		// /test-all endpoint now. Confirmed deletion happens only from the
		// background sweeper after three failed sweeps.
		"deleted":          0,
		"deleted_accounts": []any{},
		"concurrency":      concurrency,
		"auto_delete":      autoDelete,
		"results":          results,
	})
}

// Concurrency bounds for the bulk refresh path. Capped to keep upstream load
// manageable and to avoid pathological goroutine counts on huge account lists.
const (
	defaultRefreshConcurrency = 10
	minRefreshConcurrency     = 1
	maxRefreshConcurrency     = 20
)

func clampConcurrency(n int) int {
	if n < minRefreshConcurrency {
		return minRefreshConcurrency
	}
	if n > maxRefreshConcurrency {
		return maxRefreshConcurrency
	}
	return n
}

// intFromRequest reads an integer from a JSON-decoded map. JSON numbers come
// back as float64; we accept that plus a few common alternatives so the
// frontend can send either a number or a string.
func intFromRequest(req map[string]any, key string, fallback int) int {
	if req == nil {
		return fallback
	}
	v, ok := req[key]
	if !ok || v == nil {
		return fallback
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return fallback
		}
		var n int
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

func runAccountTestsConcurrently(accounts []config.Account, maxConcurrency int, testFn func(int, config.Account) map[string]any) []map[string]any {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	sem := make(chan struct{}, maxConcurrency)
	results := make([]map[string]any, len(accounts))
	var wg sync.WaitGroup
	for i, acc := range accounts {
		wg.Add(1)
		go func(idx int, account config.Account) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release
			results[idx] = testFn(idx, account)
		}(i, acc)
	}
	wg.Wait()
	return results
}

func (h *Handler) testAccount(ctx context.Context, acc config.Account, model, message string) map[string]any {
	start := time.Now()
	identifier := acc.Identifier()
	result := map[string]any{
		"account":         identifier,
		"success":         false,
		"response_time":   0,
		"message":         "",
		"model":           model,
		"session_count":   0,
		"config_writable": !h.Store.IsEnvBacked(),
		"config_warning":  "",
	}
	defer func() {
		status := "failed"
		if ok, _ := result["success"].(bool); ok {
			status = "ok"
		}
		_ = h.Store.UpdateAccountTestStatus(identifier, status)
	}()
	token, err := h.DS.Login(ctx, acc)
	if err != nil {
		result["message"] = "登录失败: " + err.Error()
		return result
	}
	if err := h.Store.UpdateAccountToken(acc.Identifier(), token); err != nil {
		result["config_warning"] = "登录成功，但 token 持久化失败（仅保存在内存，重启后会丢失）: " + err.Error()
	}
	authCtx := &authn.RequestAuth{UseConfigToken: false, DeepSeekToken: token, AccountID: identifier, Account: acc}
	proxyCtx := authn.WithAuth(ctx, authCtx)
	sessionID, err := h.DS.CreateSession(proxyCtx, authCtx, 1)
	if err != nil {
		newToken, loginErr := h.DS.Login(proxyCtx, acc)
		if loginErr != nil {
			result["message"] = "创建会话失败: " + err.Error()
			return result
		}
		token = newToken
		authCtx.DeepSeekToken = token
		if err := h.Store.UpdateAccountToken(acc.Identifier(), token); err != nil {
			result["config_warning"] = "刷新 token 成功，但 token 持久化失败（仅保存在内存，重启后会丢失）: " + err.Error()
		}
		sessionID, err = h.DS.CreateSession(proxyCtx, authCtx, 1)
		if err != nil {
			result["message"] = "创建会话失败: " + err.Error()
			return result
		}
	}

	// 获取会话数量
	sessionStats, sessionErr := h.DS.GetSessionCountForToken(proxyCtx, token)
	if sessionErr == nil && sessionStats != nil {
		result["session_count"] = sessionStats.FirstPageCount
	}

	if strings.TrimSpace(message) == "" {
		result["success"] = true
		result["message"] = "Token 刷新成功（登录与会话创建成功）"
		if warning, _ := result["config_warning"].(string); strings.TrimSpace(warning) != "" {
			result["message"] = result["message"].(string) + "；" + warning
		}
		result["response_time"] = int(time.Since(start).Milliseconds())
		return result
	}
	thinking, search, ok := config.GetModelConfig(model)
	resolvedModel, resolved := config.ResolveModel(modelAliasSnapshotReader{
		aliases: h.Store.Snapshot().ModelAliases,
	}, model)
	if resolved {
		model = resolvedModel
		thinking, search, ok = config.GetModelConfig(model)
	}
	if !ok {
		thinking, search = false, false
	}
	pow, err := h.DS.GetPow(proxyCtx, authCtx, 1)
	if err != nil {
		result["message"] = "获取 PoW 失败: " + err.Error()
		return result
	}
	payload := promptcompat.StandardRequest{
		ResolvedModel: model,
		FinalPrompt:   prompt.MessagesPrepare([]map[string]any{{"role": "user", "content": message}}),
		Thinking:      thinking,
		Search:        search,
	}.CompletionPayload(sessionID)
	resp, err := h.DS.CallCompletion(proxyCtx, authCtx, payload, pow, 1)
	if err != nil {
		result["message"] = "请求失败: " + err.Error()
		return result
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		result["message"] = fmt.Sprintf("请求失败: HTTP %d", resp.StatusCode)
		return result
	}
	collected := sse.CollectStream(resp, thinking, true)
	result["success"] = true
	result["response_time"] = int(time.Since(start).Milliseconds())
	if collected.Text != "" {
		result["message"] = collected.Text
	} else {
		result["message"] = "（无回复内容）"
	}
	if collected.Thinking != "" {
		result["thinking"] = collected.Thinking
	}
	return result
}

func (h *Handler) testAPI(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	model, _ := req["model"].(string)
	message, _ := req["message"].(string)
	apiKey, _ := req["api_key"].(string)
	if model == "" {
		model = "deepseek-v4-flash"
	}
	if message == "" {
		message = "你好"
	}
	if apiKey == "" {
		keys := h.Store.Snapshot().Keys
		if len(keys) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "没有可用的 API Key"})
			return
		}
		apiKey = keys[0]
	}
	host := r.Host
	scheme := "http"
	if strings.Contains(strings.ToLower(host), "vercel") || strings.Contains(strings.ToLower(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	payload := map[string]any{"model": model, "messages": []map[string]any{{"role": "user", "content": message}}, "stream": false}
	b, _ := json.Marshal(payload)
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, fmt.Sprintf("%s://%s/v1/chat/completions", scheme, host), bytes.NewReader(b))
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(request)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": err.Error()})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		var parsed any
		_ = json.Unmarshal(body, &parsed)
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "status_code": resp.StatusCode, "response": parsed})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": false, "status_code": resp.StatusCode, "response": string(body)})
}

func (h *Handler) deleteAllSessions(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	identifier, _ := req["identifier"].(string)
	if strings.TrimSpace(identifier) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "需要账号标识（identifier / email / mobile）"})
		return
	}
	acc, ok := findAccountByIdentifier(h.Store, identifier)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": "账号不存在"})
		return
	}

	// 每次先登录刷新一次 token，避免使用过期 token。
	authCtx := &authn.RequestAuth{UseConfigToken: false, AccountID: acc.Identifier(), Account: acc}
	proxyCtx := authn.WithAuth(r.Context(), authCtx)
	token, err := h.DS.Login(proxyCtx, acc)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": "登录失败: " + err.Error()})
		return
	}
	_ = h.Store.UpdateAccountToken(acc.Identifier(), token)
	authCtx.DeepSeekToken = token

	// 删除所有会话
	err = h.DS.DeleteAllSessionsForToken(proxyCtx, token)
	if err != nil {
		// token 可能过期，尝试重新登录并重试一次
		newToken, loginErr := h.DS.Login(proxyCtx, acc)
		if loginErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": "删除失败: " + err.Error()})
			return
		}
		token = newToken
		_ = h.Store.UpdateAccountToken(acc.Identifier(), token)
		authCtx.DeepSeekToken = token
		if retryErr := h.DS.DeleteAllSessionsForToken(proxyCtx, token); retryErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": "删除失败: " + retryErr.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "删除成功"})
}
