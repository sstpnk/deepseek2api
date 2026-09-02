package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueueStatusDefaultsToSummary(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"accounts":[{"email":"u@example.com","password":"pwd","token":"tok"}]
	}`)

	req := httptest.NewRequest(http.MethodGet, "/admin/queue/status", nil)
	rec := httptest.NewRecorder()
	h.queueStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if _, ok := payload["available_accounts"]; ok {
		t.Fatalf("expected default queue status to omit available_accounts")
	}
	if got := int(payload["available"].(float64)); got != 1 {
		t.Fatalf("expected available=1, got %d", got)
	}
}

func TestQueueStatusCanIncludeAccountDetails(t *testing.T) {
	h := newAdminTestHandler(t, `{
		"accounts":[{"email":"u@example.com","password":"pwd","token":"tok"}]
	}`)

	req := httptest.NewRequest(http.MethodGet, "/admin/queue/status?include_accounts=1", nil)
	rec := httptest.NewRecorder()
	h.queueStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	accounts, ok := payload["available_accounts"].([]any)
	if !ok || len(accounts) != 1 {
		t.Fatalf("expected available account details, got %#v", payload["available_accounts"])
	}
}
