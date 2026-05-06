package configmgmt

import (
	"path/filepath"
	"testing"

	"ds2api/internal/account"
	"ds2api/internal/config"
)

func newAdminTestHandler(t *testing.T, raw string) *Handler {
	t.Helper()
	t.Setenv("DS2API_RUNTIME_STATS_PATH", filepath.Join(t.TempDir(), "runtime_stats.json"))
	t.Setenv("DS2API_CONFIG_JSON", raw)
	store := config.LoadStore()
	return &Handler{
		Store: store,
		Pool:  account.NewPool(store),
	}
}
