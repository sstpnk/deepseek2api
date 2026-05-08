package proxies

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"ds2api/internal/account"
	"ds2api/internal/config"
	adminshared "ds2api/internal/httpapi/admin/shared"
)

func newAdminProxyTestHandler(t *testing.T, raw string) *Handler {
	t.Helper()
	t.Setenv("DS2API_RUNTIME_STATS_PATH", filepath.Join(t.TempDir(), "runtime_stats.json"))
	t.Setenv("DS2API_CONFIG_JSON", raw)
	store := config.LoadStore()
	return &Handler{
		Store: store,
		Pool:  account.NewPool(store),
	}
}

func TestAddProxyPersistsNormalizedProxy(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{"accounts":[]}`)

	r := chi.NewRouter()
	r.Post("/admin/proxies", h.addProxy)

	req := httptest.NewRequest(http.MethodPost, "/admin/proxies", bytes.NewBufferString(`{
		"name":"  HK Exit  ",
		"type":" SOCKS5H ",
		"host":" 127.0.0.1 ",
		"port":1081,
		"username":" user ",
		"password":" pass "
	}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	proxies := h.Store.Snapshot().Proxies
	if len(proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(proxies))
	}
	if proxies[0].Name != "HK Exit" {
		t.Fatalf("unexpected proxy name: %#v", proxies[0])
	}
	if proxies[0].Type != "socks5h" {
		t.Fatalf("unexpected proxy type: %#v", proxies[0])
	}
	if proxies[0].Username != "user" || proxies[0].Password != "pass" {
		t.Fatalf("expected trimmed credentials, got %#v", proxies[0])
	}
	if proxies[0].ID == "" {
		t.Fatalf("expected generated proxy id, got %#v", proxies[0])
	}
}

func TestAddProxyDoesNotFailOnUnrelatedInvalidRuntimeConfig(t *testing.T) {
	router := newHTTPAdminHarness(t, `{
		"keys":["k1"],
		"runtime":{
			"account_max_inflight":8,
			"global_max_inflight":4
		}
	}`, &testingDSMock{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodPost, "/proxies", []byte(`{
		"name":"HK Exit",
		"type":"socks5h",
		"host":"127.0.0.1",
		"port":1080
	}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected add proxy success despite unrelated runtime issue, got %d body=%s", rec.Code, rec.Body.String())
	}

	readRec := httptest.NewRecorder()
	router.ServeHTTP(readRec, adminReq(http.MethodGet, "/config", nil))
	if readRec.Code != http.StatusOK {
		t.Fatalf("config read status=%d body=%s", readRec.Code, readRec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(readRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode config response: %v", err)
	}
	proxies, _ := payload["proxies"].([]any)
	if len(proxies) != 1 {
		t.Fatalf("expected proxy to be persisted, got %#v", payload["proxies"])
	}
}

func TestDeleteProxyClearsAssignedAccountProxyID(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{
		"proxies":[{"id":"proxy-1","name":"Node 1","type":"socks5","host":"127.0.0.1","port":1080}],
		"accounts":[{"email":"u@example.com","password":"pwd","proxy_id":"proxy-1"}]
	}`)

	r := chi.NewRouter()
	r.Delete("/admin/proxies/{proxyID}", h.deleteProxy)

	req := httptest.NewRequest(http.MethodDelete, "/admin/proxies/proxy-1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	snap := h.Store.Snapshot()
	if len(snap.Proxies) != 0 {
		t.Fatalf("expected proxy removed, got %#v", snap.Proxies)
	}
	if len(snap.Accounts) != 1 {
		t.Fatalf("expected account kept, got %#v", snap.Accounts)
	}
	if snap.Accounts[0].ProxyID != "" {
		t.Fatalf("expected proxy assignment cleared, got %#v", snap.Accounts[0])
	}
}

func TestUpdateProxyResponseDoesNotExposeStoredPassword(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{
		"proxies":[{"id":"proxy-1","name":"Node 1","type":"socks5h","host":"127.0.0.1","port":1080,"username":"u","password":"secret"}]
	}`)

	r := chi.NewRouter()
	r.Put("/admin/proxies/{proxyID}", h.updateProxy)

	req := httptest.NewRequest(http.MethodPut, "/admin/proxies/proxy-1", bytes.NewBufferString(`{
		"name":"Node 1",
		"type":"socks5h",
		"host":"127.0.0.2",
		"port":1081,
		"username":"u2"
	}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	proxy, _ := payload["proxy"].(map[string]any)
	if _, exists := proxy["password"]; exists {
		t.Fatalf("response should not expose password, got %#v", proxy)
	}
	if hasPassword, _ := proxy["has_password"].(bool); !hasPassword {
		t.Fatalf("expected has_password=true, got %#v", proxy)
	}
}

func TestUpdateAccountProxyAssignsProxyID(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{
		"proxies":[{"id":"proxy-1","name":"Node 1","type":"socks5h","host":"127.0.0.1","port":1080}],
		"accounts":[{"email":"u@example.com","password":"pwd"}]
	}`)

	r := chi.NewRouter()
	r.Put("/admin/accounts/{identifier}/proxy", h.updateAccountProxy)

	req := httptest.NewRequest(http.MethodPut, "/admin/accounts/u@example.com/proxy", bytes.NewBufferString(`{"proxy_id":"proxy-1"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	acc, ok := h.Store.FindAccount("u@example.com")
	if !ok {
		t.Fatal("expected account")
	}
	if acc.ProxyID != "proxy-1" {
		t.Fatalf("expected proxy assigned, got %#v", acc)
	}
}

func TestTestProxyUsesStoredProxy(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{
		"proxies":[{"id":"proxy-1","name":"Node 1","type":"socks5h","host":"127.0.0.1","port":1080}]
	}`)

	original := proxyConnectivityTester
	defer func() { proxyConnectivityTester = original }()

	var got config.Proxy
	proxyConnectivityTester = func(_ context.Context, proxy config.Proxy) map[string]any {
		got = proxy
		return map[string]any{
			"success":       true,
			"proxy_id":      proxy.ID,
			"proxy_type":    proxy.Type,
			"response_time": 12,
		}
	}

	r := chi.NewRouter()
	r.Post("/admin/proxies/test", h.testProxy)

	req := httptest.NewRequest(http.MethodPost, "/admin/proxies/test", bytes.NewBufferString(`{"proxy_id":"proxy-1"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if got.ID != "proxy-1" || got.Type != "socks5h" {
		t.Fatalf("expected stored proxy passed to tester, got %#v", got)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if ok, _ := payload["success"].(bool); !ok {
		t.Fatalf("expected success payload, got %#v", payload)
	}
}

func TestProxySwitchCloudflareThenRestoreResin(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{
		"proxies":[
			{"id":"resin-a","name":"resin-u","type":"socks5","host":"161.118.201.199","port":21345,"username":"Default.u","password":"secret"},
			{"id":"cf-1","name":"CF Worker","type":"cloudflare","host":"worker.example.workers.dev","port":443,"worker_host":"worker.example.workers.dev"}
		],
		"accounts":[{"email":"u@example.com","password":"pwd","proxy_id":"resin-a"}]
	}`)

	r := chi.NewRouter()
	r.Post("/admin/proxies/switch", h.switchProxyMode)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/proxies/switch", bytes.NewBufferString(`{"mode":"cloudflare","cf_proxy_id":"cf-1"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch to cf status=%d body=%s", rec.Code, rec.Body.String())
	}
	snap := h.Store.Snapshot()
	if snap.ProxySwitch.Mode != "cloudflare" || snap.Accounts[0].ProxyID != "cf-1" {
		t.Fatalf("expected cloudflare mode and binding, got mode=%q account=%#v", snap.ProxySwitch.Mode, snap.Accounts[0])
	}
	if got := snap.ProxySwitch.ResinAssignments["u@example.com"]; got != "resin-a" {
		t.Fatalf("expected resin snapshot, got %q", got)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/proxies/switch", bytes.NewBufferString(`{"mode":"resin"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch to resin status=%d body=%s", rec.Code, rec.Body.String())
	}
	snap = h.Store.Snapshot()
	if snap.ProxySwitch.Mode != "resin" || snap.Accounts[0].ProxyID != "resin-a" {
		t.Fatalf("expected resin mode and restored binding, got mode=%q account=%#v", snap.ProxySwitch.Mode, snap.Accounts[0])
	}
}

func TestProxySwitchResinCreatesProxyForAccountAddedDuringCloudflareMode(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{
		"proxies":[
			{"id":"resin-a","name":"resin-u","type":"socks5","host":"161.118.201.199","port":21345,"username":"Default.u","password":"secret"},
			{"id":"cf-1","name":"CF Worker","type":"cloudflare","host":"worker.example.workers.dev","port":443,"worker_host":"worker.example.workers.dev"}
		],
		"accounts":[{"email":"u@example.com","password":"pwd","proxy_id":"resin-a"}],
		"proxy_switch":{
			"mode":"cloudflare",
			"cf_proxy_id":"cf-1",
			"resin_assignments":{"u@example.com":"resin-a"},
			"resin_proxy_template":{"name":"resin-{local}","type":"socks5","host":"161.118.201.199","port":21345,"username":"Default.{local}","password":"secret"}
		}
	}`)

	if err := h.Store.Update(func(c *config.Config) error {
		c.Accounts = append(c.Accounts, config.Account{Email: "new@example.com", Password: "pwd", ProxyID: "cf-1"})
		return nil
	}); err != nil {
		t.Fatalf("seed new account: %v", err)
	}

	r := chi.NewRouter()
	r.Post("/admin/proxies/switch", h.switchProxyMode)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/proxies/switch", bytes.NewBufferString(`{"mode":"resin"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch to resin status=%d body=%s", rec.Code, rec.Body.String())
	}
	snap := h.Store.Snapshot()
	var newAcc config.Account
	for _, acc := range snap.Accounts {
		if acc.Email == "new@example.com" {
			newAcc = acc
		}
	}
	if newAcc.ProxyID == "" || newAcc.ProxyID == "cf-1" {
		t.Fatalf("expected generated resin proxy for new account, got %#v", newAcc)
	}
	proxy, ok := adminshared.FindProxyByID(snap, newAcc.ProxyID)
	if !ok {
		t.Fatalf("generated proxy not found: %s", newAcc.ProxyID)
	}
	if proxy.Name != "resin-new" || proxy.Username != "Default.new" {
		t.Fatalf("unexpected generated proxy: %#v", proxy)
	}
}

func TestProxySwitchCloudflareSnapshotsCurrentResinAssignments(t *testing.T) {
	h := newAdminProxyTestHandler(t, `{
		"proxies":[
			{"id":"resin-a","name":"resin-old","type":"socks5","host":"161.118.201.199","port":21345,"username":"Default.old","password":"secret"},
			{"id":"resin-b","name":"resin-new","type":"socks5","host":"161.118.201.199","port":21345,"username":"Default.new","password":"secret"},
			{"id":"cf-1","name":"CF Worker","type":"cloudflare","host":"worker.example.workers.dev","port":443,"worker_host":"worker.example.workers.dev"}
		],
		"accounts":[{"email":"u@example.com","password":"pwd","proxy_id":"resin-b"}],
		"proxy_switch":{
			"mode":"resin",
			"cf_proxy_id":"cf-1",
			"resin_assignments":{"u@example.com":"resin-a"},
			"resin_proxy_template":{"name":"resin-{local}","type":"socks5","host":"161.118.201.199","port":21345,"username":"Default.{local}","password":"secret"}
		}
	}`)

	r := chi.NewRouter()
	r.Post("/admin/proxies/switch", h.switchProxyMode)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/proxies/switch", bytes.NewBufferString(`{"mode":"cloudflare"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch to cf status=%d body=%s", rec.Code, rec.Body.String())
	}
	snap := h.Store.Snapshot()
	if got := snap.ProxySwitch.ResinAssignments["u@example.com"]; got != "resin-b" {
		t.Fatalf("expected latest resin assignment to be snapshotted, got %q", got)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/proxies/switch", bytes.NewBufferString(`{"mode":"resin"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch to resin status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := h.Store.Snapshot().Accounts[0].ProxyID; got != "resin-b" {
		t.Fatalf("expected restored latest resin assignment, got %q", got)
	}
}
