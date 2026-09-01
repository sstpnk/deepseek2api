// Package client provides minimal types and stubs for the slim ds2api build.
//
// In the upstream codebase this package holds the real DeepSeek HTTP client
// implementation. The slim build replaces it with placeholder types that let
// the OpenAI-compatible HTTP surface compile and run end-to-end, while the
// actual DeepSeek request pipeline is wired up in a follow-up change.
package client

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"ds2api/internal/config"
)

// ErrNoAccount is returned when no account is available to serve a request.
var ErrNoAccount = errors.New("no account available")

// ErrNotImplemented is returned by stubbed DeepSeek client methods.
var ErrNotImplemented = errors.New("deepseek client: not implemented in slim build")

// RequestAuth is the per-request authentication context passed from the
// resolver into the client and HTTP handlers.
type RequestAuth struct {
	DeepSeekToken  string
	AccountID      string
	Account        any
	UseConfigToken bool
	CallerID       string
}

// Resolver selects an account from the pool for a given request.
type Resolver struct {
	mu   sync.Mutex
	pool Pool
}

// NewResolver returns a resolver that draws accounts from the given pool.
func NewResolver(p Pool) *Resolver {
	return &Resolver{pool: p}
}

// Pool is the minimal subset of the account pool interface used by the
// resolver. The real implementation lives in internal/account.
type Pool interface {
	Acquire(target string, exclude map[string]bool) (any, bool)
	Release(id string)
	Success(id, reason string)
	Cooldown(id, reason string)
}

// Determine picks an account for the request. With no pool wired up it
// returns a stub auth that uses the config token.
func (r *Resolver) Determine(_ context.Context) (*RequestAuth, error) {
	if r == nil {
		return nil, ErrNoAccount
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pool == nil {
		return &RequestAuth{UseConfigToken: true, CallerID: "slim"}, nil
	}
	acc, ok := r.pool.Acquire("", nil)
	if !ok {
		return &RequestAuth{UseConfigToken: true, CallerID: "slim"}, ErrNoAccount
	}
	accountID := ""
	if acc != nil {
		if m, ok := acc.(interface{ Name() string }); ok {
			accountID = m.Name()
		} else if s, ok := acc.(interface{ ID() string }); ok {
			accountID = s.ID()
		}
	}
	return &RequestAuth{
		AccountID:      accountID,
		Account:        acc,
		UseConfigToken: false,
		CallerID:       "slim",
	}, nil
}

// DetermineCaller is a convenience wrapper.
func (r *Resolver) DetermineCaller(ctx context.Context, _ string) (*RequestAuth, error) {
	return r.Determine(ctx)
}

// Release returns an account to the pool.
func (r *Resolver) Release(a *RequestAuth) {
	if r == nil || a == nil || r.pool == nil {
		return
	}
	if a.AccountID != "" {
		r.pool.Release(a.AccountID)
	}
}

// CooldownAccount marks an account as temporarily unavailable.
func (r *Resolver) CooldownAccount(a *RequestAuth, _ time.Duration, reason string) {
	if r == nil || a == nil || r.pool == nil {
		return
	}
	if a.AccountID != "" {
		r.pool.Cooldown(a.AccountID, reason)
	}
}

// RecordAccountSuccess records a successful response for an account.
func (r *Resolver) RecordAccountSuccess(a *RequestAuth, reason string) {
	if r == nil || a == nil || r.pool == nil {
		return
	}
	if a.AccountID != "" {
		r.pool.Success(a.AccountID, reason)
	}
}

// Client is the slim DeepSeek client stub.
type Client struct {
	store  *config.Store
	auth   *Resolver
	mu     sync.Mutex
	closed bool
}

// NewClient returns a new slim client. pool may be nil.
func NewClient(store *config.Store, pool Pool) *Client {
	return &Client{store: store, auth: NewResolver(pool)}
}

// PreloadPow is a no-op in the slim build.
func (c *Client) PreloadPow(_ context.Context) {}

// Close releases client resources.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
}

// Auth returns the account resolver.
func (c *Client) Auth() *Resolver { return c.auth }

// Store returns the config store.
func (c *Client) Store() any { return c.store }

// CallCompletion is the slim stub for the DeepSeek completion call. It
// always returns ErrNotImplemented so callers can fail fast.
func (c *Client) CallCompletion(_ context.Context, _ *RequestAuth, _ map[string]any, _ string, _ int) (*http.Response, error) {
	return nil, ErrNotImplemented
}
