package client

import (
	"testing"
	"time"

	"ds2api/internal/account"
	"ds2api/internal/auth"
	"ds2api/internal/config"
)

func newAccountHealthClientForTest(t *testing.T) (*Client, *account.Pool, *auth.RequestAuth) {
	t.Helper()
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["k1"],
		"proxies":[
			{"id":"proxy-a","type":"socks5h","host":"127.0.0.1","port":1080},
			{"id":"proxy-b","type":"socks5h","host":"127.0.0.1","port":1081}
		],
		"accounts":[{"email":"acc1@example.com","token":"token1","proxy_id":"proxy-a"}]
	}`)
	store := config.LoadStore()
	pool := account.NewPool(store)
	resolver := auth.NewResolver(store, pool, nil)
	client := NewClient(store, resolver)
	acc, ok := store.FindAccount("acc1@example.com")
	if !ok {
		t.Fatal("test account not found")
	}
	return client, pool, &auth.RequestAuth{
		UseConfigToken: true,
		AccountID:      acc.Identifier(),
		Account:        acc,
	}
}

func TestRecordAccountEmptyOutputCoolsAfterThreshold(t *testing.T) {
	client, pool, requestAuth := newAccountHealthClientForTest(t)

	for i := 0; i < accountEmptyOutputThreshold-1; i++ {
		client.RecordAccountEmptyOutput(requestAuth, "test")
	}
	if got := pool.Status()["cooling_down"]; got != 0 {
		t.Fatalf("expected no cooldown before threshold, got %#v", got)
	}

	client.RecordAccountEmptyOutput(requestAuth, "test")
	if got := pool.Status()["cooling_down"]; got != 1 {
		t.Fatalf("expected cooldown at threshold, got %#v", got)
	}
}

func TestRecordAccountVisibleSuccessDecaysEmptyOutputPenalty(t *testing.T) {
	client, pool, requestAuth := newAccountHealthClientForTest(t)

	// 1 failure — not enough to cool
	client.RecordAccountEmptyOutput(requestAuth, "test")
	if got := pool.Status()["cooling_down"]; got != 0 {
		t.Fatalf("expected 1 failure to not cooldown, got %#v", got)
	}

	// 2nd failure → cooldown (new threshold=2)
	client.RecordAccountEmptyOutput(requestAuth, "test")
	if got := pool.Status()["cooling_down"]; got != 1 {
		t.Fatalf("expected 2 failures to cooldown, got %#v", got)
	}
}

func TestRecordAccountEmptyOutputSuppressesGlobalCooldownStorm(t *testing.T) {
	client, pool, requestAuth := newAccountHealthClientForTest(t)
	client.accountEmptyOutputGlobal.cooldowns = accountEmptyOutputGlobalMaxCooldown
	client.accountEmptyOutputGlobal.windowStart = time.Now()

	for i := 0; i < accountEmptyOutputThreshold; i++ {
		client.RecordAccountEmptyOutput(requestAuth, "test")
	}

	if got := pool.Status()["cooling_down"]; got != 0 {
		t.Fatalf("expected global budget to suppress cooldown, got %#v", got)
	}
}
