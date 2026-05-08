package client

import (
	"context"
	dsprotocol "ds2api/internal/deepseek/protocol"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	trans "ds2api/internal/deepseek/transport"
)

type requestClients struct {
	regular    trans.Doer
	stream     trans.Doer
	fallback   *http.Client // shared between regular and stream paths
	proxyID    string
	workerHost string // set for cloudflare proxies
}

type proxyIDCtxKey struct{}
type avoidedProxyIDsCtxKey struct{}

// withActiveProxyID attaches the proxyID currently in use to ctx so that
// downstream transport-error sites can mark the right proxy unhealthy
// (the active proxy may differ from acc.ProxyID after a soft switch).
func withActiveProxyID(ctx context.Context, proxyID string) context.Context {
	if ctx == nil || proxyID == "" {
		return ctx
	}
	return context.WithValue(ctx, proxyIDCtxKey{}, proxyID)
}

type workerHostCtxKey struct{}

func withWorkerHost(ctx context.Context, host string) context.Context {
	if ctx == nil || host == "" {
		return ctx
	}
	return context.WithValue(ctx, workerHostCtxKey{}, host)
}

func workerHostFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(workerHostCtxKey{}).(string)
	return v
}

func rewriteURLForWorker(ctx context.Context, url string) string {
	wh := workerHostFromContext(ctx)
	if wh == "" {
		return url
	}
	return strings.Replace(url, "chat.deepseek.com", wh, 1)
}

func activeProxyIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(proxyIDCtxKey{}).(string)
	return v
}

func withAvoidedProxyID(ctx context.Context, proxyID string) context.Context {
	if ctx == nil || strings.TrimSpace(proxyID) == "" {
		return ctx
	}
	avoid := map[string]bool{}
	if existing, ok := ctx.Value(avoidedProxyIDsCtxKey{}).(map[string]bool); ok {
		for id, v := range existing {
			avoid[id] = v
		}
	}
	avoid[strings.TrimSpace(proxyID)] = true
	return context.WithValue(ctx, avoidedProxyIDsCtxKey{}, avoid)
}

func avoidedProxyIDsFromContext(ctx context.Context) map[string]bool {
	if ctx == nil {
		return nil
	}
	avoid, _ := ctx.Value(avoidedProxyIDsCtxKey{}).(map[string]bool)
	return avoid
}

type hostLookupFunc func(ctx context.Context, network, host string) ([]string, error)

var proxyConnectivityTestURL = "https://chat.deepseek.com/"

var defaultHostLookup hostLookupFunc = func(ctx context.Context, _ string, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

func proxyDialAddress(ctx context.Context, proxyType, address string, lookup hostLookupFunc) (string, error) {
	proxyType = strings.ToLower(strings.TrimSpace(proxyType))
	if proxyType != "socks5" {
		return address, nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	if net.ParseIP(host) != nil {
		return address, nil
	}
	if lookup == nil {
		lookup = defaultHostLookup
	}
	addrs, err := lookup(ctx, "ip", host)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("no ip address resolved for %s", host)
	}
	return net.JoinHostPort(addrs[0], port), nil
}

func proxyCacheKey(proxyCfg config.Proxy) string {
	proxyCfg = config.NormalizeProxy(proxyCfg)
	return strings.Join([]string{
		proxyCfg.ID,
		proxyCfg.Type,
		strings.ToLower(proxyCfg.Host),
		strconv.Itoa(proxyCfg.Port),
		proxyCfg.Username,
		proxyCfg.Password,
	}, "|")
}

func proxyDialContext(proxyCfg config.Proxy) (trans.DialContextFunc, error) {
	proxyCfg = config.NormalizeProxy(proxyCfg)
	var authCfg *proxy.Auth
	if proxyCfg.Username != "" || proxyCfg.Password != "" {
		authCfg = &proxy.Auth{User: proxyCfg.Username, Password: proxyCfg.Password}
	}
	forward := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort(proxyCfg.Host, strconv.Itoa(proxyCfg.Port)), authCfg, forward)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		target, err := proxyDialAddress(ctx, proxyCfg.Type, address, defaultHostLookup)
		if err != nil {
			return nil, err
		}
		if ctxDialer, ok := dialer.(proxy.ContextDialer); ok {
			return ctxDialer.DialContext(ctx, network, target)
		}
		return dialer.Dial(network, target)
	}, nil
}

func (c *Client) defaultRequestClients() requestClients {
	return requestClients{
		regular:  c.regular,
		stream:   c.stream,
		fallback: c.fallback,
	}
}

func (c *Client) resolveProxyForAccount(acc config.Account) (config.Proxy, bool) {
	if c == nil || c.Store == nil {
		return config.Proxy{}, false
	}
	proxyID := strings.TrimSpace(acc.ProxyID)
	if proxyID == "" {
		return config.Proxy{}, false
	}
	return c.Store.FindProxy(proxyID)
}

func (c *Client) requestClientsFromContext(ctx context.Context) requestClients {
	if a, ok := auth.FromContext(ctx); ok {
		return c.requestClientsForAccountWithAvoid(a.Account, avoidedProxyIDsFromContext(ctx))
	}
	return c.defaultRequestClients()
}

func (c *Client) requestClientsForAuth(ctx context.Context, a *auth.RequestAuth) requestClients {
	if a != nil {
		clients := c.requestClientsForAccountWithAvoid(a.Account, avoidedProxyIDsFromContext(ctx))
		if clients.workerHost != "" {
			ctx = withWorkerHost(ctx, clients.workerHost)
		}
		return clients
	}
	return c.requestClientsFromContext(ctx)
}

// requestClientsForAuthWithContext is like requestClientsForAuth but returns
// the context enriched with worker host for URL rewriting.
func (c *Client) requestClientsForAuthWithContext(ctx context.Context, a *auth.RequestAuth) (requestClients, context.Context) {
	if a != nil {
		clients := c.requestClientsForAccountWithAvoid(a.Account, avoidedProxyIDsFromContext(ctx))
		if clients.workerHost != "" {
			ctx = withWorkerHost(ctx, clients.workerHost)
		}
		return clients, ctx
	}
	return c.requestClientsFromContext(ctx), ctx
}

func (c *Client) requestClientsForAccount(acc config.Account) requestClients {
	return c.requestClientsForAccountWithAvoid(acc, nil)
}

func (c *Client) requestClientsForAccountWithAvoid(acc config.Account, avoid map[string]bool) requestClients {
	proxyCfg, ok := c.resolveProxyForAccount(acc)
	if !ok {
		return c.defaultRequestClients()
	}

	originalID := proxyCfg.ID
	if c.proxyOnCooldown(originalID) || (avoid != nil && avoid[originalID]) {
		if alt, found := c.pickHealthyProxy(originalID, avoid); found {
			reason := "cooldown"
			if avoid != nil && avoid[originalID] {
				reason = "request_retry"
			}
			config.Logger.Info("[proxy_sticky] switched", "from", originalID, "to", alt.ID, "reason", reason)
			proxyCfg = alt
		}
	}

	// Cloudflare Worker proxy: connect directly, override target host via context
	if proxyCfg.Type == "cloudflare" && proxyCfg.WorkerHost != "" {
		key := proxyCacheKey(proxyCfg)
		c.proxyClientsMu.RLock()
		cached, ok := c.proxyClients[key]
		c.proxyClientsMu.RUnlock()
		if ok {
			return cached
		}
		bundle := requestClients{
			regular:    trans.New(60 * time.Second),
			stream:     trans.New(0),
			fallback:   trans.NewFallbackClient(0, nil),
			proxyID:    proxyCfg.ID,
			workerHost: proxyCfg.WorkerHost,
		}
		c.proxyClientsMu.Lock()
		if c.proxyClients == nil {
			c.proxyClients = make(map[string]requestClients)
		}
		c.proxyClients[key] = bundle
		c.proxyClientsMu.Unlock()
		return bundle
	}

	key := proxyCacheKey(proxyCfg)
	c.proxyClientsMu.RLock()
	cached, ok := c.proxyClients[key]
	c.proxyClientsMu.RUnlock()
	if ok {
		return cached
	}

	dialContext, err := proxyDialContext(proxyCfg)
	if err != nil {
		config.Logger.Warn("[proxy] build dialer failed", "proxy_id", proxyCfg.ID, "error", err)
		return c.defaultRequestClients()
	}

	bundle := requestClients{
		regular:   trans.NewWithDialContext(60*time.Second, dialContext),
		stream:    trans.NewWithDialContext(0, dialContext),
		fallback:  trans.NewFallbackClient(0, dialContext),
		proxyID:   proxyCfg.ID,
	}

	c.proxyClientsMu.Lock()
	if c.proxyClients == nil {
		c.proxyClients = make(map[string]requestClients)
	}
	c.proxyClients[key] = bundle
	c.proxyClientsMu.Unlock()
	return bundle
}

func applyProxyConnectivityHeaders(req *http.Request) {
	if req == nil {
		return
	}
	for key, value := range dsprotocol.BaseHeaders {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		req.Header.Set(key, value)
	}
}

func proxyConnectivityStatus(statusCode int) (bool, string) {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return true, fmt.Sprintf("代理可达，目标返回 HTTP %d", statusCode)
	case statusCode >= 300 && statusCode < 500:
		return true, fmt.Sprintf("代理可达，但目标返回 HTTP %d（可能是风控或挑战）", statusCode)
	default:
		return false, fmt.Sprintf("目标返回 HTTP %d", statusCode)
	}
}

func TestProxyConnectivity(ctx context.Context, proxyCfg config.Proxy) map[string]any {
	start := time.Now()
	proxyCfg = config.NormalizeProxy(proxyCfg)
	result := map[string]any{
		"success":       false,
		"proxy_id":      proxyCfg.ID,
		"proxy_type":    proxyCfg.Type,
		"response_time": 0,
	}

	if err := config.ValidateProxyConfig([]config.Proxy{proxyCfg}); err != nil {
		result["message"] = "代理配置无效: " + err.Error()
		return result
	}
	dialContext, err := proxyDialContext(proxyCfg)
	if err != nil {
		result["message"] = "代理拨号器初始化失败: " + err.Error()
		return result
	}

	client := trans.NewFallbackClient(15*time.Second, dialContext)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyConnectivityTestURL, nil)
	if err != nil {
		result["message"] = err.Error()
		return result
	}
	applyProxyConnectivityHeaders(req)

	resp, err := client.Do(req)
	result["response_time"] = int(time.Since(start).Milliseconds())
	if err != nil {
		result["message"] = err.Error()
		return result
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			config.Logger.Warn("[proxy] close response body failed", "proxy_id", proxyCfg.ID, "error", closeErr)
		}
	}()

	result["status_code"] = resp.StatusCode
	result["success"], result["message"] = proxyConnectivityStatus(resp.StatusCode)
	return result
}
