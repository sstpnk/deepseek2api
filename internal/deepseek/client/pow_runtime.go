package client

import (
	"context"
	"strings"
	"sync"
	"time"

	"ds2api/internal/config"
	"ds2api/pow"
)

const powCacheTTL = 45 * time.Second

type powConfigStore interface {
	RuntimePowMaxConcurrency() int
}

type powRuntime struct {
	store powConfigStore

	mu      sync.Mutex
	sem     chan struct{}
	semSize int
	cache   map[string]powCacheEntry
	active  map[string]*powFlight
}

type powCacheEntry struct {
	header    string
	expiresAt time.Time
}

type powFlight struct {
	done   chan struct{}
	header string
	err    error
}

func newPowRuntime(store powConfigStore) *powRuntime {
	return &powRuntime{
		store:  store,
		cache:  map[string]powCacheEntry{},
		active: map[string]*powFlight{},
	}
}

func (r *powRuntime) compute(ctx context.Context, key string, challenge map[string]any) (string, error) {
	if r == nil {
		return solvePowHeaderDirect(ctx, challenge)
	}
	key = strings.TrimSpace(key)
	if header, ok := r.getCached(key); ok {
		return header, nil
	}
	if flight, owner := r.startFlight(key); !owner {
		return r.waitFlight(ctx, flight)
	} else {
		defer r.finishFlight(key, flight)
	}
	release, err := r.acquire(ctx)
	if err != nil {
		r.recordFlight(key, "", err)
		return "", err
	}
	defer release()
	if header, ok := r.getCached(key); ok {
		r.recordFlight(key, header, nil)
		return header, nil
	}
	answer, err := ComputePow(ctx, challenge)
	if err != nil {
		r.recordFlight(key, "", err)
		return "", err
	}
	header, err := BuildPowHeader(challenge, answer)
	if err != nil {
		r.recordFlight(key, "", err)
		return "", err
	}
	r.recordFlight(key, header, nil)
	r.setCached(key, header, time.Now().Add(powCacheTTL))
	return header, nil
}

func (r *powRuntime) computeUncached(ctx context.Context, challenge map[string]any) (string, error) {
	if r == nil {
		return solvePowHeaderDirect(ctx, challenge)
	}
	release, err := r.acquire(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	return solvePowHeaderDirect(ctx, challenge)
}

func solvePowHeaderDirect(ctx context.Context, challenge map[string]any) (string, error) {
	answer, err := ComputePow(ctx, challenge)
	if err != nil {
		return "", err
	}
	return BuildPowHeader(challenge, answer)
}

func (r *powRuntime) invalidatePrefix(prefix string) {
	if r == nil {
		return
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return
	}
	r.mu.Lock()
	for key := range r.cache {
		if key == prefix || strings.HasPrefix(key, prefix+"|") {
			delete(r.cache, key)
		}
	}
	r.mu.Unlock()
}

func (r *powRuntime) startFlight(key string) (*powFlight, bool) {
	if key == "" {
		return &powFlight{done: closedDone()}, true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active == nil {
		r.active = map[string]*powFlight{}
	}
	if flight, ok := r.active[key]; ok {
		return flight, false
	}
	flight := &powFlight{done: make(chan struct{})}
	r.active[key] = flight
	return flight, true
}

func (r *powRuntime) recordFlight(key, header string, err error) {
	if key == "" {
		return
	}
	r.mu.Lock()
	if flight := r.active[key]; flight != nil {
		flight.header = header
		flight.err = err
	}
	r.mu.Unlock()
}

func (r *powRuntime) finishFlight(key string, flight *powFlight) {
	if key == "" || flight == nil {
		return
	}
	r.mu.Lock()
	if r.active[key] == flight {
		delete(r.active, key)
	}
	close(flight.done)
	r.mu.Unlock()
}

func (r *powRuntime) waitFlight(ctx context.Context, flight *powFlight) (string, error) {
	if flight == nil {
		return "", nil
	}
	select {
	case <-flight.done:
		return flight.header, flight.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func closedDone() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (r *powRuntime) getCached(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.cache[key]
	if !ok {
		return "", false
	}
	if now.After(entry.expiresAt) {
		delete(r.cache, key)
		return "", false
	}
	return entry.header, true
}

func (r *powRuntime) setCached(key, header string, expiresAt time.Time) {
	if key == "" || strings.TrimSpace(header) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[key] = powCacheEntry{header: header, expiresAt: expiresAt}
	for k, entry := range r.cache {
		if time.Now().After(entry.expiresAt) {
			delete(r.cache, k)
		}
	}
}

func (r *powRuntime) acquire(ctx context.Context) (func(), error) {
	sem := r.semaphore()
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *powRuntime) semaphore() chan struct{} {
	size := r.maxConcurrency()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sem == nil || r.semSize != size {
		r.sem = make(chan struct{}, size)
		r.semSize = size
	}
	return r.sem
}

func (r *powRuntime) maxConcurrency() int {
	// When SolvePow uses internal parallelism, cap external semaphore at 1
	// to prevent M concurrent requests × N workers from oversubscribing CPU.
	if pow.PowInternalParallel() >= 2 {
		return 1
	}
	if r == nil || r.store == nil {
		return config.DefaultPowMaxConcurrency()
	}
	n := r.store.RuntimePowMaxConcurrency()
	if n <= 0 {
		return config.DefaultPowMaxConcurrency()
	}
	return n
}
