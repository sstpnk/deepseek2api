package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBulkImportLinesCreatesAccountsAndProxies(t *testing.T) {
	router := newHTTPAdminHarness(t, `{}`, &testingDSMock{})

	body := []byte(`{
        "lines": "JeremyMoon2391@outlook.com:WnIli0JJ00LXkH\nJamieMartinez6433@outlook.com:i4FFpVPMecmafs",
        "auto_proxy": {
            "enabled": true,
            "type": "socks5",
            "host": "172.20.0.1",
            "port": 21345,
            "username_template": "Default.{local}",
            "password": "inliverBAIPIAO",
            "name_template": "resin-{local}"
        }
    }`)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts/bulk-import", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("expected success=true, got %#v", payload)
	}
	if payload["imported_accounts"].(float64) != 2 {
		t.Fatalf("expected imported_accounts=2, got %v", payload["imported_accounts"])
	}
	if payload["imported_proxies"].(float64) != 2 {
		t.Fatalf("expected imported_proxies=2, got %v", payload["imported_proxies"])
	}

	// Verify the account list now includes both with proxy_id set
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodGet, "/accounts?page_size=10", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d", rec.Code)
	}
	var listPayload map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &listPayload)
	items, _ := listPayload["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(items))
	}
	for _, raw := range items {
		acc, _ := raw.(map[string]any)
		if pid, _ := acc["proxy_id"].(string); strings.TrimSpace(pid) == "" {
			t.Fatalf("account %v expected proxy_id, got empty", acc["email"])
		}
	}
}

func TestBulkImportUsesCloudflareProxyWhenInCloudflareMode(t *testing.T) {
	router := newHTTPAdminHarness(t, `{
		"proxies":[
			{"id":"resin-a","name":"resin-u","type":"socks5","host":"161.118.201.199","port":21345,"username":"Default.u","password":"secret"},
			{"id":"cf-1","name":"CF Worker","type":"cloudflare","host":"worker.example.workers.dev","port":443,"worker_host":"worker.example.workers.dev"}
		],
		"accounts":[{"email":"u@example.com","password":"pwd","proxy_id":"cf-1"}],
		"proxy_switch":{"cf_proxy_id":"cf-1"}
	}`, &testingDSMock{})

	body := []byte(`{
        "lines": "new@example.com:pwd",
        "auto_proxy": {
            "enabled": true,
            "type": "socks5",
            "host": "172.20.0.1",
            "port": 21345,
            "username_template": "Default.{local}",
            "password": "secret",
            "name_template": "resin-{local}"
        }
    }`)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts/bulk-import", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["imported_accounts"].(float64) != 1 {
		t.Fatalf("expected imported_accounts=1, got %v", payload["imported_accounts"])
	}
	if payload["imported_proxies"].(float64) != 0 {
		t.Fatalf("expected imported_proxies=0 in cloudflare mode, got %v", payload["imported_proxies"])
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodGet, "/accounts?page_size=10", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d", rec.Code)
	}
	var listPayload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	items, _ := listPayload["items"].([]any)
	for _, raw := range items {
		acc, _ := raw.(map[string]any)
		if acc["email"] == "new@example.com" {
			if got, _ := acc["proxy_id"].(string); got != "cf-1" {
				t.Fatalf("expected imported account to use cloudflare proxy, got %q", got)
			}
			return
		}
	}
	t.Fatalf("imported account not found in list: %#v", items)
}

func TestBulkImportSkipsExistingEmail(t *testing.T) {
	router := newHTTPAdminHarness(t, `{
        "accounts":[{"email":"existing@outlook.com","password":"old"}]
    }`, &testingDSMock{})

	body := []byte(`{
        "lines": "existing@outlook.com:newpwd\nfresh@outlook.com:abc123",
        "auto_proxy": {"enabled": false}
    }`)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts/bulk-import", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	if payload["imported_accounts"].(float64) != 1 {
		t.Fatalf("expected imported_accounts=1, got %v", payload["imported_accounts"])
	}
	skipped, _ := payload["skipped"].([]any)
	if len(skipped) != 1 {
		t.Fatalf("expected one skip entry, got %#v", skipped)
	}
}

func TestBulkImportRejectsEmptyLines(t *testing.T) {
	router := newHTTPAdminHarness(t, `{}`, &testingDSMock{})

	body := []byte(`{"lines":"   \n   ","auto_proxy":{"enabled":false}}`)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, adminReq(http.MethodPost, "/accounts/bulk-import", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestParseBulkImportLinesAcceptsTabAndSpaceSeparators(t *testing.T) {
	blob := "a@x.com:p1\nb@x.com\tp2\nc@x.com p3 with spaces\n# comment line\n"
	got := parseBulkImportLines(blob)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %#v", len(got), got)
	}
	if got[0].Email != "a@x.com" || got[0].Password != "p1" {
		t.Fatalf("colon parse wrong: %#v", got[0])
	}
	if got[1].Email != "b@x.com" || got[1].Password != "p2" {
		t.Fatalf("tab parse wrong: %#v", got[1])
	}
	if got[2].Email != "c@x.com" || got[2].Password != "p3 with spaces" {
		t.Fatalf("space parse wrong: %#v", got[2])
	}
}
