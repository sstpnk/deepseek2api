package cfworker

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildWorkerUploadBodyUsesModuleMultipartMetadata(t *testing.T) {
	body, contentType, err := buildWorkerUploadBody([]byte("export default {}"))
	if err != nil {
		t.Fatalf("buildWorkerUploadBody returned error: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data; boundary=") {
		t.Fatalf("expected multipart content type, got %q", contentType)
	}

	reader := multipart.NewReader(bytes.NewReader(body), strings.TrimPrefix(contentType, "multipart/form-data; boundary="))
	parts := map[string]struct {
		filename    string
		contentType string
		body        string
	}{}
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(part); err != nil {
			t.Fatalf("read part %q: %v", part.FormName(), err)
		}
		parts[part.FormName()] = struct {
			filename    string
			contentType string
			body        string
		}{
			filename:    part.FileName(),
			contentType: part.Header.Get("Content-Type"),
			body:        buf.String(),
		}
	}

	metadata, ok := parts["metadata"]
	if !ok {
		t.Fatal("expected metadata part")
	}
	if metadata.contentType != "application/json" {
		t.Fatalf("metadata content type = %q", metadata.contentType)
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(metadata.body), &meta); err != nil {
		t.Fatalf("metadata is not JSON: %v", err)
	}
	if meta["main_module"] != workerModuleName {
		t.Fatalf("main_module = %q, want %q", meta["main_module"], workerModuleName)
	}
	if meta["compatibility_date"] == "" {
		t.Fatal("expected compatibility_date")
	}

	filePart, ok := parts[workerModuleName]
	if !ok {
		t.Fatal("expected worker module part")
	}
	if filePart.filename != workerModuleName {
		t.Fatalf("file filename = %q, want %q", filePart.filename, workerModuleName)
	}
	if filePart.contentType != "application/javascript+module" {
		t.Fatalf("file content type = %q", filePart.contentType)
	}
	if filePart.body != "export default {}" {
		t.Fatalf("file body = %q", filePart.body)
	}
}

func TestCFAPIResponseMessageAndSummary(t *testing.T) {
	resp := parseCFResponse([]byte(`{"success":false,"errors":[{"code":10001,"message":"bad request"}],"messages":[]}`))
	if got := resp.messageOr("fallback"); got != "bad request" {
		t.Fatalf("messageOr = %q", got)
	}
	if got := resp.errorSummary(); got != "10001:bad request" {
		t.Fatalf("errorSummary = %q", got)
	}
}

func TestTruncateForLog(t *testing.T) {
	if got := truncateForLog("  abc  ", 10); got != "abc" {
		t.Fatalf("unexpected untruncated value %q", got)
	}
	if got := truncateForLog("abcdef", 3); got != "abc...(truncated)" {
		t.Fatalf("unexpected truncated value %q", got)
	}
}

func TestGetSubdomainNormalizesAccountWorkersDevHost(t *testing.T) {
	originalBase := cfAPIBase
	originalClient := cfHTTPClient
	defer func() {
		cfAPIBase = originalBase
		cfHTTPClient = originalClient
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/accounts/account-id/workers/subdomain" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("unexpected auth header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"subdomain":"acct-subdomain"}}`))
	}))
	defer server.Close()

	cfAPIBase = server.URL
	cfHTTPClient = server.Client()

	host, err := getSubdomain("token", "account-id")
	if err != nil {
		t.Fatalf("getSubdomain returned error: %v", err)
	}
	if host != "acct-subdomain.workers.dev" {
		t.Fatalf("host = %q", host)
	}
}
