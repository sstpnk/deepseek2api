// Package account is a slim placeholder. The real account pool lives in the
// upstream codebase and is reintroduced once the slim build is wired up.
package account

// Pool is intentionally empty. The slim build uses config.Store.Pool() -> nil
// and the deepseek client resolver handles nil pools by falling back to the
// config-level token.
type Pool struct{}
