package client

import (
	"context"
	"net/http"
	"strings"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

const (
	accountEmptyOutputThreshold         = 1 << 30 // effectively disabled
	accountEmptyOutputFailureWindow     = 15 * time.Minute
	accountEmptyOutputRecoverySuccesses = 2
	accountEmptyOutputGlobalWindow      = 10 * time.Minute
	accountEmptyOutputGlobalMaxCooldown = 0 // disabled
	completionProxyHeader               = "X-Ds2api-Proxy-Id"
)

type accountEmptyOutputHealth struct {
	failures          int
	recoverySuccesses int
	lastFailure       time.Time
}

type accountEmptyOutputGlobalHealth struct {
	cooldowns   int
	windowStart time.Time
}

func (c *Client) RecordAccountEmptyOutput(a *auth.RequestAuth, reason string) {
	accountID := managedAccountID(a)
	if c == nil || accountID == "" {
		return
	}

	now := time.Now()
	c.accountHealthMu.Lock()
	if c.accountHealthMap == nil {
		c.accountHealthMap = map[string]*accountEmptyOutputHealth{}
	}
	h := c.accountHealthMap[accountID]
	if h == nil {
		h = &accountEmptyOutputHealth{}
		c.accountHealthMap[accountID] = h
	}
	if !h.lastFailure.IsZero() && now.Sub(h.lastFailure) > accountEmptyOutputFailureWindow {
		h.failures = 0
		h.recoverySuccesses = 0
	}
	h.failures++
	h.recoverySuccesses = 0
	h.lastFailure = now
	failures := h.failures
	shouldCooldown := failures >= accountEmptyOutputThreshold
	globalCooldowns := 0
	globalBudgetExceeded := false
	if shouldCooldown {
		h.failures = 0
		globalCooldowns, globalBudgetExceeded = c.recordEmptyOutputCooldownLocked(now)
	}
	c.accountHealthMu.Unlock()

	if !shouldCooldown {
		config.Logger.Info("[account_health] empty output observed",
			"account", accountID,
			"failures", failures,
			"threshold", accountEmptyOutputThreshold,
			"reason", reason,
		)
		return
	}

	if globalBudgetExceeded {
		config.Logger.Warn("[account_health] empty output cooldown suppressed",
			"account", accountID,
			"failures", failures,
			"threshold", accountEmptyOutputThreshold,
			"global_cooldowns", globalCooldowns,
			"global_window_seconds", int(accountEmptyOutputGlobalWindow/time.Second),
			"reason", reason,
		)
		return
	}

	config.Logger.Warn("[account_health] empty output threshold reached",
		"account", accountID,
		"failures", failures,
		"threshold", accountEmptyOutputThreshold,
		"global_cooldowns", globalCooldowns,
		"reason", reason,
	)
	if c.Auth != nil {
		c.Auth.CooldownAccount(a, "upstream_empty_output")
	}
}

func (c *Client) RecordAccountVisibleSuccess(a *auth.RequestAuth, reason string) {
	accountID := managedAccountID(a)
	if c == nil || accountID == "" {
		return
	}
	if c.Auth != nil {
		c.Auth.RecordAccountSuccess(a, reason)
	}

	c.accountHealthMu.Lock()
	defer c.accountHealthMu.Unlock()
	h := c.accountHealthMap[accountID]
	if h == nil || h.failures == 0 {
		return
	}
	h.recoverySuccesses++
	if h.recoverySuccesses < accountEmptyOutputRecoverySuccesses {
		return
	}
	h.failures--
	h.recoverySuccesses = 0
	config.Logger.Info("[account_health] empty output penalty decayed",
		"account", accountID,
		"remaining_failures", h.failures,
		"reason", reason,
	)
	if h.failures <= 0 {
		delete(c.accountHealthMap, accountID)
	}
}

func (c *Client) AvoidProxyForResponse(ctx context.Context, resp *http.Response, a *auth.RequestAuth, reason string) context.Context {
	if c == nil || ctx == nil {
		return ctx
	}
	proxyID := completionProxyID(resp)
	if proxyID == "" && a != nil {
		proxyID = strings.TrimSpace(a.Account.ProxyID)
	}
	if proxyID == "" {
		return ctx
	}
	config.Logger.Info("[proxy_sticky] avoiding proxy for retry",
		"proxy_id", proxyID,
		"account", managedAccountID(a),
		"reason", reason,
	)
	return withAvoidedProxyID(ctx, proxyID)
}

func attachCompletionProxyID(resp *http.Response, proxyID string) {
	if resp == nil || strings.TrimSpace(proxyID) == "" {
		return
	}
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	resp.Header.Set(completionProxyHeader, strings.TrimSpace(proxyID))
}

func completionProxyID(resp *http.Response) string {
	if resp == nil || resp.Header == nil {
		return ""
	}
	return strings.TrimSpace(resp.Header.Get(completionProxyHeader))
}

func managedAccountID(a *auth.RequestAuth) string {
	if a == nil || !a.UseConfigToken {
		return ""
	}
	return strings.TrimSpace(a.AccountID)
}

func (c *Client) recordEmptyOutputCooldownLocked(now time.Time) (int, bool) {
	if c.accountEmptyOutputGlobal.windowStart.IsZero() || now.Sub(c.accountEmptyOutputGlobal.windowStart) > accountEmptyOutputGlobalWindow {
		c.accountEmptyOutputGlobal.windowStart = now
		c.accountEmptyOutputGlobal.cooldowns = 0
	}
	c.accountEmptyOutputGlobal.cooldowns++
	return c.accountEmptyOutputGlobal.cooldowns, c.accountEmptyOutputGlobal.cooldowns > accountEmptyOutputGlobalMaxCooldown
}
