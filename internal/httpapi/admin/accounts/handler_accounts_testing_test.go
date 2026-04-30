package accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

type testingDSMock struct {
	loginCalls                 int
	createSessionCalls         int
	getPowCalls                int
	callCompletionCalls        int
	deleteAllSessionsCalls     int
	deleteAllSessionsError     error
	deleteAllSessionsErrorOnce bool
}

func (m *testingDSMock) Login(_ context.Context, _ config.Account) (string, error) {
	m.loginCalls++
	return "new-token", nil
}

func (m *testingDSMock) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	m.createSessionCalls++
	return "session-id", nil
}

func (m *testingDSMock) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	m.getPowCalls++
	return "", errors.New("should not call GetPow in this test")
}

func (m *testingDSMock) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ map[string]any, _ string, _ int) (*http.Response, error) {
	m.callCompletionCalls++
	return nil, errors.New("should not call CallCompletion in this test")
}

func (m *testingDSMock) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	m.deleteAllSessionsCalls++
	if m.deleteAllSessionsError != nil {
		err := m.deleteAllSessionsError
		if m.deleteAllSessionsErrorOnce {
			m.deleteAllSessionsError = nil
		}
		return err
	}
	return nil
}

func (m *testingDSMock) GetSessionCountForToken(_ context.Context, _ string) (*dsclient.SessionStats, error) {
	return &dsclient.SessionStats{Success: true}, nil
}

func TestTestAccount_BatchModeOnlyCreatesSession(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"batch@example.com","password":"pwd","token":""}]}`)
	store := config.LoadStore()
	ds := &testingDSMock{}
	h := &Handler{Store: store, DS: ds}
	acc, ok := store.FindAccount("batch@example.com")
	if !ok {
		t.Fatal("expected test account")
	}

	result := h.testAccount(context.Background(), acc, "deepseek-v4-flash", "")

	if ok, _ := result["success"].(bool); !ok {
		t.Fatalf("expected success=true, got %#v", result)
	}
	msg, _ := result["message"].(string)
	if !strings.Contains(msg, "Token 刷新成功") {
		t.Fatalf("expected session-only success message, got %q", msg)
	}
	if ds.loginCalls != 1 || ds.createSessionCalls != 1 {
		t.Fatalf("unexpected Login/CreateSession calls: login=%d createSession=%d", ds.loginCalls, ds.createSessionCalls)
	}
	if ds.getPowCalls != 0 || ds.callCompletionCalls != 0 {
		t.Fatalf("expected no completion flow calls, got getPow=%d callCompletion=%d", ds.getPowCalls, ds.callCompletionCalls)
	}
	updated, ok := store.FindAccount("batch@example.com")
	if !ok {
		t.Fatal("expected updated account")
	}
	if updated.Token != "new-token" {
		t.Fatalf("expected refreshed token to be persisted, got %q", updated.Token)
	}
	testStatus, ok := store.AccountTestStatus("batch@example.com")
	if !ok || testStatus != "ok" {
		t.Fatalf("expected runtime test status ok, got %q (ok=%v)", testStatus, ok)
	}
}

func TestDeleteAllSessions_RetryWithReloginOnDeleteFailure(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"batch@example.com","password":"pwd","token":"expired-token"}]}`)
	store := config.LoadStore()
	ds := &testingDSMock{deleteAllSessionsError: errors.New("token expired"), deleteAllSessionsErrorOnce: true}
	h := &Handler{Store: store, DS: ds}

	req := httptest.NewRequest(http.MethodPost, "/delete-all", bytes.NewBufferString(`{"identifier":"batch@example.com"}`))
	rec := httptest.NewRecorder()
	h.deleteAllSessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if ok, _ := resp["success"].(bool); !ok {
		t.Fatalf("expected success response, got %#v", resp)
	}
	if ds.loginCalls != 2 {
		t.Fatalf("expected initial login plus relogin, got %d", ds.loginCalls)
	}
	if ds.deleteAllSessionsCalls != 2 {
		t.Fatalf("expected delete called twice, got %d", ds.deleteAllSessionsCalls)
	}
	updated, ok := store.FindAccount("batch@example.com")
	if !ok {
		t.Fatal("expected account")
	}
	if updated.Token != "new-token" {
		t.Fatalf("expected refreshed token persisted, got %q", updated.Token)
	}
}

type completionPayloadDSMock struct {
	payload map[string]any
}

func (m *completionPayloadDSMock) Login(_ context.Context, _ config.Account) (string, error) {
	return "new-token", nil
}

func (m *completionPayloadDSMock) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "session-id", nil
}

func (m *completionPayloadDSMock) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "pow-ok", nil
}

func (m *completionPayloadDSMock) CallCompletion(_ context.Context, _ *auth.RequestAuth, payload map[string]any, _ string, _ int) (*http.Response, error) {
	m.payload = payload
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("data: {\"v\":\"ok\"}\n\ndata: [DONE]\n\n")),
	}, nil
}

func (m *completionPayloadDSMock) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	return nil
}

func (m *completionPayloadDSMock) GetSessionCountForToken(_ context.Context, _ string) (*dsclient.SessionStats, error) {
	return &dsclient.SessionStats{Success: true}, nil
}

func TestTestAccount_MessageModeUsesExpertModelTypeForExpertModel(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"batch@example.com","password":"pwd","token":"seed-token"}]}`)
	store := config.LoadStore()
	ds := &completionPayloadDSMock{}
	h := &Handler{Store: store, DS: ds}
	acc, ok := store.FindAccount("batch@example.com")
	if !ok {
		t.Fatal("expected test account")
	}

	result := h.testAccount(context.Background(), acc, "deepseek-v4-pro", "hello")

	if ok, _ := result["success"].(bool); !ok {
		t.Fatalf("expected success=true, got %#v", result)
	}
	if got := ds.payload["model_type"]; got != "expert" {
		t.Fatalf("expected model_type expert, got %#v", got)
	}
	if got := ds.payload["chat_session_id"]; got != "session-id" {
		t.Fatalf("unexpected chat_session_id: %#v", got)
	}
}

func TestTestAccount_MessageModeUsesVisionModelTypeForVisionModel(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{"accounts":[{"email":"batch@example.com","password":"pwd","token":"seed-token"}]}`)
	store := config.LoadStore()
	ds := &completionPayloadDSMock{}
	h := &Handler{Store: store, DS: ds}
	acc, ok := store.FindAccount("batch@example.com")
	if !ok {
		t.Fatal("expected test account")
	}

	result := h.testAccount(context.Background(), acc, "deepseek-v4-vision", "hello")

	if ok, _ := result["success"].(bool); !ok {
		t.Fatalf("expected success=true, got %#v", result)
	}
	if got := ds.payload["model_type"]; got != "vision" {
		t.Fatalf("expected model_type vision, got %#v", got)
	}
}

// alwaysFailLoginDSMock simulates a permanently broken account: every Login
// returns an authentication error. Used to verify auto-delete confirms the
// account is unusable via the two-strike Login probe before deleting.
type alwaysFailLoginDSMock struct {
	loginCalls int
}

func (m *alwaysFailLoginDSMock) Login(_ context.Context, _ config.Account) (string, error) {
	m.loginCalls++
	return "", errors.New("login failed: invalid credentials")
}

func (m *alwaysFailLoginDSMock) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "", errors.New("create session should not be reached")
}

func (m *alwaysFailLoginDSMock) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "", errors.New("get pow should not be reached")
}

func (m *alwaysFailLoginDSMock) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ map[string]any, _ string, _ int) (*http.Response, error) {
	return nil, errors.New("completion should not be reached")
}

func (m *alwaysFailLoginDSMock) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	return nil
}

func (m *alwaysFailLoginDSMock) GetSessionCountForToken(_ context.Context, _ string) (*dsclient.SessionStats, error) {
	return nil, errors.New("session count should not be reached")
}

func TestTestSingleAccount_AutoDeleteRemovesAccountWhenLoginFailsTwice(t *testing.T) {
	srv := newHTTPAdminHarness(t, `{"accounts":[{"email":"dead@example.com","password":"pwd","token":""}]}`, &alwaysFailLoginDSMock{})

	body := []byte(`{"identifier":"dead@example.com","auto_delete":true}`)
	req := adminReq(http.MethodPost, "/accounts/test", body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("expected success=false for failing login, got %#v", resp)
	}
	if got, _ := resp["verified_unusable"].(bool); !got {
		t.Fatalf("expected verified_unusable=true, got %#v", resp)
	}
	// New auto-delete contract: account is moved to quarantine, NOT deleted
	// outright. The background sweeper takes over from here.
	if got, _ := resp["quarantined"].(bool); !got {
		t.Fatalf("expected quarantined=true after two-strike login fail, got %#v", resp)
	}
	if got, _ := resp["deleted"].(bool); got {
		t.Fatalf("expected deleted=false (quarantine replaces immediate delete), got %#v", resp)
	}

	// Account should be gone from the active listing — it's in quarantine.
	listReq := adminReq(http.MethodGet, "/accounts?page=1&page_size=10", nil)
	listRec := httptest.NewRecorder()
	srv.ServeHTTP(listRec, listReq)
	var listResp map[string]any
	_ = json.Unmarshal(listRec.Body.Bytes(), &listResp)
	if total, _ := listResp["total"].(float64); total != 0 {
		t.Fatalf("expected zero active accounts after quarantine, got total=%v body=%s", total, listRec.Body.String())
	}

	// And it must show up under /accounts/quarantine with failures=0 and a
	// remaining_attempts of QuarantineMaxFailures.
	qReq := adminReq(http.MethodGet, "/accounts/quarantine", nil)
	qRec := httptest.NewRecorder()
	srv.ServeHTTP(qRec, qReq)
	var qResp map[string]any
	_ = json.Unmarshal(qRec.Body.Bytes(), &qResp)
	if total, _ := qResp["total"].(float64); total != 1 {
		t.Fatalf("expected 1 quarantined account, got total=%v body=%s", total, qRec.Body.String())
	}
	items, _ := qResp["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected items[0], got %#v", items)
	}
	first := items[0].(map[string]any)
	if got, _ := first["failures"].(float64); got != 0 {
		t.Fatalf("expected failures=0 on entry, got %v", got)
	}
	if got, _ := first["remaining_attempts"].(float64); got != float64(QuarantineMaxFailures) {
		t.Fatalf("expected remaining_attempts=%d, got %v", QuarantineMaxFailures, got)
	}
}

// flakyLoginDSMock fails the first Login call (so testAccount returns failed)
// but succeeds on the verification probes. Used to ensure transient failures
// do not trigger auto-delete: the two-strike check requires BOTH probes to
// fail.
type flakyLoginDSMock struct {
	loginCalls int
}

func (m *flakyLoginDSMock) Login(_ context.Context, _ config.Account) (string, error) {
	m.loginCalls++
	if m.loginCalls == 1 {
		return "", errors.New("login failed: transient error")
	}
	return "good-token", nil
}

func (m *flakyLoginDSMock) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "session-id", nil
}

func (m *flakyLoginDSMock) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "", errors.New("get pow should not be reached")
}

func (m *flakyLoginDSMock) CallCompletion(_ context.Context, _ *auth.RequestAuth, _ map[string]any, _ string, _ int) (*http.Response, error) {
	return nil, errors.New("completion should not be reached")
}

func (m *flakyLoginDSMock) DeleteAllSessionsForToken(_ context.Context, _ string) error {
	return nil
}

func (m *flakyLoginDSMock) GetSessionCountForToken(_ context.Context, _ string) (*dsclient.SessionStats, error) {
	return &dsclient.SessionStats{Success: true}, nil
}

func TestTestSingleAccount_AutoDeleteSkippedWhenVerificationLoginSucceeds(t *testing.T) {
	srv := newHTTPAdminHarness(t, `{"accounts":[{"email":"flaky@example.com","password":"pwd","token":""}]}`, &flakyLoginDSMock{})

	body := []byte(`{"identifier":"flaky@example.com","auto_delete":true}`)
	req := adminReq(http.MethodPost, "/accounts/test", body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if got, _ := resp["quarantined"].(bool); got {
		t.Fatalf("expected NOT quarantined when verification login succeeds, got %#v", resp)
	}
	if got, _ := resp["verified_unusable"].(bool); got {
		t.Fatalf("expected verified_unusable=false when verification recovers, got %#v", resp)
	}

	// Account should still be present in the active list.
	listReq := adminReq(http.MethodGet, "/accounts?page=1&page_size=10", nil)
	listRec := httptest.NewRecorder()
	srv.ServeHTTP(listRec, listReq)
	var listResp map[string]any
	_ = json.Unmarshal(listRec.Body.Bytes(), &listResp)
	if total, _ := listResp["total"].(float64); total != 1 {
		t.Fatalf("expected account preserved when verification recovers, got total=%v", total)
	}
}

func TestTestAllAccounts_AutoDeleteRemovesOnlyConfirmedDeadAccounts(t *testing.T) {
	const cfg = `{"accounts":[
		{"email":"dead1@example.com","password":"pwd","token":""},
		{"email":"dead2@example.com","password":"pwd","token":""}
	]}`
	srv := newHTTPAdminHarness(t, cfg, &alwaysFailLoginDSMock{})

	body := []byte(`{"auto_delete":true,"concurrency":4}`)
	req := adminReq(http.MethodPost, "/accounts/test-all", body)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	// New contract: confirmed-dead accounts go to quarantine, not the trash.
	// `deleted` is always 0 from /test-all; the sweeper does the real delete.
	if got, _ := resp["quarantined"].(float64); got != 2 {
		t.Fatalf("expected quarantined=2, got %v body=%s", got, rec.Body.String())
	}
	if got, _ := resp["deleted"].(float64); got != 0 {
		t.Fatalf("expected deleted=0 (sweeper owns deletions now), got %v", got)
	}
	if got, _ := resp["concurrency"].(float64); got != 4 {
		t.Fatalf("expected concurrency=4 in response, got %v", got)
	}
	listReq := adminReq(http.MethodGet, "/accounts?page=1&page_size=10", nil)
	listRec := httptest.NewRecorder()
	srv.ServeHTTP(listRec, listReq)
	var listResp map[string]any
	_ = json.Unmarshal(listRec.Body.Bytes(), &listResp)
	if total, _ := listResp["total"].(float64); total != 0 {
		t.Fatalf("expected both accounts moved out of active list, got total=%v", total)
	}
	qReq := adminReq(http.MethodGet, "/accounts/quarantine", nil)
	qRec := httptest.NewRecorder()
	srv.ServeHTTP(qRec, qReq)
	var qResp map[string]any
	_ = json.Unmarshal(qRec.Body.Bytes(), &qResp)
	if total, _ := qResp["total"].(float64); total != 2 {
		t.Fatalf("expected 2 in quarantine, got total=%v body=%s", total, qRec.Body.String())
	}
}

func TestClampConcurrencyClampsToBounds(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{-5, minRefreshConcurrency},
		{0, minRefreshConcurrency},
		{1, 1},
		{10, 10},
		{20, 20},
		{50, maxRefreshConcurrency},
	}
	for _, tc := range cases {
		if got := clampConcurrency(tc.in); got != tc.want {
			t.Fatalf("clampConcurrency(%d)=%d want %d", tc.in, got, tc.want)
		}
	}
}
