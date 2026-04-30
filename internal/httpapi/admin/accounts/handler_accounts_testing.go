package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"ds2api/internal/config"
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
