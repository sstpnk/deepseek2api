package client

import (
	"hash/fnv"
	"strings"
	"time"

	"ds2api/internal/config"
)

const (
	proxyFailureThreshold     = 3
	proxyCooldownInitial      = 30 * time.Second
	proxyCooldownMax          = time.Hour
	proxyCooldownGrowthFactor = 2
	proxyRecoverySuccesses    = 3
)

type proxyHealth struct {
	failures          int
	recoverySuccesses int
	cooldownLevel     int
	cooldownUntil     time.Time
	lastFailure       time.Time
	lastSuccess       time.Time
}

// markProxyFailure records a transport-level failure for proxyID.
// Once failures reach the threshold, the proxy enters a cooldown window
// (exponentially increasing on repeated failures, capped).
func (c *Client) markProxyFailure(proxyID string) {
	proxyID = strings.TrimSpace(proxyID)
	if c == nil || proxyID == "" {
		return
	}
	c.proxyHealthMu.Lock()
	defer c.proxyHealthMu.Unlock()
	if c.proxyHealthMap == nil {
		c.proxyHealthMap = map[string]*proxyHealth{}
	}
	h, ok := c.proxyHealthMap[proxyID]
	if !ok {
		h = &proxyHealth{}
		c.proxyHealthMap[proxyID] = h
	}
	h.failures++
	h.recoverySuccesses = 0
	h.lastFailure = time.Now()
	if h.failures < proxyFailureThreshold {
		return
	}
	cooldown := proxyCooldownInitial
	for i := 0; i < h.cooldownLevel && cooldown < proxyCooldownMax; i++ {
		cooldown *= proxyCooldownGrowthFactor
	}
	if cooldown > proxyCooldownMax {
		cooldown = proxyCooldownMax
	}
	h.cooldownUntil = h.lastFailure.Add(cooldown)
	if h.cooldownLevel < 16 {
		h.cooldownLevel++
	}
	h.failures = 0
	config.Logger.Info("[proxy_health] marked unhealthy",
		"proxy_id", proxyID,
		"cooldown_seconds", int(cooldown/time.Second),
		"cooldown_level", h.cooldownLevel,
	)
}

// markProxySuccess records a healthy request for proxyID. A single success
// clears the active cooldown so recovered proxies can be used, but the
// cooldown level only decays after multiple consecutive successes. This keeps
// repeatedly flaky Resin leases from re-entering rotation at full weight every
// few minutes.
func (c *Client) markProxySuccess(proxyID string) {
	proxyID = strings.TrimSpace(proxyID)
	if c == nil || proxyID == "" {
		return
	}
	c.proxyHealthMu.Lock()
	defer c.proxyHealthMu.Unlock()
	h, ok := c.proxyHealthMap[proxyID]
	if !ok {
		return
	}
	h.failures = 0
	h.lastSuccess = time.Now()
	h.cooldownUntil = time.Time{}
	if h.cooldownLevel > 0 {
		h.recoverySuccesses++
		if h.recoverySuccesses >= proxyRecoverySuccesses {
			h.cooldownLevel--
			h.recoverySuccesses = 0
		}
	} else {
		h.recoverySuccesses = 0
	}
}

// proxyOnCooldown reports whether proxyID is currently in a cooldown window.
func (c *Client) proxyOnCooldown(proxyID string) bool {
	proxyID = strings.TrimSpace(proxyID)
	if c == nil || proxyID == "" {
		return false
	}
	c.proxyHealthMu.Lock()
	defer c.proxyHealthMu.Unlock()
	h, ok := c.proxyHealthMap[proxyID]
	if !ok {
		return false
	}
	return time.Now().Before(h.cooldownUntil)
}

// pickHealthyProxy returns a proxy from the store that is not on cooldown
// and not equal to skipID. Selection is deterministic per (skipID, time
// bucket) to spread load across replacements rather than herd onto one.
// Returns false if no healthy alternative exists.
func (c *Client) pickHealthyProxy(skipID string, avoid map[string]bool) (config.Proxy, bool) {
	if c == nil || c.Store == nil {
		return config.Proxy{}, false
	}
	skipID = strings.TrimSpace(skipID)
	proxies := c.Store.Proxies()
	if len(proxies) == 0 {
		return config.Proxy{}, false
	}
	candidates := make([]config.Proxy, 0, len(proxies))
	bestScore := -1
	for _, p := range proxies {
		p = config.NormalizeProxy(p)
		if p.ID == "" || p.ID == skipID {
			continue
		}
		if avoid != nil && avoid[p.ID] {
			continue
		}
		if c.proxyOnCooldown(p.ID) {
			continue
		}
		score := c.proxyHealthScore(p.ID)
		if score > bestScore {
			bestScore = score
			candidates = candidates[:0]
		}
		if score == bestScore {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return config.Proxy{}, false
	}
	bucket := time.Now().Unix() / 60
	h := fnv.New32a()
	_, _ = h.Write([]byte(skipID))
	var bb [8]byte
	for i := 0; i < 8; i++ {
		bb[i] = byte(bucket >> (i * 8))
	}
	_, _ = h.Write(bb[:])
	idx := int(h.Sum32()) % len(candidates)
	if idx < 0 {
		idx = -idx
	}
	return candidates[idx], true
}

func (c *Client) proxyHealthScore(proxyID string) int {
	proxyID = strings.TrimSpace(proxyID)
	if c == nil || proxyID == "" {
		return 0
	}
	c.proxyHealthMu.Lock()
	defer c.proxyHealthMu.Unlock()
	h, ok := c.proxyHealthMap[proxyID]
	if !ok {
		return 1000
	}
	score := 1000 - h.cooldownLevel*50 - h.failures*10
	if h.recoverySuccesses > 0 {
		score += h.recoverySuccesses * 5
	}
	if score < 0 {
		return 0
	}
	return score
}

// proxyHealthGC removes proxy health entries for proxies no longer in the store.
// Called periodically (every 5 minutes) to prevent memory leak from deleted proxies.
func (c *Client) proxyHealthGC() {
	if c == nil || c.Store == nil {
		return
	}
	active := map[string]bool{}
	for _, p := range c.Store.Proxies() {
		active[p.ID] = true
	}
	c.proxyHealthMu.Lock()
	for id := range c.proxyHealthMap {
		if !active[id] {
			delete(c.proxyHealthMap, id)
		}
	}
	c.proxyHealthMu.Unlock()
}

// accountEmptyOutputGC removes expired empty-output health entries that haven't
// been updated within 2x the failure window.
func (c *Client) accountEmptyOutputGC() {
	if c == nil {
		return
	}
	c.accountHealthMu.Lock()
	for id, h := range c.accountHealthMap {
		if time.Since(h.lastFailure) > 2*accountEmptyOutputFailureWindow {
			delete(c.accountHealthMap, id)
		}
	}
	c.accountHealthMu.Unlock()
}

// StartBackgroundGC launches periodic cleanup goroutines for proxy health,
// empty-output accounts, and proxy client caches. Call once at startup.
func (c *Client) StartBackgroundGC() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			c.proxyHealthGC()
			c.accountEmptyOutputGC()
			c.proxyClientsMu.Lock()
			// Clear client cache so stale proxy configs are rebuilt
			c.proxyClients = make(map[string]requestClients)
			c.proxyClientsMu.Unlock()
		}
	}()
}
