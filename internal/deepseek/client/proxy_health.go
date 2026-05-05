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
	proxyCooldownMax          = 5 * time.Minute
	proxyCooldownGrowthFactor = 2
)

type proxyHealth struct {
	failures      int
	cooldownLevel int
	cooldownUntil time.Time
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
	h.cooldownUntil = time.Now().Add(cooldown)
	if h.cooldownLevel < 8 {
		h.cooldownLevel++
	}
	h.failures = 0
	config.Logger.Info("[proxy_health] marked unhealthy",
		"proxy_id", proxyID,
		"cooldown_seconds", int(cooldown/time.Second),
		"cooldown_level", h.cooldownLevel,
	)
}

// markProxySuccess clears failure counters and cooldown for proxyID.
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
	if h.failures == 0 && h.cooldownLevel == 0 && h.cooldownUntil.IsZero() {
		return
	}
	h.failures = 0
	h.cooldownLevel = 0
	h.cooldownUntil = time.Time{}
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
func (c *Client) pickHealthyProxy(skipID string) (config.Proxy, bool) {
	if c == nil || c.Store == nil {
		return config.Proxy{}, false
	}
	skipID = strings.TrimSpace(skipID)
	snap := c.Store.Snapshot()
	if len(snap.Proxies) == 0 {
		return config.Proxy{}, false
	}
	candidates := make([]config.Proxy, 0, len(snap.Proxies))
	for _, p := range snap.Proxies {
		p = config.NormalizeProxy(p)
		if p.ID == "" || p.ID == skipID {
			continue
		}
		if c.proxyOnCooldown(p.ID) {
			continue
		}
		candidates = append(candidates, p)
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
