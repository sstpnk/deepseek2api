// Package auth is a thin forwarder to internal/deepseek/client so the
// existing import paths in the slim codebase keep working.
package auth

import (
	"context"
	"time"

	"ds2api/internal/deepseek/client"
)

type (
	RequestAuth = client.RequestAuth
	Resolver    = client.Resolver
	Pool        = client.Pool
)

var (
	NewResolver       = client.NewResolver
	ErrNoAccount      = client.ErrNoAccount
	ErrNotImplemented = client.ErrNotImplemented
)

type contextKey struct{}

var authKey contextKey

// WithAuth attaches a RequestAuth to the context.
func WithAuth(ctx context.Context, a *RequestAuth) context.Context {
	return context.WithValue(ctx, authKey, a)
}

// FromAuth extracts a RequestAuth from the context, if any.
func FromAuth(ctx context.Context) *RequestAuth {
	if a, ok := ctx.Value(authKey).(*RequestAuth); ok {
		return a
	}
	return nil
}

// CooldownAccount forwards to the resolver.
func CooldownAccount(r *Resolver, a *RequestAuth, ttl time.Duration, reason string) {
	r.CooldownAccount(a, ttl, reason)
}
