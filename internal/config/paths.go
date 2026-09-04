package config

import (
	"os"
	"path/filepath"
	"strings"
)

func BaseDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func ResolvePath(envKey, defaultRel string) string {
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw != "" {
		if filepath.IsAbs(raw) {
			return raw
		}
		return filepath.Join(BaseDir(), raw)
	}
	return filepath.Join(BaseDir(), defaultRel)
}

func ConfigPath() string {
	if strings.TrimSpace(os.Getenv("DS2API_CONFIG_PATH")) == "" && BaseDir() == "/app" {
		return containerDefaultConfigPath()
	}
	return ResolvePath("DS2API_CONFIG_PATH", "config.json")
}

func containerDefaultConfigPath() string {
	// Container images run as non-root by default. Only use /data when mounted/provisioned.
	// Otherwise keep /app/config.json so runtime saves do not fail on MkdirAll("/data").
	if st, err := os.Stat("/data"); err == nil && st.IsDir() {
		return "/data/config.json"
	}
	return "/app/config.json"
}

func legacyContainerConfigPath() string {
	return "/app/config.json"
}

func shouldTryLegacyContainerConfigPath() bool {
	return strings.TrimSpace(os.Getenv("DS2API_CONFIG_PATH")) == "" && BaseDir() == "/app"
}

func ChatHistoryPath() string {
	return ResolvePath("DS2API_CHAT_HISTORY_PATH", "data/chat_history.json")
}

func RuntimeStatsPath() string {
	return ResolvePath("DS2API_RUNTIME_STATS_PATH", "data/runtime_stats.json")
}
