package config

import "testing"

func TestStoreCurrentInputFileAccessors(t *testing.T) {
	store := &Store{cfg: Config{}}
	if !store.CurrentInputFileEnabled() {
		t.Fatal("expected current input file enabled by default")
	}
	if got := store.CurrentInputFileMinChars(); got != 0 {
		t.Fatalf("default current input file min_chars=%d want=0", got)
	}

	enabled := false
	store.cfg.CurrentInputFile = CurrentInputFileConfig{Enabled: &enabled, MinChars: 12345}
	if store.CurrentInputFileEnabled() {
		t.Fatal("expected current input file disabled")
	}

	enabled = true
	store.cfg.CurrentInputFile.Enabled = &enabled
	if !store.CurrentInputFileEnabled() {
		t.Fatal("expected current input file enabled")
	}
	if got := store.CurrentInputFileMinChars(); got != 12345 {
		t.Fatalf("current input file min_chars=%d want=12345", got)
	}
}

func TestStoreThinkingInjectionAccessors(t *testing.T) {
	store := &Store{cfg: Config{}}
	if !store.ThinkingInjectionEnabled() {
		t.Fatal("expected thinking injection enabled by default")
	}

	disabled := false
	store.cfg.ThinkingInjection.Enabled = &disabled
	if store.ThinkingInjectionEnabled() {
		t.Fatal("expected thinking injection disabled by explicit config")
	}

	store.cfg.ThinkingInjection.Prompt = "  custom thinking prompt  "
	if got := store.ThinkingInjectionPrompt(); got != "custom thinking prompt" {
		t.Fatalf("thinking injection prompt=%q want custom thinking prompt", got)
	}
}

func TestStoreProxyAndAccountCountAccessors(t *testing.T) {
	store := &Store{cfg: Config{
		Keys:     []string{"k1"},
		APIKeys:  []APIKey{{Key: "k1", Name: "primary"}},
		Accounts: []Account{{Email: "a@example.com"}, {Email: "b@example.com"}},
		Proxies: []Proxy{{
			ID:   "proxy-a",
			Type: "socks5h",
			Host: "127.0.0.1",
			Port: 1080,
		}},
	}}
	store.rebuildIndexes()

	if got := store.AccountCount(); got != 2 {
		t.Fatalf("AccountCount=%d want=2", got)
	}
	if keys := store.Keys(); len(keys) != 1 || keys[0] != "k1" {
		t.Fatalf("unexpected keys: %#v", keys)
	}
	if apiKeys := store.APIKeys(); len(apiKeys) != 1 || apiKeys[0].Name != "primary" {
		t.Fatalf("unexpected api keys: %#v", apiKeys)
	}
	if got := store.RuntimeAccountCount(); got != 2 {
		t.Fatalf("RuntimeAccountCount=%d want=2", got)
	}
	proxy, ok := store.FindProxy("proxy-a")
	if !ok {
		t.Fatal("expected proxy-a to be found")
	}
	if proxy.Type != "socks5h" || proxy.Host != "127.0.0.1" {
		t.Fatalf("unexpected proxy: %#v", proxy)
	}
	proxies := store.Proxies()
	if len(proxies) != 1 || proxies[0].ID != "proxy-a" {
		t.Fatalf("unexpected proxies: %#v", proxies)
	}
}

func TestStoreAccountsPageAvoidsFullSnapshotAndSupportsMobileAlias(t *testing.T) {
	t.Setenv("DS2API_CONFIG_JSON", `{
		"keys":["k1"],
		"accounts":[
			{"email":"a@example.com","password":"p"},
			{"email":"b@example.com","password":"p","remark":"needle"},
			{"mobile":"+8613800138000","password":"p"}
		]
	}`)
	store := LoadStore()

	page, total := store.AccountsPage(1, 2, "")
	if total != 3 {
		t.Fatalf("unexpected total: %d", total)
	}
	if len(page) != 2 || page[0].Identifier() != "+8613800138000" || page[1].Identifier() != "b@example.com" {
		t.Fatalf("unexpected newest-first page: %#v", page)
	}

	filtered, total := store.AccountsPage(1, 10, "needle")
	if total != 1 || len(filtered) != 1 || filtered[0].Identifier() != "b@example.com" {
		t.Fatalf("unexpected filtered page total=%d items=%#v", total, filtered)
	}

	acc, ok := store.FindAccount("13800138000")
	if !ok || acc.Identifier() != "+8613800138000" {
		t.Fatalf("expected canonical mobile lookup, got ok=%v acc=%#v", ok, acc)
	}
}
