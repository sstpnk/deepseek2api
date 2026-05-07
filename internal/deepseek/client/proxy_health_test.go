package client

import (
	"testing"
	"time"

	"ds2api/internal/config"
)

func newProxyHealthClientForTest(t *testing.T) *Client {
	t.Helper()
	t.Setenv("DS2API_CONFIG_JSON", `{
		"proxies":[
			{"id":"proxy-a","type":"socks5h","host":"127.0.0.1","port":1080},
			{"id":"proxy-b","type":"socks5h","host":"127.0.0.1","port":1081},
			{"id":"proxy-c","type":"socks5h","host":"127.0.0.1","port":1082}
		],
		"accounts":[{"email":"u@example.com","token":"t","proxy_id":"proxy-a"}]
	}`)
	return NewClient(config.LoadStore(), nil)
}

func TestProxyCooldownGrowsBeyondLegacyFiveMinuteCap(t *testing.T) {
	client := newProxyHealthClientForTest(t)

	for i := 0; i < proxyFailureThreshold*8; i++ {
		client.markProxyFailure("proxy-a")
	}

	client.proxyHealthMu.Lock()
	h := client.proxyHealthMap["proxy-a"]
	remaining := time.Until(h.cooldownUntil)
	client.proxyHealthMu.Unlock()

	if remaining < 55*time.Minute {
		t.Fatalf("expected long cooldown near one hour, got %s", remaining)
	}
}

func TestProxySuccessDecaysCooldownGradually(t *testing.T) {
	client := newProxyHealthClientForTest(t)
	for i := 0; i < proxyFailureThreshold*2; i++ {
		client.markProxyFailure("proxy-a")
	}

	client.markProxySuccess("proxy-a")

	client.proxyHealthMu.Lock()
	h := client.proxyHealthMap["proxy-a"]
	levelAfterOneSuccess := h.cooldownLevel
	onCooldown := time.Now().Before(h.cooldownUntil)
	client.proxyHealthMu.Unlock()

	if onCooldown {
		t.Fatal("expected a success to clear active cooldown")
	}
	if levelAfterOneSuccess == 0 {
		t.Fatal("expected cooldown level to survive one success")
	}

	client.markProxySuccess("proxy-a")
	client.markProxySuccess("proxy-a")

	client.proxyHealthMu.Lock()
	levelAfterRecovery := client.proxyHealthMap["proxy-a"].cooldownLevel
	client.proxyHealthMu.Unlock()

	if levelAfterRecovery >= levelAfterOneSuccess {
		t.Fatalf("expected cooldown level to decay after recovery successes, got before=%d after=%d", levelAfterOneSuccess, levelAfterRecovery)
	}
}

func TestPickHealthyProxyPrefersHigherScoreAndAvoidsRetryProxy(t *testing.T) {
	client := newProxyHealthClientForTest(t)
	for i := 0; i < proxyFailureThreshold*2; i++ {
		client.markProxyFailure("proxy-b")
	}
	client.markProxySuccess("proxy-b")

	for i := 0; i < 20; i++ {
		proxy, ok := client.pickHealthyProxy("proxy-a", map[string]bool{"proxy-c": true})
		if !ok {
			t.Fatal("expected replacement proxy")
		}
		if proxy.ID != "proxy-b" {
			t.Fatalf("expected proxy-b because proxy-c is avoided, got %s", proxy.ID)
		}
	}
}
