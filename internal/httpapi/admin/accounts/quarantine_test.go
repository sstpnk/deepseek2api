package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

// scriptedLoginDSMock returns whatever sequence of errors the test programs.
// Index advances on each Login. Once exhausted it returns the last entry
// indefinitely so we never run off the end of a slice.
type scriptedLoginDSMock struct {
	mu      sync.Mutex
	errs    []error
	calls   int32
	perAcct map[string][]error
	idxByID map[string]int
}

func (m *scriptedLoginDSMock) Login(_ context.Context, acc config.Account) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	atomic.AddInt32(&m.calls, 1)
	id := acc.Identifier()
	if seq, ok := m.perAcct[id]; ok && len(seq) > 0 {
		i := m.idxByID[id]
		if i >= len(seq) {
			i = len(seq) - 1
		}
		err := seq[i]
		m.idxByID[id] = i + 1
		if err != nil {
			return "", err
		}
		return "ok-token", nil
	}
	if len(m.errs) == 0 {
		return "ok-token", nil
	}
	i := int(atomic.LoadInt32(&m.calls)) - 1
	if i >= len(m.errs) {
		i = len(m.errs) - 1
	}
	if err := m.errs[i]; err != nil {
		return "", err
	}
	return "ok-token", nil
}

func (m *scriptedLoginDSMock) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "session-id", nil
}
func (m *scriptedLoginDSMock) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "", errors.New("unused")
}
func (m *scriptedLoginDSMock) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ map[string]any, _ string, _ int) (*http.Response, error) {
	return nil, errors.New("unused")
}
func (m *scriptedLoginDSMock) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	return nil
}
func (m *scriptedLoginDSMock) GetSessionCountForToken(_ context.Context, _ string) (*dsclient.SessionStats, error) {
	return &dsclient.SessionStats{Success: true}, nil
}

// newHarnessWithSweeper wires up the same harness as newHTTPAdminHarness but
// also constructs and attaches a Sweeper so the /accounts/quarantine/sweep
// endpoint and SweepOnce calls have somewhere to land.
func newHarnessWithSweeper(t *testing.T, rawConfig string, ds *scriptedLoginDSMock) (http.Handler, *Sweeper, *Handler) {
	t.Helper()
	t.Setenv("DS2API_CONFIG_JSON", rawConfig)
	store := config.LoadStore()
	pool := account.NewPool(store)
	sweeper := NewSweeper(store, pool, ds)
	h := &Handler{Store: store, Pool: pool, DS: ds, Sweeper: sweeper}
	r := chi.NewRouter()
	RegisterRoutes(r, h)
	return r, sweeper, h
}

// TestQuarantine_EntryAndThreeStrikeDelete verifies the full lifecycle: an
// account that fails the auto-delete probe enters quarantine with Failures=0,
// each subsequent failed sweep increments Failures, and the third strike
// removes it for real along with its dedicated proxy.
func TestQuarantine_EntryAndThreeStrikeDelete(t *testing.T) {
	const cfg = `{
		"accounts":[{"email":"dead@example.com","password":"pwd","token":"","proxy_id":"proxy_dead"}],
		"proxies":[{"id":"proxy_dead","name":"dedicated","type":"http","host":"127.0.0.1","port":1234}]
	}`
	srv, sweeper, h := newHarnessWithSweeper(t, cfg, &scriptedLoginDSMock{errs: []error{errors.New("boom")}})

	// 1. Auto-delete request quarantines the account.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts/test", []byte(`{"identifier":"dead@example.com","auto_delete":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("/accounts/test: expected 200 got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := h.Store.Snapshot(); len(got.Accounts) != 0 {
		t.Fatalf("expected accounts emptied after quarantine, got %#v", got.Accounts)
	}
	if got := h.Store.Snapshot().Quarantine; len(got) != 1 || got[0].Failures != 0 {
		t.Fatalf("expected 1 quarantined entry with Failures=0, got %#v", got)
	}

	// 2. Three failed sweeps: Failures should go 1 -> 2 -> remove.
	for strike := 1; strike <= QuarantineMaxFailures; strike++ {
		probed, restored, deleted, still := sweeper.SweepOnce(context.Background())
		if probed != 1 {
			t.Fatalf("strike %d: expected probed=1 got %d", strike, probed)
		}
		if restored != 0 {
			t.Fatalf("strike %d: expected restored=0 got %d", strike, restored)
		}
		if strike < QuarantineMaxFailures {
			if deleted != 0 || still != 1 {
				t.Fatalf("strike %d: expected deleted=0 still=1 got deleted=%d still=%d", strike, deleted, still)
			}
			snap := h.Store.Snapshot()
			if got := snap.Quarantine[0].Failures; got != strike {
				t.Fatalf("strike %d: expected Failures=%d got %d", strike, strike, got)
			}
		} else {
			if deleted != 1 || still != 0 {
				t.Fatalf("final strike: expected deleted=1 still=0 got deleted=%d still=%d", deleted, still)
			}
		}
	}

	// 3. The dedicated proxy was orphaned by the deletion and should be gone.
	final := h.Store.Snapshot()
	if len(final.Quarantine) != 0 {
		t.Fatalf("expected quarantine empty after final strike, got %#v", final.Quarantine)
	}
	if len(final.Proxies) != 0 {
		t.Fatalf("expected dedicated proxy removed with the account, got %#v", final.Proxies)
	}
}

// TestQuarantine_RestoreOnSweepSuccess verifies that a single successful Login
// during a sweep brings the account back to the active list with its ProxyID
// intact, and clears the quarantine slot.
func TestQuarantine_RestoreOnSweepSuccess(t *testing.T) {
	const cfg = `{
		"accounts":[{"email":"flaky@example.com","password":"pwd","token":"","proxy_id":"proxy_keep"}],
		"proxies":[{"id":"proxy_keep","name":"keep","type":"http","host":"127.0.0.1","port":1234}]
	}`
	// The auto-delete path needs three boom Logins (testAccount + two-strike
	// verifyAccountUnusable). Then a later sweep succeeds —
	// verifyAccountUsable returns true on the first successful Login.
	mock := &scriptedLoginDSMock{
		errs: []error{
			errors.New("boom1"),
			errors.New("boom2"),
			errors.New("boom3"),
			nil,
		},
	}
	srv, sweeper, h := newHarnessWithSweeper(t, cfg, mock)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts/test", []byte(`{"identifier":"flaky@example.com","auto_delete":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rec.Code)
	}
	if len(h.Store.Snapshot().Quarantine) != 1 {
		t.Fatalf("expected account in quarantine after auto-delete, got %#v", h.Store.Snapshot().Quarantine)
	}

	// Sweep: Login succeeds → restore.
	probed, restored, deleted, still := sweeper.SweepOnce(context.Background())
	if probed != 1 || restored != 1 || deleted != 0 || still != 0 {
		t.Fatalf("expected probed=1 restored=1 deleted=0 still=0, got %d %d %d %d", probed, restored, deleted, still)
	}
	snap := h.Store.Snapshot()
	if len(snap.Quarantine) != 0 {
		t.Fatalf("expected quarantine cleared after restore, got %#v", snap.Quarantine)
	}
	if len(snap.Accounts) != 1 || snap.Accounts[0].ProxyID != "proxy_keep" {
		t.Fatalf("expected account restored with ProxyID intact, got %#v", snap.Accounts)
	}
	if len(snap.Proxies) != 1 {
		t.Fatalf("expected proxy preserved on restore, got %#v", snap.Proxies)
	}
}

// TestQuarantine_OrphanProxyKeptWhenShared ensures the proxy-cleanup logic
// only removes a proxy when no surviving account references it.
func TestQuarantine_OrphanProxyKeptWhenShared(t *testing.T) {
	const cfg = `{
		"accounts":[
			{"email":"dead@example.com","password":"pwd","proxy_id":"proxy_shared"},
			{"email":"alive@example.com","password":"pwd","proxy_id":"proxy_shared"}
		],
		"proxies":[{"id":"proxy_shared","name":"shared","type":"http","host":"127.0.0.1","port":1234}]
	}`
	// dead@ always fails; alive@ always succeeds. Sweeper will only see the
	// dead account because alive stays in Accounts.
	mock := &scriptedLoginDSMock{
		perAcct: map[string][]error{
			"dead@example.com":  {errors.New("boom"), errors.New("boom"), errors.New("boom"), errors.New("boom"), errors.New("boom")},
			"alive@example.com": {nil},
		},
		idxByID: map[string]int{},
	}
	srv, sweeper, h := newHarnessWithSweeper(t, cfg, mock)

	// Quarantine the dead one.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts/test", []byte(`{"identifier":"dead@example.com","auto_delete":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rec.Code)
	}

	// Three failed sweeps → the dead account is removed but the shared proxy
	// must stay because alive@ still points at it.
	for i := 0; i < QuarantineMaxFailures; i++ {
		sweeper.SweepOnce(context.Background())
	}
	snap := h.Store.Snapshot()
	if len(snap.Quarantine) != 0 {
		t.Fatalf("expected dead account gone after three strikes, got %#v", snap.Quarantine)
	}
	if len(snap.Accounts) != 1 || snap.Accounts[0].Email != "alive@example.com" {
		t.Fatalf("expected only alive account remaining, got %#v", snap.Accounts)
	}
	if len(snap.Proxies) != 1 || snap.Proxies[0].ID != "proxy_shared" {
		t.Fatalf("expected shared proxy preserved, got %#v", snap.Proxies)
	}
}

// TestQuarantine_ManualRestoreEndpoint exercises the admin restore endpoint
// without running a sweep (e.g., user knows the upstream is fine and wants
// the account back immediately).
func TestQuarantine_ManualRestoreEndpoint(t *testing.T) {
	const cfg = `{
		"accounts":[{"email":"dead@example.com","password":"pwd","proxy_id":"proxy_keep"}],
		"proxies":[{"id":"proxy_keep","type":"http","host":"127.0.0.1","port":1234}]
	}`
	mock := &scriptedLoginDSMock{errs: []error{errors.New("boom")}}
	srv, _, h := newHarnessWithSweeper(t, cfg, mock)

	// Quarantine.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts/test", []byte(`{"identifier":"dead@example.com","auto_delete":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rec.Code)
	}

	// Manual restore.
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts/quarantine/restore", []byte(`{"identifier":"dead@example.com"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("restore: expected 200 got %d body=%s", rec.Code, rec.Body.String())
	}

	snap := h.Store.Snapshot()
	if len(snap.Quarantine) != 0 {
		t.Fatalf("expected quarantine empty after restore, got %#v", snap.Quarantine)
	}
	if len(snap.Accounts) != 1 || snap.Accounts[0].ProxyID != "proxy_keep" {
		t.Fatalf("expected restored account with ProxyID intact, got %#v", snap.Accounts)
	}
}

// TestQuarantine_ListEndpointShape sanity-checks the JSON shape returned by
// GET /accounts/quarantine, since the WebUI relies on its specific keys.
func TestQuarantine_ListEndpointShape(t *testing.T) {
	const cfg = `{"accounts":[{"email":"dead@example.com","password":"pwd"}]}`
	mock := &scriptedLoginDSMock{errs: []error{errors.New("boom")}}
	srv, _, _ := newHarnessWithSweeper(t, cfg, mock)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts/test", []byte(`{"identifier":"dead@example.com","auto_delete":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("test: expected 200 got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, adminReq(http.MethodGet, "/accounts/quarantine", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200 got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"items", "total", "max_failures", "sweep_interval"} {
		if _, ok := resp[k]; !ok {
			t.Fatalf("expected key %q in list response, body=%s", k, rec.Body.String())
		}
	}
	items := resp["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	first := items[0].(map[string]any)
	for _, k := range []string{"identifier", "failures", "remaining_attempts", "max_failures", "quarantined_at"} {
		if _, ok := first[k]; !ok {
			t.Fatalf("expected item key %q, got %#v", k, first)
		}
	}
}
