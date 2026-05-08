package configmgmt

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestBatchImportUsesCloudflareProxyWhenInCloudflareMode(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"proxies":[
			{"id":"cf-1","name":"CF Worker","type":"cloudflare","host":"worker.example.workers.dev","port":443,"worker_host":"worker.example.workers.dev"}
		],
		"accounts":[{"email":"u@example.com","password":"pwd","proxy_id":"cf-1"}],
		"proxy_switch":{"cf_proxy_id":"cf-1"}
	}`)

	r := chi.NewRouter()
	r.Post("/admin/import", h.batchImport)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/import", bytes.NewBufferString(`{
		"accounts":[{"email":"new@example.com","password":"pwd"}]
	}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	snap := h.Store.Snapshot()
	if len(snap.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %#v", snap.Accounts)
	}
	if got := snap.Accounts[1].ProxyID; got != "cf-1" {
		t.Fatalf("expected imported account to use cloudflare proxy, got %q", got)
	}
}

func TestConfigImportMergeUsesCloudflareProxyWhenInCloudflareMode(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"proxies":[
			{"id":"cf-1","name":"CF Worker","type":"cloudflare","host":"worker.example.workers.dev","port":443,"worker_host":"worker.example.workers.dev"}
		],
		"accounts":[{"email":"u@example.com","password":"pwd","proxy_id":"cf-1"}],
		"proxy_switch":{"cf_proxy_id":"cf-1"}
	}`)

	r := chi.NewRouter()
	r.Post("/admin/config/import", h.configImport)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/config/import?mode=merge", bytes.NewBufferString(`{
		"accounts":[{"email":"new@example.com","password":"pwd"}]
	}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	snap := h.Store.Snapshot()
	if len(snap.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %#v", snap.Accounts)
	}
	if got := snap.Accounts[1].ProxyID; got != "cf-1" {
		t.Fatalf("expected imported account to use cloudflare proxy, got %q", got)
	}
}

func TestUpdateConfigUsesCloudflareProxyForNewAccounts(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"proxies":[
			{"id":"cf-1","name":"CF Worker","type":"cloudflare","host":"worker.example.workers.dev","port":443,"worker_host":"worker.example.workers.dev"}
		],
		"accounts":[{"email":"u@example.com","password":"pwd","proxy_id":"cf-1"}],
		"proxy_switch":{"cf_proxy_id":"cf-1"}
	}`)

	r := chi.NewRouter()
	r.Post("/admin/config", h.updateConfig)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/config", bytes.NewBufferString(`{
		"accounts":[
			{"email":"u@example.com","password":"pwd","proxy_id":"cf-1"},
			{"email":"new@example.com","password":"pwd"}
		]
	}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	snap := h.Store.Snapshot()
	if len(snap.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %#v", snap.Accounts)
	}
	if got := snap.Accounts[1].ProxyID; got != "cf-1" {
		t.Fatalf("expected new account to use cloudflare proxy, got %q", got)
	}
}
