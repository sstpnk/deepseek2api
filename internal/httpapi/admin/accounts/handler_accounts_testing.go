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
	if autoDelete && h.maybeAutoDeleteFailedAccount(r.Context(), acc, result) {
		h.Pool.Reset()
	}
	writeJSON(w, http.StatusOK, result)
}

// maybeAutoDeleteFailedAccount inspects a failed test result and, when the
// caller opted into auto-deletion, runs a two-strike Login verification before
// removing the account. It mutates result in place to record the outcome:
//   - result["verified_unusable"] = true   if both verification logins failed
//   - result["deleted"] = true             if the account was removed
//   - result["delete_error"] = "..."       if removal failed
//
// Returns true when an account was actually deleted. The caller is
// responsible for calling Pool.Reset() once after any batch of removals so the
// pool's queue reflects the smaller account set; see testAllAccounts and
// testSingleAccount for the two call sites.
//
// Only call this when result["success"] is false; success accounts are not
// touched even if auto_delete is enabled.
func (h *Handler) maybeAutoDeleteFailedAccount(ctx context.Context, acc config.Account, result map[string]any) bool {
	if ok, _ := result["success"].(bool); ok {
		return false
	}
	if !h.verifyAccountUnusable(ctx, acc) {
		return false
	}
	result["verified_unusable"] = true
	if err := h.deleteAccountByIdentifierInternal(acc.Identifier()); err != nil {
		result["delete_error"] = err.Error()
		return false
	}
	result["deleted"] = true
	return true
}

// verifyAccountUnusable performs two consecutive Login attempts against the
// upstream. Both must fail for the account to be considered confirmed-broken.
// This filters most transient network blips: a single failed attempt is not
// enough to trigger deletion. The two attempts go through the same per-account
// proxy as the regular login path, so a flaky proxy will register as a failure
// — the user opts into this risk by enabling auto_delete.
func (h *Handler) verifyAccountUnusable(ctx context.Context, acc config.Account) bool {
	if _, err := h.DS.Login(ctx, acc); err == nil {
		return false
	}
	if _, err := h.DS.Login(ctx, acc); err == nil {
		return false
	}
	return true
}

// deleteAccountByIdentifierInternal removes a single account from the store
// without touching Pool. The caller is responsible for Pool.Reset() once after
// any batch of removals to avoid Reset thrash under concurrent test workers.
func (h *Handler) deleteAccountByIdentifierInternal(identifier string) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return fmt.Errorf("identifier 为空")
	}
	return h.Store.Update(func(c *config.Config) error {
		for i, a := range c.Accounts {
			if accountMatchesIdentifier(a, identifier) {
				c.Accounts = append(c.Accounts[:i], c.Accounts[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("账号不存在: %s", identifier)
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
		writeJSON(w, http.StatusOK, map[string]any{"total": 0, "success": 0, "failed": 0, "deleted": 0, "results": []any{}, "deleted_accounts": []any{}})
		return
	}

	results := runAccountTestsConcurrently(accounts, concurrency, func(_ int, account config.Account) map[string]any {
		res := h.testAccount(r.Context(), account, model, "")
		if autoDelete {
			h.maybeAutoDeleteFailedAccount(r.Context(), account, res)
		}
		return res
	})

	success := 0
	deletedIDs := make([]string, 0)
	for _, res := range results {
		if ok, _ := res["success"].(bool); ok {
			success++
		}
		if dl, _ := res["deleted"].(bool); dl {
			id, _ := res["account"].(string)
			if id != "" {
				deletedIDs = append(deletedIDs, id)
			}
		}
	}
	// Single Pool.Reset for the whole batch — the per-test goroutines mutate
	// store.Accounts via deleteAccountByIdentifierInternal under the store
	// mutex, but Pool.queue is rebuilt only here so live traffic doesn't see
	// the queue churned 1×N times for a refresh-all run.
	if len(deletedIDs) > 0 {
		h.Pool.Reset()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":            len(accounts),
		"success":          success,
		"failed":           len(accounts) - success,
		"deleted":          len(deletedIDs),
		"deleted_accounts": deletedIDs,
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
		result["message"] = "登录成功但写入运行时 token 失败: " + err.Error()
		return result
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
			result["message"] = "刷新 token 成功但写入运行时 token 失败: " + err.Error()
			return result
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
