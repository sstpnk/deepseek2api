package config

import "testing"

func TestValidateProxyConfigAllowsCloudflareWorkerProxy(t *testing.T) {
	err := ValidateProxyConfig([]Proxy{{
		Type:       "cloudflare",
		Host:       "ds2api-proxy.example.workers.dev",
		Port:       443,
		WorkerHost: "ds2api-proxy.example.workers.dev",
	}})
	if err != nil {
		t.Fatalf("expected cloudflare proxy config to validate: %v", err)
	}
}
