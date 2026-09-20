package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func TestDeviceIDForAccountUsesConfiguredValue(t *testing.T) {
	got := deviceIDForAccount(config.Account{
		Email:    "user@example.com",
		DeviceID: " configured-device ",
	})
	if got != "configured-device" {
		t.Fatalf("deviceIDForAccount()=%q want configured-device", got)
	}
}

func TestDeviceIDForAccountDerivesStableValue(t *testing.T) {
	first := deviceIDForAccount(config.Account{Email: " User@Example.COM "})
	second := deviceIDForAccount(config.Account{Email: "user@example.com"})

	if first != second {
		t.Fatalf("expected stable derived device id, got %q and %q", first, second)
	}
	if first == defaultLoginDeviceID {
		t.Fatalf("derived device id should not use shared fallback %q", defaultLoginDeviceID)
	}
	if len(first) != 16 {
		t.Fatalf("unexpected derived device id shape: %q", first)
	}
	for _, r := range first {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("derived device id should be lowercase hex, got %q", first)
		}
	}
}

func TestLoginSendsConfiguredDeviceID(t *testing.T) {
	var payload map[string]any
	var xDeviceID string
	client := &Client{
		regular: doerFunc(func(req *http.Request) (*http.Response, error) {
			xDeviceID = req.Header.Get("x-device-id")
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("decode request body: %v", err)
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
		}),
	}

	token, err := client.Login(context.Background(), config.Account{
		Email:    "user@example.com",
		Password: "password",
		DeviceID: "configured-device",
	})
	if err != nil {
		t.Fatalf("Login error: %v", err)
	}
	if token != "token" {
		t.Fatalf("token=%q want token", token)
	}
	if payload["device_id"] != "configured-device" {
		t.Fatalf("device_id=%#v want configured-device; payload=%#v", payload["device_id"], payload)
	}
	if xDeviceID != "configured-device" {
		t.Fatalf("x-device-id=%q want configured-device", xDeviceID)
	}
}

func TestAuthHeadersForAuthAddsManagedAccountDeviceID(t *testing.T) {
	client := &Client{}
	headers := client.authHeadersForAuth(&auth.RequestAuth{
		UseConfigToken: true,
		DeepSeekToken:  "token",
		Account: config.Account{
			Email:    "user@example.com",
			DeviceID: "configured-device",
		},
	})

	if headers["authorization"] != "Bearer token" {
		t.Fatalf("authorization=%q want bearer token", headers["authorization"])
	}
	if headers["x-device-id"] != "configured-device" {
		t.Fatalf("x-device-id=%q want configured-device", headers["x-device-id"])
	}
}

func TestAuthHeadersForAuthLeavesDirectTokenWithoutDeviceID(t *testing.T) {
	client := &Client{}
	headers := client.authHeadersForAuth(&auth.RequestAuth{
		UseConfigToken: false,
		DeepSeekToken:  "direct-token",
	})

	if headers["authorization"] != "Bearer direct-token" {
		t.Fatalf("authorization=%q want bearer direct-token", headers["authorization"])
	}
	if got := headers["x-device-id"]; got != "" {
		t.Fatalf("x-device-id=%q want empty for direct token", got)
	}
}

func TestExtractCreateSessionIDSupportsLegacyShape(t *testing.T) {
	resp := map[string]any{
		"data": map[string]any{
			"biz_data": map[string]any{
				"id": "legacy-session-id",
			},
		},
	}

	if got := extractCreateSessionID(resp); got != "legacy-session-id" {
		t.Fatalf("expected legacy session id, got %q", got)
	}
}

func TestExtractCreateSessionIDSupportsNestedChatSessionShape(t *testing.T) {
	resp := map[string]any{
		"data": map[string]any{
			"biz_data": map[string]any{
				"chat_session": map[string]any{
					"id":         "nested-session-id",
					"model_type": "default",
				},
			},
		},
	}

	if got := extractCreateSessionID(resp); got != "nested-session-id" {
		t.Fatalf("expected nested session id, got %q", got)
	}
}
