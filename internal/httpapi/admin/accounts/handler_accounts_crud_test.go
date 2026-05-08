package accounts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestListAccountsPageSizeCapIs5000(t *testing.T) {
	accounts := make([]string, 0, 150)
	for i := range 150 {
		accounts = append(accounts, fmt.Sprintf(`{"email":"u%d@example.com","password":"pwd"}`, i))
	}
	raw := fmt.Sprintf(`{"accounts":[%s]}`, strings.Join(accounts, ","))
	router := newHTTPAdminHarness(t, raw, &testingDSMock{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodGet, "/accounts?page=1&page_size=200", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	items, _ := payload["items"].([]any)
	if len(items) != 150 {
		t.Fatalf("expected all 150 accounts with page_size=200, got %d", len(items))
	}
	if ps, _ := payload["page_size"].(float64); ps != 200 {
		t.Fatalf("expected page_size=200 in response, got %v", payload["page_size"])
	}
}

func TestListAccountsPageSizeAbove5000ClampedTo5000(t *testing.T) {
	router := newHTTPAdminHarness(t, `{"accounts":[{"email":"u@example.com","password":"pwd"}]}`, &testingDSMock{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodGet, "/accounts?page=1&page_size=9999", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if ps, _ := payload["page_size"].(float64); ps != 5000 {
		t.Fatalf("expected page_size clamped to 5000, got %v", payload["page_size"])
	}
}

func TestUpdateAccountMetadataPreservesCredentials(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"accounts":[{"email":"u@example.com","name":"old name","remark":"old remark","password":"secret"}]
	}`)

	r := chi.NewRouter()
	r.Put("/admin/accounts/{identifier}", h.updateAccount)

	body := []byte(`{"name":"new name","remark":"new remark"}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/accounts/u@example.com", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	snap := h.Store.Snapshot()
	if len(snap.Accounts) != 1 {
		t.Fatalf("unexpected accounts after update: %#v", snap.Accounts)
	}
	acc := snap.Accounts[0]
	if acc.Email != "u@example.com" {
		t.Fatalf("identifier changed unexpectedly: %#v", acc)
	}
	if acc.Name != "new name" || acc.Remark != "new remark" {
		t.Fatalf("metadata update did not persist: %#v", acc)
	}
	if acc.Password != "secret" {
		t.Fatalf("password should be preserved, got %#v", acc)
	}
}

func TestAddAccountUsesCloudflareProxyWhenInCloudflareMode(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"proxies":[
			{"id":"resin-a","name":"resin-u","type":"socks5","host":"161.118.201.199","port":21345,"username":"Default.u","password":"secret"},
			{"id":"cf-1","name":"CF Worker","type":"cloudflare","host":"worker.example.workers.dev","port":443,"worker_host":"worker.example.workers.dev"}
		],
		"accounts":[{"email":"u@example.com","password":"pwd","proxy_id":"cf-1"}],
		"proxy_switch":{"cf_proxy_id":"cf-1"}
	}`)

	r := chi.NewRouter()
	r.Post("/admin/accounts", h.addAccount)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/accounts", bytes.NewBufferString(`{"email":"new@example.com","password":"pwd"}`)))
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

func TestListAccountsMasksTokenPreview(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"accounts":[{"email":"u@example.com","password":"pwd"}]
	}`)
	if err := h.Store.UpdateAccountToken("u@example.com", "abcdefgh"); err != nil {
		t.Fatalf("seed runtime token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/accounts?page=1&page_size=10", nil)
	rec := httptest.NewRecorder()
	h.listAccounts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	items, _ := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	first, _ := items[0].(map[string]any)
	if got, _ := first["token_preview"].(string); got != "ab****gh" {
		t.Fatalf("expected masked token preview, got %q", got)
	}
}
