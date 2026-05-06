package config

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

func (s *Store) ModelAliases() map[string]string {
	if s == nil {
		return DefaultModelAliases()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := DefaultModelAliases()
	for k, v := range s.cfg.ModelAliases {
		key := strings.TrimSpace(lower(k))
		val := strings.TrimSpace(lower(v))
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	return out
}

func (s *Store) ConfiguredModelAliases() map[string]string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneStringMap(s.cfg.ModelAliases)
}

func (s *Store) VercelConfig() VercelConfig {
	if s == nil {
		return VercelConfig{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return NormalizeVercelConfig(s.cfg.Vercel)
}

func (s *Store) ToolcallMode() string {
	return "feature_match"
}

func (s *Store) ToolcallEarlyEmitConfidence() string {
	return "high"
}

func (s *Store) ResponsesStoreTTLSeconds() int {
	if s == nil {
		return 900
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Responses.StoreTTLSeconds > 0 {
		return s.cfg.Responses.StoreTTLSeconds
	}
	return 900
}

func (s *Store) ResponsesConfig() ResponsesConfig {
	if s == nil {
		return ResponsesConfig{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Responses
}

func (s *Store) EmbeddingsProvider() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.Embeddings.Provider)
}

func (s *Store) EmbeddingsConfig() EmbeddingsConfig {
	if s == nil {
		return EmbeddingsConfig{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Embeddings
}

func (s *Store) AutoDeleteConfig() AutoDeleteConfig {
	if s == nil {
		return AutoDeleteConfig{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.AutoDelete
}

func (s *Store) AutoDeleteMode() string {
	if s == nil {
		return "none"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	mode := strings.ToLower(strings.TrimSpace(s.cfg.AutoDelete.Mode))
	switch mode {
	case "none", "single", "all":
		return mode
	}
	if s.cfg.AutoDelete.Sessions {
		return "all"
	}
	return "none"
}

func (s *Store) AdminPasswordHash() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.Admin.PasswordHash)
}

func (s *Store) AdminJWTExpireHours() int {
	if s == nil {
		return 24
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Admin.JWTExpireHours > 0 {
		return s.cfg.Admin.JWTExpireHours
	}
	if raw := strings.TrimSpace(os.Getenv("DS2API_JWT_EXPIRE_HOURS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return 24
}

func (s *Store) AdminJWTValidAfterUnix() int64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Admin.JWTValidAfterUnix
}

func (s *Store) RuntimeAccountMaxInflight() int {
	if s == nil {
		return 2
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Runtime.AccountMaxInflight > 0 {
		return s.cfg.Runtime.AccountMaxInflight
	}
	if raw := strings.TrimSpace(os.Getenv("DS2API_ACCOUNT_MAX_INFLIGHT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return 2
}

func (s *Store) RuntimeAccountMaxQueue(defaultSize int) int {
	if s == nil {
		if defaultSize < 0 {
			return 0
		}
		return defaultSize
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Runtime.AccountMaxQueue > 0 {
		return s.cfg.Runtime.AccountMaxQueue
	}
	if raw := strings.TrimSpace(os.Getenv("DS2API_ACCOUNT_MAX_QUEUE")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			return n
		}
	}
	if defaultSize < 0 {
		return 0
	}
	return defaultSize
}

func (s *Store) RuntimeGlobalMaxInflight(defaultSize int) int {
	if s == nil {
		if defaultSize < 0 {
			return 0
		}
		return defaultSize
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Runtime.GlobalMaxInflight > 0 {
		return s.cfg.Runtime.GlobalMaxInflight
	}
	if raw := strings.TrimSpace(os.Getenv("DS2API_GLOBAL_MAX_INFLIGHT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	if defaultSize < 0 {
		return 0
	}
	return defaultSize
}

func (s *Store) RuntimePowMaxConcurrency() int {
	if s == nil {
		return DefaultPowMaxConcurrency()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Runtime.PowMaxConcurrency > 0 {
		return s.cfg.Runtime.PowMaxConcurrency
	}
	if raw := strings.TrimSpace(os.Getenv("DS2API_POW_MAX_CONCURRENCY")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return DefaultPowMaxConcurrency()
}

func DefaultPowMaxConcurrency() int {
	n := runtime.GOMAXPROCS(0) / 2
	if n < 1 {
		return 1
	}
	if n > 4 {
		return 4
	}
	return n
}

func (s *Store) RuntimeTokenRefreshIntervalHours() int {
	if s == nil {
		return 6
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Runtime.TokenRefreshIntervalHours > 0 {
		return s.cfg.Runtime.TokenRefreshIntervalHours
	}
	return 6
}

func (s *Store) RuntimeConfig() RuntimeConfig {
	if s == nil {
		return RuntimeConfig{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Runtime
}

func (s *Store) RuntimeAccountCount() int {
	if s == nil {
		return 0
	}
	return s.AccountCount()
}

func (s *Store) AutoDeleteSessions() bool {
	return s.AutoDeleteMode() != "none"
}

func (s *Store) CurrentInputFileEnabled() bool {
	if s == nil {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.CurrentInputFile.Enabled == nil {
		return true
	}
	return *s.cfg.CurrentInputFile.Enabled
}

func (s *Store) CurrentInputFileMinChars() int {
	if s == nil {
		return DefaultCurrentInputFileMinChars
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.cfg.CurrentInputFile.MinCharsSet && s.cfg.CurrentInputFile.MinChars == 0 {
		return DefaultCurrentInputFileMinChars
	}
	return s.cfg.CurrentInputFile.MinChars
}

func (s *Store) ThinkingInjectionEnabled() bool {
	if s == nil {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.ThinkingInjection.Enabled == nil {
		return true
	}
	return *s.cfg.ThinkingInjection.Enabled
}

func (s *Store) ThinkingInjectionPrompt() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.cfg.ThinkingInjection.Prompt)
}
