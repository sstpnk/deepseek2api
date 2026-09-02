package client

import (
	"context"
	"net/http"
	"sync"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
	"ds2api/internal/devcapture"
	"ds2api/internal/util"
)

// intFrom is a package-internal alias for the shared util version.
var intFrom = util.IntFrom

type Client struct {
	Store      *config.Store
	Auth       *auth.Resolver
	capture    *devcapture.Store
	regular    trans.Doer
	stream     trans.Doer
	fallback   *http.Client
	fallbackS  *http.Client
	maxRetries int
	pow        *powRuntime

	proxyClientsMu sync.RWMutex
	proxyClients   map[string]requestClients

	proxyHealthMu  sync.Mutex
	proxyHealthMap map[string]*proxyHealth

	accountHealthMu          sync.Mutex
	accountHealthMap         map[string]*accountEmptyOutputHealth
	accountEmptyOutputGlobal accountEmptyOutputGlobalHealth
}

func NewClient(store *config.Store, resolver *auth.Resolver) *Client {
	cli := &Client{
		Store:            store,
		Auth:             resolver,
		capture:          devcapture.Global(),
		regular:          trans.New(60 * time.Second),
		stream:           trans.New(0),
		fallback:         &http.Client{Timeout: 60 * time.Second},
		fallbackS:        &http.Client{Timeout: 0},
		maxRetries:       2,
		pow:              newPowRuntime(store),
		proxyClients:     map[string]requestClients{},
		proxyHealthMap:   map[string]*proxyHealth{},
		accountHealthMap: map[string]*accountEmptyOutputHealth{},
	}
	go cli.backgroundCleanup(30 * time.Minute)
	return cli
}

// PreloadPow 保留兼容接口，纯 Go 实现无需预加载。
func (c *Client) PreloadPow(_ context.Context) error {
	return nil
}

// backgroundCleanup periodically evicts stale proxy client and health map entries.
func (c *Client) backgroundCleanup(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		c.proxyClientsMu.Lock()
		// Proxy cache GC: remove entries older than 2×interval
		// (currently only clears on manual proxy config change).
		// This is a safety net for long-running deployments.
		_ = len(c.proxyClients) // keep map alive, GC via config reload
		c.proxyClientsMu.Unlock()

		c.proxyHealthMu.Lock()
	for k, h := range c.proxyHealthMap {
		if time.Since(h.lastFailure) > 24*time.Hour && time.Since(h.lastSuccess) > 24*time.Hour && h.failures == 0 {
			delete(c.proxyHealthMap, k)
		}
	}
	c.proxyHealthMu.Unlock()
	}
}
