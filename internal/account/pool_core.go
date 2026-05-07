package account

import (
	"sort"
	"sync"
	"time"

	"ds2api/internal/config"
)

type Pool struct {
	store                  *config.Store
	mu                     sync.Mutex
	queue                  []string
	inUse                  map[string]int
	waiters                []chan struct{}
	cooldowns              map[string]*accountCooldown
	maxInflightPerAccount  int
	recommendedConcurrency int
	maxQueueSize           int
	globalMaxInflight      int
	stats                  *runtimeStats
}

type StatusOptions struct {
	IncludeAccounts bool
}

var (
	defaultRuntimeStatsMu       sync.Mutex
	defaultRuntimeStatsInstance *runtimeStats
	defaultRuntimeStatsPath     string
)

func sharedRuntimeStats() *runtimeStats {
	path := config.RuntimeStatsPath()
	defaultRuntimeStatsMu.Lock()
	defer defaultRuntimeStatsMu.Unlock()
	if defaultRuntimeStatsInstance == nil || defaultRuntimeStatsPath != path {
		defaultRuntimeStatsInstance = newRuntimeStats(path)
		defaultRuntimeStatsPath = path
	}
	return defaultRuntimeStatsInstance
}

func RecordDirectRequest() {
	sharedRuntimeStats().record(time.Now())
}

func NewPool(store *config.Store) *Pool {
	maxPer := 2
	if store != nil {
		maxPer = store.RuntimeAccountMaxInflight()
	}
	p := &Pool{
		store:                 store,
		inUse:                 map[string]int{},
		cooldowns:             map[string]*accountCooldown{},
		maxInflightPerAccount: maxPer,
		stats:                 sharedRuntimeStats(),
	}
	if store == nil {
		p.stats = newInMemoryRuntimeStats()
	}
	p.Reset()
	return p
}

func (p *Pool) Reset() {
	var accounts []config.Account
	if p.store != nil {
		accounts = p.store.Accounts()
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		iHas := accounts[i].Token != ""
		jHas := accounts[j].Token != ""
		if iHas == jHas {
			return i < j
		}
		return iHas
	})
	ids := make([]string, 0, len(accounts))
	for _, a := range accounts {
		id := a.Identifier()
		if id != "" {
			ids = append(ids, id)
		}
	}
	if p.store != nil {
		p.maxInflightPerAccount = p.store.RuntimeAccountMaxInflight()
	} else {
		p.maxInflightPerAccount = maxInflightFromEnv()
	}
	recommended := defaultRecommendedConcurrency(len(ids), p.maxInflightPerAccount)
	queueLimit := maxQueueFromEnv(recommended)
	globalLimit := recommended
	if p.store != nil {
		queueLimit = p.store.RuntimeAccountMaxQueue(recommended)
		globalLimit = p.store.RuntimeGlobalMaxInflight(recommended)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.drainWaitersLocked()
	p.queue = ids
	p.inUse = map[string]int{}
	if p.cooldowns == nil {
		p.cooldowns = map[string]*accountCooldown{}
	}
	p.recommendedConcurrency = recommended
	p.maxQueueSize = queueLimit
	p.globalMaxInflight = globalLimit
	config.Logger.Info(
		"[init_account_queue] initialized",
		"total", len(ids),
		"max_inflight_per_account", p.maxInflightPerAccount,
		"global_max_inflight", p.globalMaxInflight,
		"recommended_concurrency", p.recommendedConcurrency,
		"max_queue_size", p.maxQueueSize,
	)
}

func (p *Pool) Release(accountID string) {
	if accountID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	count := p.inUse[accountID]
	if count <= 0 {
		return
	}
	if count == 1 {
		delete(p.inUse, accountID)
		p.notifyWaiterLocked()
		return
	}
	p.inUse[accountID] = count - 1
	p.notifyWaiterLocked()
}

func (p *Pool) Status() map[string]any {
	return p.StatusWithOptions(StatusOptions{IncludeAccounts: true})
}

func (p *Pool) StatusSummary() map[string]any {
	return p.StatusWithOptions(StatusOptions{})
}

func (p *Pool) StatusWithOptions(opts StatusOptions) map[string]any {
	p.mu.Lock()
	stats := p.stats
	if stats == nil {
		stats = sharedRuntimeStats()
	}
	inUseAccounts := make([]string, 0, len(p.inUse))
	availableAccounts := make([]string, 0)
	availableSlots := 0
	inUseSlots := 0
	coolingDown := 0
	for _, id := range p.queue {
		if p.accountOnCooldownLocked(id, time.Now()) {
			coolingDown++
			continue
		}
		if p.inUse[id] >= p.maxInflightPerAccount {
			continue
		}
		availableSlots++
		if opts.IncludeAccounts {
			availableAccounts = append(availableAccounts, id)
		}
	}
	for id, count := range p.inUse {
		if count > 0 {
			if opts.IncludeAccounts {
				inUseAccounts = append(inUseAccounts, id)
			}
			inUseSlots += count
		}
	}
	if opts.IncludeAccounts {
		sort.Strings(inUseAccounts)
	}
	totalAccounts := len(p.queue)
	store := p.store
	status := map[string]any{
		"available":                availableSlots,
		"in_use":                   inUseSlots,
		"max_inflight_per_account": p.maxInflightPerAccount,
		"global_max_inflight":      p.globalMaxInflight,
		"recommended_concurrency":  p.recommendedConcurrency,
		"waiting":                  len(p.waiters),
		"cooling_down":             coolingDown,
		"max_queue_size":           p.maxQueueSize,
	}
	if opts.IncludeAccounts {
		status["available_accounts"] = availableAccounts
		status["in_use_accounts"] = inUseAccounts
	}
	p.mu.Unlock()
	if store != nil {
		totalAccounts = store.AccountCount()
	}
	totalRequests, rpm := stats.snapshot(time.Now())
	status["total"] = totalAccounts
	status["rpm"] = rpm
	status["total_requests"] = totalRequests
	return status
}

func (p *Pool) RecordRequest() {
	if p == nil {
		return
	}
	stats := p.stats
	if stats == nil {
		stats = sharedRuntimeStats()
	}
	stats.record(time.Now())
}
