package client

import (
	"context"
	"math/rand/v2"
	"time"
)

const (
	backoffBaseUpstream  = 200 * time.Millisecond
	backoffBaseTransport = 600 * time.Millisecond
	backoffCap           = 3 * time.Second
)

// sleepWithCtx sleeps for d, but returns early with ctx.Err() if the context
// is cancelled. d <= 0 means no sleep, just check context state.
func sleepWithCtx(ctx context.Context, d time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// retryDelay returns an exponential backoff with ±25% jitter, capped.
// attempt is 0-indexed (0 = before first retry sleep).
// transport=true uses a longer base because transport errors usually need
// more recovery time than upstream business errors.
func retryDelay(attempt int, transport bool) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	base := backoffBaseUpstream
	if transport {
		base = backoffBaseTransport
	}
	shift := attempt
	if shift > 4 {
		shift = 4
	}
	d := base << shift
	if d > backoffCap {
		d = backoffCap
	}
	j := d / 4
	if j <= 0 {
		return d
	}
	off := rand.Int64N(int64(2*j) + 1)
	return d - j + time.Duration(off)
}
