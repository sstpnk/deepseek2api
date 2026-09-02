package account

import (
	"strings"
	"time"

	"ds2api/internal/config"
)

const (
	accountCooldownInitial      = 5 * time.Minute
	accountCooldownMax          = 6 * time.Hour
	accountCooldownGrowthFactor = 3
	accountCooldownRecoveryHits = 3
)

type accountCooldown struct {
	level     int
	successes int
	until     time.Time
}

func (p *Pool) Cooldown(accountID, reason string) {
	accountID = strings.TrimSpace(accountID)
	if p == nil || accountID == "" {
		return
	}
	now := time.Now()
	p.mu.Lock()
	if p.cooldowns == nil {
		p.cooldowns = map[string]*accountCooldown{}
	}
	cd := p.cooldowns[accountID]
	if cd == nil {
		cd = &accountCooldown{}
		p.cooldowns[accountID] = cd
	}
	d := accountCooldownInitial
	for i := 0; i < cd.level && d < accountCooldownMax; i++ {
		d *= accountCooldownGrowthFactor
	}
	if d > accountCooldownMax {
		d = accountCooldownMax
	}
	cd.until = now.Add(d)
	if cd.level < 8 {
		cd.level++
	}
	cd.successes = 0
	level := cd.level
	p.notifyWaiterLocked()
	p.mu.Unlock()

	config.Logger.Warn("[account_health] marked cooling_down",
		"account", accountID,
		"cooldown_seconds", int(d/time.Second),
		"cooldown_level", level,
		"reason", reason,
	)
}

func (p *Pool) Success(accountID, reason string) {
	accountID = strings.TrimSpace(accountID)
	if p == nil || accountID == "" {
		return
	}
	p.mu.Lock()
	if p.cooldowns == nil {
		p.mu.Unlock()
		return
	}
	cd := p.cooldowns[accountID]
	if cd == nil || cd.level <= 0 {
		p.mu.Unlock()
		return
	}
	if time.Now().Before(cd.until) {
		p.mu.Unlock()
		return
	}
	cd.successes++
	if cd.successes < accountCooldownRecoveryHits {
		successes := cd.successes
		level := cd.level
		p.mu.Unlock()
		config.Logger.Info("[account_health] recovery success observed",
			"account", accountID,
			"successes", successes,
			"required", accountCooldownRecoveryHits,
			"cooldown_level", level,
			"reason", reason,
		)
		return
	}
	cd.level--
	cd.successes = 0
	level := cd.level
	if cd.level <= 0 {
		delete(p.cooldowns, accountID)
	}
	p.mu.Unlock()

	config.Logger.Info("[account_health] cooldown penalty decayed",
		"account", accountID,
		"cooldown_level", level,
		"reason", reason,
	)
}

func (p *Pool) accountOnCooldownLocked(accountID string, now time.Time) bool {
	if p == nil || strings.TrimSpace(accountID) == "" || p.cooldowns == nil {
		return false
	}
	cd, ok := p.cooldowns[accountID]
	if !ok || cd == nil {
		return false
	}
	if now.Before(cd.until) {
		return true
	}
	return false
}
