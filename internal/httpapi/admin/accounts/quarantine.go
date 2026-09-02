package accounts

// Account quarantine ("观察区") sits between auto-delete-on-failure and the
// final delete. When an account fails the two-strike Login probe in the
// testing handlers, instead of being removed we move it here. A background
// sweeper re-verifies every quarantined account every QuarantineSweepInterval;
// any single Login success restores the account (proxy_id intact), three
// consecutive sweep failures mark it as confirmed dead and remove it for real
// — and remove its dedicated proxy if no one else references it.
//
// Concurrency rules (mirrors feedback_ds2api_account_pool_safety): every
// mutation goes through Store.Update so it lands in config.json, and Pool.Reset
// is called only once per sweep batch (not per account) so the queue does not
// thrash live traffic.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/config"
	adminshared "ds2api/internal/httpapi/admin/shared"
)

const (
	// QuarantineSweepInterval is the period at which the sweeper re-verifies
	// every quarantined account. The user's spec is "三次机会, 6 小时" so each
	// quarantined account gets three probes spaced two hours apart.
	QuarantineSweepInterval = 2 * time.Hour
	// QuarantineMaxFailures is the cap on per-account failures before we
	// confirm the account is dead and remove it. Failures starts at 0 on
	// quarantine entry and increments by one per failed sweep, so reaching 3
	// means three consecutive sweep failures — the third strike.
	QuarantineMaxFailures = 3
	// quarantineSweepConcurrency keeps the verification load on the upstream
	// bounded; sweeps run unattended and we don't want them to behave like a
	// burst of test traffic.
	quarantineSweepConcurrency = 5
)

// Sweeper drives the periodic re-verification of quarantined accounts. It is
// constructed once at server startup and Run in a goroutine; the context
// passed to Run controls shutdown.
type Sweeper struct {
	Store    adminshared.ConfigStore
	Pool     adminshared.PoolController
	DS       adminshared.DeepSeekCaller
	Interval time.Duration

	// running is incremented while a sweep is in progress so manual sweeps
	// invoked via the admin endpoint and the periodic sweeper do not interleave
	// (the second caller observes the busy state and returns immediately).
	running int32
}

// NewSweeper constructs a Sweeper with the default sweep interval. Tests may
// override Interval afterwards.
func NewSweeper(store adminshared.ConfigStore, pool adminshared.PoolController, ds adminshared.DeepSeekCaller) *Sweeper {
	return &Sweeper{Store: store, Pool: pool, DS: ds, Interval: QuarantineSweepInterval}
}

// Run blocks until ctx is done. It runs SweepOnce on a ticker.
//
// We deliberately do NOT run a sweep immediately on startup: the user only
// just rebooted, accounts may be hot, and any transient login failure during
// boot would be amplified by an immediate sweep racing the warmup. The first
// sweep happens after Interval has elapsed.
func (s *Sweeper) Run(ctx context.Context) {
	if s == nil {
		return
	}
	interval := s.Interval
	if interval <= 0 {
		interval = QuarantineSweepInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	config.Logger.Info("[quarantine] sweeper started", "interval", interval.String(), "max_failures", QuarantineMaxFailures)
	for {
		select {
		case <-ctx.Done():
			config.Logger.Info("[quarantine] sweeper stopped")
			return
		case <-t.C:
			s.SweepOnce(ctx)
		}
	}
}

// SweepOnce re-verifies every quarantined account and applies the result:
// success (any single Login OK) restores the account; failure increments the
// per-account counter; reaching QuarantineMaxFailures removes it for real.
// Returns the counts useful for logging and the manual-sweep endpoint.
func (s *Sweeper) SweepOnce(ctx context.Context) (probed, restored, deleted, stillQuarantined int) {
	if s == nil {
		return 0, 0, 0, 0
	}
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		// Already in progress.
		return 0, 0, 0, 0
	}
	defer atomic.StoreInt32(&s.running, 0)

	snapshot := s.Store.Snapshot().Quarantine
	if len(snapshot) == 0 {
		return 0, 0, 0, 0
	}

	type probeOutcome struct {
		identifier string
		success    bool
		lastErr    string
	}
	outcomes := make([]probeOutcome, len(snapshot))

	// Reuse the same worker-pool helper used by /accounts/test-all so the
	// concurrency story is identical: bounded fan-out, per-account independent
	// upstream calls, no cross-talk between probes.
	accs := make([]config.Account, len(snapshot))
	for i, q := range snapshot {
		accs[i] = q.Account
	}
	_ = runAccountTestsConcurrently(accs, quarantineSweepConcurrency, func(idx int, acc config.Account) map[string]any {
		ok, lastErr := verifyAccountUsable(ctx, s.DS, acc)
		outcomes[idx] = probeOutcome{identifier: acc.Identifier(), success: ok, lastErr: lastErr}
		// We don't use the returned map — outcomes carries the data we need.
		return map[string]any{}
	})

	probed = len(outcomes)
	now := time.Now().Unix()
	restoredIDs := make([]string, 0)
	deletedIDs := make([]string, 0)

	// Apply all outcomes in a single Store.Update so the file is rewritten
	// once per sweep regardless of how many entries flipped state.
	err := s.Store.Update(func(c *config.Config) error {
		// Build a quick lookup so we can update Failures/LastError in place.
		idx := make(map[string]int, len(c.Quarantine))
		for i, q := range c.Quarantine {
			id := q.Account.Identifier()
			if id != "" {
				idx[id] = i
			}
		}

		newQuarantine := c.Quarantine[:0:0]
		newQuarantine = append(newQuarantine, c.Quarantine...)

		for _, out := range outcomes {
			pos, ok := idx[out.identifier]
			if !ok {
				continue
			}
			entry := newQuarantine[pos]
			entry.LastCheckedAt = now
			if out.success {
				// Restore: account moves back to Accounts with ProxyID intact.
				// We do NOT keep its old Token (which we cleared) — the next
				// Pool.acquire round will trigger a fresh Login via the
				// resolver. ProxyID is preserved verbatim because the
				// quarantine entry stored a verbatim copy of the Account.
				c.Accounts = append(c.Accounts, entry.Account)
				newQuarantine[pos] = QuarantinedAccount{} // mark for removal
				restoredIDs = append(restoredIDs, out.identifier)
				continue
			}
			entry.Failures++
			entry.LastError = strings.TrimSpace(out.lastErr)
			if entry.Failures >= QuarantineMaxFailures {
				// Confirmed dead: remove account AND its dedicated proxy if
				// no other account (active or quarantined) still references
				// it. Keeping the proxy when shared is the safe default —
				// it's cheap to leave it lying around and disastrous to
				// silently rip it out from under another account.
				dropProxyIfOrphan(c, entry.Account.ProxyID, out.identifier)
				newQuarantine[pos] = QuarantinedAccount{} // mark for removal
				deletedIDs = append(deletedIDs, out.identifier)
				continue
			}
			newQuarantine[pos] = entry
		}

		// Compact: drop entries we marked empty.
		compact := make([]QuarantinedAccount, 0, len(newQuarantine))
		for _, q := range newQuarantine {
			if q.Account.Identifier() == "" {
				continue
			}
			compact = append(compact, q)
		}
		c.Quarantine = compact
		return nil
	})
	if err != nil {
		config.Logger.Warn("[quarantine] sweep persist failed", "error", err)
		return probed, 0, 0, 0
	}

	if len(restoredIDs) > 0 {
		// Pool only sees Accounts; restoring brings IDs back into rotation.
		s.Pool.Reset()
	}

	// Recount what stayed in quarantine.
	stillQuarantined = len(s.Store.Snapshot().Quarantine)
	restored = len(restoredIDs)
	deleted = len(deletedIDs)

	config.Logger.Info("[quarantine] sweep done",
		"probed", probed,
		"restored", restored,
		"deleted", deleted,
		"still_quarantined", stillQuarantined,
		"restored_ids", restoredIDs,
		"deleted_ids", deletedIDs,
	)
	return probed, restored, deleted, stillQuarantined
}

// verifyAccountUsable runs the same two-strike Login probe used by the
// auto-delete path, but inverted: returns (true, "") if either probe
// succeeds, (false, lastErr) if both fail. Mirrors verifyAccountUnusable so
// quarantine entry and quarantine sweep use the exact same definition of
// "the account works."
func verifyAccountUsable(ctx context.Context, ds adminshared.DeepSeekCaller, acc config.Account) (bool, string) {
	if _, err := ds.Login(ctx, acc); err == nil {
		return true, ""
	} else {
		// Hold onto the first error in case the second also fails.
		first := err.Error()
		if _, err2 := ds.Login(ctx, acc); err2 == nil {
			return true, ""
		} else {
			return false, joinErrors(first, err2.Error())
		}
	}
}

func joinErrors(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	switch {
	case a == "" && b == "":
		return ""
	case a == "":
		return b
	case b == "":
		return a
	case a == b:
		return a
	default:
		return a + "; " + b
	}
}

// quarantineAccount moves an account from c.Accounts to c.Quarantine inside a
// Store.Update mutator. Failures starts at 0 — the testing handler that
// triggers this has already run two-strike verification, but the user's spec
// says "three sweep chances" beyond that, so we don't pre-charge a failure.
// lastError is what the caller saw on the test that put the account here.
func quarantineAccountLocked(c *config.Config, identifier, lastError, reason string) error {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return fmt.Errorf("identifier 为空")
	}
	idx := -1
	for i, a := range c.Accounts {
		if adminshared.AccountMatchesIdentifier(a, identifier) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("账号不存在: %s", identifier)
	}
	acc := c.Accounts[idx]
	c.Accounts = append(c.Accounts[:idx], c.Accounts[idx+1:]...)

	// If the same identifier already has a quarantine entry (e.g. user
	// re-imported then it failed again), update in place instead of creating
	// a duplicate. Failures resets to 0 — the user is giving the account a
	// fresh round of three chances.
	for i, q := range c.Quarantine {
		if adminshared.AccountMatchesIdentifier(q.Account, identifier) {
			c.Quarantine[i] = QuarantinedAccount{
				Account:       acc,
				QuarantinedAt: time.Now().Unix(),
				LastCheckedAt: 0,
				Failures:      0,
				LastError:     strings.TrimSpace(lastError),
				Reason:        strings.TrimSpace(reason),
			}
			return nil
		}
	}
	c.Quarantine = append(c.Quarantine, QuarantinedAccount{
		Account:       acc,
		QuarantinedAt: time.Now().Unix(),
		Failures:      0,
		LastError:     strings.TrimSpace(lastError),
		Reason:        strings.TrimSpace(reason),
	})
	return nil
}

// QuarantinedAccount is a re-export so package-internal code can use the
// short name without importing config; the codec lives in config so the
// JSON shape is owned there.
type QuarantinedAccount = config.QuarantinedAccount

// dropProxyIfOrphan removes the named proxy entry from c.Proxies but only if
// no remaining account (active or quarantined besides the one being deleted)
// references it. Mutates c in place.
func dropProxyIfOrphan(c *config.Config, proxyID, deletingIdentifier string) {
	proxyID = strings.TrimSpace(proxyID)
	if proxyID == "" {
		return
	}
	for _, a := range c.Accounts {
		if a.ProxyID == proxyID {
			return
		}
	}
	for _, q := range c.Quarantine {
		if q.Account.ProxyID != proxyID {
			continue
		}
		// Skip the entry we're deleting so we don't mistake it for a still-
		// present user.
		if adminshared.AccountMatchesIdentifier(q.Account, deletingIdentifier) {
			continue
		}
		return
	}
	for i, p := range c.Proxies {
		if p.ID == proxyID {
			c.Proxies = append(c.Proxies[:i], c.Proxies[i+1:]...)
			config.Logger.Info("[quarantine] removed orphan proxy", "proxy_id", proxyID, "with_account", deletingIdentifier)
			return
		}
	}
}

// ---- HTTP handlers ----

// listQuarantine returns the current quarantine entries. It is read-only and
// never triggers a sweep, so calling it from the WebUI is cheap.
func (h *Handler) listQuarantine(w http.ResponseWriter, _ *http.Request) {
	q := h.Store.Snapshot().Quarantine
	items := make([]map[string]any, 0, len(q))
	for _, entry := range q {
		items = append(items, quarantineToMap(entry))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":          items,
		"total":          len(items),
		"max_failures":   QuarantineMaxFailures,
		"sweep_interval": int(QuarantineSweepInterval.Seconds()),
	})
}

func quarantineToMap(entry QuarantinedAccount) map[string]any {
	acc := entry.Account
	return map[string]any{
		"identifier":         acc.Identifier(),
		"name":               acc.Name,
		"remark":             acc.Remark,
		"email":              acc.Email,
		"mobile":             acc.Mobile,
		"proxy_id":           acc.ProxyID,
		"failures":           entry.Failures,
		"max_failures":       QuarantineMaxFailures,
		"last_error":         entry.LastError,
		"reason":             entry.Reason,
		"quarantined_at":     entry.QuarantinedAt,
		"last_checked_at":    entry.LastCheckedAt,
		"remaining_attempts": maxInt(0, QuarantineMaxFailures-entry.Failures),
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// sweepQuarantineNow triggers an immediate SweepOnce. The Handler holds a
// reference to the Sweeper instance so live traffic can re-verify on demand
// (e.g. user just fixed a flaky proxy and wants to drag accounts out without
// waiting two hours).
func (h *Handler) sweepQuarantineNow(w http.ResponseWriter, r *http.Request) {
	if h.Sweeper == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"detail": "quarantine sweeper not initialized"})
		return
	}
	probed, restored, deleted, still := h.Sweeper.SweepOnce(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"probed":            probed,
		"restored":          restored,
		"deleted":           deleted,
		"still_quarantined": still,
	})
}

// restoreQuarantineEntry pulls an account out of quarantine and back into
// Accounts without re-verifying it — this is the "I know this account is
// fine, stop probing it" escape hatch. ProxyID is preserved.
func (h *Handler) restoreQuarantineEntry(w http.ResponseWriter, r *http.Request) {
	identifier := identifierFromQuarantineRequest(r)
	if identifier == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "需要 identifier"})
		return
	}
	err := h.Store.Update(func(c *config.Config) error {
		for i, q := range c.Quarantine {
			if !adminshared.AccountMatchesIdentifier(q.Account, identifier) {
				continue
			}
			c.Accounts = append(c.Accounts, q.Account)
			c.Quarantine = append(c.Quarantine[:i], c.Quarantine[i+1:]...)
			return nil
		}
		return newRequestError("观察区中无此账号")
	})
	if err != nil {
		if detail, ok := requestErrorDetail(err); ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"detail": detail})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	h.Pool.Reset()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// deleteQuarantineEntry removes an account from quarantine immediately,
// applying the same orphan-proxy cleanup the sweeper would. Used when the
// user is confident the account is dead and doesn't want to wait the full
// six hours.
func (h *Handler) deleteQuarantineEntry(w http.ResponseWriter, r *http.Request) {
	identifier := identifierFromQuarantineRequest(r)
	if identifier == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "需要 identifier"})
		return
	}
	err := h.Store.Update(func(c *config.Config) error {
		for i, q := range c.Quarantine {
			if !adminshared.AccountMatchesIdentifier(q.Account, identifier) {
				continue
			}
			dropProxyIfOrphan(c, q.Account.ProxyID, identifier)
			c.Quarantine = append(c.Quarantine[:i], c.Quarantine[i+1:]...)
			return nil
		}
		return newRequestError("观察区中无此账号")
	})
	if err != nil {
		if detail, ok := requestErrorDetail(err); ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"detail": detail})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// identifierFromQuarantineRequest accepts the identifier from either a URL
// path parameter or a JSON body, mirroring how /admin/accounts/test resolves
// it. A POST endpoint with the identifier in the body is the easiest call
// for the WebUI; the URL form is convenient for shell scripts.
func identifierFromQuarantineRequest(r *http.Request) string {
	identifier := strings.TrimSpace(chi.URLParam(r, "identifier"))
	if identifier != "" {
		if decoded, err := url.PathUnescape(identifier); err == nil {
			identifier = decoded
		}
		return identifier
	}
	if r.Body == nil {
		return ""
	}
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	id, _ := req["identifier"].(string)
	return strings.TrimSpace(id)
}
