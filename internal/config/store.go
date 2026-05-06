package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
)

type Store struct {
	mu      sync.RWMutex
	cfg     Config
	path    string
	fromEnv bool
	keyMap  map[string]struct{} // O(1) API key lookup index
	accMap  map[string]int      // O(1) account lookup: identifier -> slice index
	accTest map[string]string   // runtime-only account test status cache
}

func LoadStore() *Store {
	store, err := loadStore()
	if err != nil {
		Logger.Warn("[config] load failed", "error", err)
	}
	if len(store.cfg.Keys) == 0 && len(store.cfg.Accounts) == 0 {
		Logger.Warn("[config] empty config loaded")
	}
	store.rebuildIndexes()
	return store
}

func LoadStoreWithError() (*Store, error) {
	store, err := loadStore()
	if err != nil {
		return nil, err
	}
	store.rebuildIndexes()
	return store, nil
}

func loadStore() (*Store, error) {
	cfg, fromEnv, err := loadConfig()
	cfg.NormalizeCredentials()
	if validateErr := ValidateConfig(cfg); validateErr != nil {
		err = errors.Join(err, validateErr)
	}
	return &Store{cfg: cfg, path: ConfigPath(), fromEnv: fromEnv}, err
}

func loadConfig() (Config, bool, error) {
	rawCfg := strings.TrimSpace(os.Getenv("DS2API_CONFIG_JSON"))
	path := ConfigPath()
	if rawCfg != "" {
		cfg, err := parseConfigString(rawCfg)
		if err != nil {
			if !IsVercel() && envWritebackEnabled() {
				if fileCfg, fileErr := loadConfigFromFile(path); fileErr == nil {
					return fileCfg, false, nil
				}
			}
			return cfg, true, err
		}
		cfg.ClearAccountTokens()
		cfg.DropInvalidAccounts()
		if IsVercel() || !envWritebackEnabled() {
			return cfg, true, err
		}
		content, fileErr := os.ReadFile(path)
		if fileErr == nil {
			var fileCfg Config
			if unmarshalErr := json.Unmarshal(content, &fileCfg); unmarshalErr == nil {
				fileCfg.DropInvalidAccounts()
				return fileCfg, false, err
			}
		}
		if errors.Is(fileErr, os.ErrNotExist) {
			if validateErr := ValidateConfig(cfg); validateErr != nil {
				return cfg, true, validateErr
			}
			if writeErr := writeConfigFile(path, cfg.Clone()); writeErr == nil {
				return cfg, false, err
			} else {
				Logger.Warn("[config] env writeback bootstrap failed", "error", writeErr)
			}
		}
		return cfg, true, err
	}
	cfg, err := loadConfigFromFile(path)
	if err != nil {
		if shouldTryLegacyContainerConfigPath() {
			legacyPath := legacyContainerConfigPath()
			if legacyCfg, legacyErr := loadConfigFromFile(legacyPath); legacyErr == nil {
				Logger.Info("[config] loaded legacy container config path", "path", legacyPath)
				return legacyCfg, false, nil
			}
		}
		if IsVercel() {
			// Vercel may start without writable/present config; keep in-memory bootstrap config.
			return Config{}, true, nil
		}
		if shouldBootstrapMissingConfigFile(err) {
			Logger.Warn("[config] config file missing; starting with empty file-backed config", "path", path)
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	if IsVercel() {
		// Vercel filesystem is ephemeral/read-only for runtime writes; avoid save errors.
		return cfg, true, nil
	}
	return cfg, false, nil
}

func shouldBootstrapMissingConfigFile(err error) bool {
	return errors.Is(err, os.ErrNotExist) && strings.TrimSpace(os.Getenv("DS2API_CONFIG_PATH")) != ""
}

func loadConfigFromFile(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return Config{}, err
	}
	cfg.NormalizeCredentials()
	cfg.DropInvalidAccounts()
	if strings.Contains(string(content), `"test_status"`) && !IsVercel() {
		if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			_ = os.WriteFile(path, b, 0o644)
		}
	}
	return cfg, nil
}

func (s *Store) Snapshot() Config {
	if s == nil {
		return Config{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Clone()
}

func (s *Store) HasAPIKey(k string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.keyMap[k]
	return ok
}

func (s *Store) Keys() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Keys)
}

func (s *Store) APIKeys() []APIKey {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.APIKeys)
}

func (s *Store) Accounts() []Account {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.cfg.Accounts)
}

func (s *Store) AccountCount() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cfg.Accounts)
}

func (s *Store) FindAccount(identifier string) (Account, bool) {
	if s == nil {
		return Account{}, false
	}
	identifier = strings.TrimSpace(identifier)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if idx, ok := s.findAccountIndexLocked(identifier); ok {
		return s.cfg.Accounts[idx], true
	}
	if looksLikeMobileIdentifier(identifier) {
		mobile := CanonicalMobileKey(identifier)
		if idx, ok := s.findAccountIndexLocked(mobile); ok {
			return s.cfg.Accounts[idx], true
		}
	}
	return Account{}, false
}

func (s *Store) AccountsPage(page, pageSize int, query string) ([]Account, int) {
	if s == nil {
		return nil, 0
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	query = strings.ToLower(strings.TrimSpace(query))
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := 0
	start := (page - 1) * pageSize
	items := make([]Account, 0, pageSize)
	for i := len(s.cfg.Accounts) - 1; i >= 0; i-- {
		acc := s.cfg.Accounts[i]
		if query != "" && !accountMatchesQuery(acc, query) {
			continue
		}
		if total >= start && len(items) < pageSize {
			items = append(items, acc)
		}
		total++
	}
	return items, total
}

func (s *Store) FindProxy(identifier string) (Proxy, bool) {
	if s == nil {
		return Proxy{}, false
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return Proxy{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, proxyCfg := range s.cfg.Proxies {
		proxyCfg = NormalizeProxy(proxyCfg)
		if proxyCfg.ID == identifier {
			return proxyCfg, true
		}
	}
	return Proxy{}, false
}

func (s *Store) Proxies() []Proxy {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Proxy, 0, len(s.cfg.Proxies))
	for _, proxyCfg := range s.cfg.Proxies {
		out = append(out, NormalizeProxy(proxyCfg))
	}
	return out
}

func accountMatchesQuery(acc Account, q string) bool {
	if q == "" {
		return true
	}
	values := []string{
		acc.Identifier(),
		acc.Name,
		acc.Remark,
		acc.Email,
		acc.Mobile,
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), q) {
			return true
		}
	}
	return false
}

func looksLikeMobileIdentifier(identifier string) bool {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || strings.Contains(identifier, "@") {
		return false
	}
	hasDigit := false
	digits := 0
	for _, r := range identifier {
		if r >= '0' && r <= '9' {
			hasDigit = true
			digits++
			continue
		}
		switch r {
		case '+', ' ', '\t', '\n', '\r', '-', '(', ')', '.', '/':
			continue
		default:
			return false
		}
	}
	return hasDigit && digits >= 6
}

func (s *Store) UpdateAccountTestStatus(identifier, status string) error {
	if s == nil {
		return errors.New("config store is nil")
	}
	identifier = strings.TrimSpace(identifier)
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.findAccountIndexLocked(identifier)
	if !ok {
		return errors.New("account not found")
	}
	s.setAccountTestStatusLocked(s.cfg.Accounts[idx], status, identifier)
	return nil
}

func (s *Store) AccountTestStatus(identifier string) (string, bool) {
	if s == nil {
		return "", false
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	status, ok := s.accTest[identifier]
	return status, ok
}

func (s *Store) UpdateAccountToken(identifier, token string) error {
	if s == nil {
		return errors.New("config store is nil")
	}
	identifier = strings.TrimSpace(identifier)
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.findAccountIndexLocked(identifier)
	if !ok {
		return errors.New("account not found")
	}
	oldID := s.cfg.Accounts[idx].Identifier()
	s.cfg.Accounts[idx].Token = token
	newID := s.cfg.Accounts[idx].Identifier()
	// Keep historical aliases usable for long-lived queues while also adding
	// the latest identifier after token refresh.
	if identifier != "" {
		s.accMap[identifier] = idx
	}
	if oldID != "" {
		s.accMap[oldID] = idx
	}
	if newID != "" {
		s.accMap[newID] = idx
	}
	return s.saveLocked()
}

func (s *Store) Replace(cfg Config) error {
	if s == nil {
		return errors.New("config store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg.NormalizeCredentials()
	s.cfg = cfg.Clone()
	s.rebuildIndexes()
	return s.saveLocked()
}

func (s *Store) Update(mutator func(*Config) error) error {
	if s == nil {
		return errors.New("config store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	base := s.cfg.Clone()
	cfg := base.Clone()
	if err := mutator(&cfg); err != nil {
		return err
	}
	cfg.ReconcileCredentials(base)
	cfg.NormalizeCredentials()
	s.cfg = cfg
	s.rebuildIndexes()
	return s.saveLocked()
}

func (s *Store) Save() error {
	if s == nil {
		return errors.New("config store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fromEnv && (IsVercel() || !envWritebackEnabled()) {
		Logger.Info("[save_config] source from env, skip write")
		return nil
	}
	persistCfg := s.cfg.Clone()
	persistCfg.ClearAccountTokens()
	b, err := json.MarshalIndent(persistCfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writeConfigBytes(s.path, b); err != nil {
		return err
	}
	s.fromEnv = false
	return nil
}

func (s *Store) saveLocked() error {
	if s.fromEnv && (IsVercel() || !envWritebackEnabled()) {
		Logger.Info("[save_config] source from env, skip write")
		return nil
	}
	persistCfg := s.cfg.Clone()
	persistCfg.ClearAccountTokens()
	b, err := json.MarshalIndent(persistCfg, "", "  ")
	if err != nil {
		return err
	}
	if err := writeConfigBytes(s.path, b); err != nil {
		return err
	}
	s.fromEnv = false
	return nil
}

func (s *Store) IsEnvBacked() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fromEnv
}

func (s *Store) SetVercelSync(hash string, ts int64) error {
	if s == nil {
		return errors.New("config store is nil")
	}
	return s.Update(func(c *Config) error {
		c.VercelSyncHash = hash
		c.VercelSyncTime = ts
		return nil
	})
}

func (s *Store) ExportJSONAndBase64() (string, string, error) {
	if s == nil {
		return "", "", errors.New("config store is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	exportCfg := s.cfg.Clone()
	exportCfg.ClearAccountTokens()
	b, err := json.Marshal(exportCfg)
	if err != nil {
		return "", "", err
	}
	return string(b), base64.StdEncoding.EncodeToString(b), nil
}
