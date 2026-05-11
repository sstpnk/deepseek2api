package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"ds2api/internal/config"
	dsprotocol "ds2api/internal/deepseek/protocol"
)

func TestPostJSONWithStatusUsesProvidedFallbackClient(t *testing.T) {
	var fallbackCalled bool
	client := &Client{}
	primary := failingDoer{err: errors.New("primary failed")}
	fallbackDoer := doerFunc(func(req *http.Request) (*http.Response, error) {
		fallbackCalled = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Request:    req,
		}, nil
	})

	resp, status, err := client.postJSONWithStatus(
		context.Background(),
		primary,
		fallbackDoer,
		"https://example.com/api",
		map[string]string{"x-test": "1"},
		map[string]any{"foo": "bar"},
	)
	if err != nil {
		t.Fatalf("postJSONWithStatus error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status=%d want=%d", status, http.StatusOK)
	}
	if !fallbackCalled {
		t.Fatal("expected provided fallback doer to be called")
	}
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("unexpected response body: %#v", resp)
	}
}

func TestJSONHeadersDoesNotMutateInput(t *testing.T) {
	client := &Client{}
	headers := map[string]string{"x-test": "1"}

	out := client.jsonHeaders(headers)

	if _, ok := headers["Content-Type"]; ok {
		t.Fatalf("jsonHeaders mutated input headers: %#v", headers)
	}
	if out["Content-Type"] != "application/json" {
		t.Fatalf("missing JSON content type: %#v", out)
	}
	if out["x-test"] != "1" {
		t.Fatalf("missing copied header: %#v", out)
	}
}

func TestPostJSONWithStatusAllowsConcurrentSharedHeaders(t *testing.T) {
	client := &Client{}
	sharedHeaders := map[string]string{"x-shared": "1"}
	doer := doerFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type=%q want application/json", got)
		}
		if got := req.Header.Get("x-shared"); got != "1" {
			t.Errorf("x-shared=%q want 1", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Request:    req,
		}, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, status, err := client.postJSONWithStatus(
				context.Background(),
				doer,
				doer,
				"https://example.com/api",
				sharedHeaders,
				map[string]any{"foo": "bar"},
			)
			if err != nil {
				t.Errorf("postJSONWithStatus error: %v", err)
				return
			}
			if status != http.StatusOK {
				t.Errorf("status=%d want=%d", status, http.StatusOK)
			}
			if ok, _ := resp["ok"].(bool); !ok {
				t.Errorf("unexpected response body: %#v", resp)
			}
		}()
	}
	wg.Wait()

	if _, ok := sharedHeaders["Content-Type"]; ok {
		t.Fatalf("postJSONWithStatus mutated shared headers: %#v", sharedHeaders)
	}
}

func TestLoginAllowsConcurrentBaseHeaders(t *testing.T) {
	client := &Client{
		regular: loginDoer{t: t},
	}

	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			token, err := client.Login(context.Background(), config.Account{
				Email:    fmt.Sprintf("user-%d@example.com", i),
				Password: "password",
			})
			if err != nil {
				t.Errorf("Login error: %v", err)
				return
			}
			if token != "token" {
				t.Errorf("token=%q want token", token)
			}
		}(i)
	}
	wg.Wait()
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

type loginDoer struct {
	t *testing.T
}

func (d loginDoer) Do(req *http.Request) (*http.Response, error) {
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		d.t.Errorf("Content-Type=%q want application/json", got)
	}
	if got := req.Header.Get("User-Agent"); got != dsprotocol.BaseHeaders["User-Agent"] {
		d.t.Errorf("User-Agent=%q want %q", got, dsprotocol.BaseHeaders["User-Agent"])
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{
			"code": 0,
			"data": {
				"biz_code": 0,
				"biz_data": {
					"user": {"token": "token"}
				}
			}
		}`)),
		Request: req,
	}, nil
}
