package shared

import "ds2api/internal/config"

// StoreConfigReader adapts *config.Store to ConfigReader via Snapshot().
type StoreConfigReader struct{ Store *config.Store }

func (r StoreConfigReader) ModelAliases() map[string]string {
	return r.Store.Snapshot().ModelAliases
}
func (r StoreConfigReader) ThinkingInjectionEnabled() bool {
	return boolPtr(r.Store.Snapshot().ThinkingInjection.Enabled)
}
func (r StoreConfigReader) ThinkingInjectionMinChars() int { return 0 }
func (r StoreConfigReader) ThinkingInjectionPrompt() string {
	return r.Store.Snapshot().ThinkingInjection.Prompt
}
func (r StoreConfigReader) CurrentInputFileEnabled() bool {
	return boolPtr(r.Store.Snapshot().CurrentInputFile.Enabled)
}
func (r StoreConfigReader) CurrentInputFileMinChars() int {
	return r.Store.Snapshot().CurrentInputFile.MinChars
}
func (r StoreConfigReader) CurrentInputFilePrompt() string { return "" }

func boolPtr(p *bool) bool { return p != nil && *p }
