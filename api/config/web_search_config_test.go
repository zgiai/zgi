package config

import (
	"strings"
	"testing"
)

func TestLoadWebSearchConfigDefaults(t *testing.T) {
	cfg := &Config{}
	if err := loadWebSearchConfig(cfg, webSearchEnvSource(nil)); err != nil {
		t.Fatalf("loadWebSearchConfig() error = %v", err)
	}

	if cfg.WebSearch.Enabled {
		t.Fatal("WebSearch.Enabled = true, want false")
	}
	if cfg.WebSearch.Provider != "exa" {
		t.Fatalf("WebSearch.Provider = %q, want exa", cfg.WebSearch.Provider)
	}
	if cfg.WebSearch.OrgDailyLimit != 1000 {
		t.Fatalf("WebSearch.OrgDailyLimit = %d, want 1000", cfg.WebSearch.OrgDailyLimit)
	}
	if cfg.WebSearch.Exa.TimeoutSeconds != 20 {
		t.Fatalf("WebSearch.Exa.TimeoutSeconds = %d, want 20", cfg.WebSearch.Exa.TimeoutSeconds)
	}
	if cfg.WebSearch.Exa.MaxResults != 10 {
		t.Fatalf("WebSearch.Exa.MaxResults = %d, want 10", cfg.WebSearch.Exa.MaxResults)
	}
	if cfg.WebSearch.Exa.DefaultSearchType != "auto" {
		t.Fatalf("WebSearch.Exa.DefaultSearchType = %q, want auto", cfg.WebSearch.Exa.DefaultSearchType)
	}
	if cfg.WebSearch.Exa.MaxFetchURLs != 5 {
		t.Fatalf("WebSearch.Exa.MaxFetchURLs = %d, want 5", cfg.WebSearch.Exa.MaxFetchURLs)
	}
	if cfg.WebSearch.Exa.MaxContentCharacters != 20000 {
		t.Fatalf("WebSearch.Exa.MaxContentCharacters = %d, want 20000", cfg.WebSearch.Exa.MaxContentCharacters)
	}
}

func TestLoadWebSearchConfigFromEnvironment(t *testing.T) {
	cfg := &Config{}
	source := webSearchEnvSource(map[string]string{
		envWebSearchEnabled:        "true",
		envWebSearchProvider:       " EXA ",
		envWebSearchOrgDailyLimit:  "321",
		envExaTimeoutSeconds:       "30",
		envExaMaxResults:           "8",
		envExaDefaultSearchType:    " FAST ",
		envExaMaxFetchURLs:         "4",
		envExaMaxContentCharacters: "12000",
	})

	if err := loadWebSearchConfig(cfg, source); err != nil {
		t.Fatalf("loadWebSearchConfig() error = %v", err)
	}

	if !cfg.WebSearch.Enabled || cfg.WebSearch.Provider != "exa" {
		t.Fatalf("WebSearch enabled/provider = %v/%q, want true/exa", cfg.WebSearch.Enabled, cfg.WebSearch.Provider)
	}
	if cfg.WebSearch.OrgDailyLimit != 321 || cfg.WebSearch.Exa.TimeoutSeconds != 30 {
		t.Fatalf("WebSearch limits = daily %d, timeout %d", cfg.WebSearch.OrgDailyLimit, cfg.WebSearch.Exa.TimeoutSeconds)
	}
	if cfg.WebSearch.Exa.MaxResults != 8 || cfg.WebSearch.Exa.DefaultSearchType != "fast" || cfg.WebSearch.Exa.MaxFetchURLs != 4 || cfg.WebSearch.Exa.MaxContentCharacters != 12000 {
		t.Fatalf("unexpected Exa limits: %+v", cfg.WebSearch.Exa)
	}
}

func TestLoadWebSearchConfigRejectsInvalidScalar(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "enabled", key: envWebSearchEnabled, value: "sometimes"},
		{name: "daily limit", key: envWebSearchOrgDailyLimit, value: "many"},
		{name: "timeout", key: envExaTimeoutSeconds, value: "slow"},
		{name: "max results", key: envExaMaxResults, value: "all"},
		{name: "max fetch URLs", key: envExaMaxFetchURLs, value: "several"},
		{name: "max content characters", key: envExaMaxContentCharacters, value: "large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			err := loadWebSearchConfig(cfg, webSearchEnvSource(map[string]string{tt.key: tt.value}))
			if err == nil || !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("loadWebSearchConfig() error = %v, want error containing %s", err, tt.key)
			}
		})
	}
}

func TestValidateWebSearchConfig(t *testing.T) {
	valid := WebSearchConfig{
		Enabled:       true,
		Provider:      "exa",
		OrgDailyLimit: 1000,
		Exa: ExaConfig{
			TimeoutSeconds:       20,
			MaxResults:           10,
			DefaultSearchType:    "auto",
			MaxFetchURLs:         5,
			MaxContentCharacters: 20000,
		},
	}

	if err := validateWebSearchConfig(WebSearchConfig{}); err != nil {
		t.Fatalf("disabled config validation error = %v", err)
	}
	if err := validateWebSearchConfig(valid); err != nil {
		t.Fatalf("valid config validation error = %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*WebSearchConfig)
		wantKey string
	}{
		{name: "provider", mutate: func(cfg *WebSearchConfig) { cfg.Provider = "other" }, wantKey: envWebSearchProvider},
		{name: "daily limit", mutate: func(cfg *WebSearchConfig) { cfg.OrgDailyLimit = 0 }, wantKey: envWebSearchOrgDailyLimit},
		{name: "timeout", mutate: func(cfg *WebSearchConfig) { cfg.Exa.TimeoutSeconds = 0 }, wantKey: envExaTimeoutSeconds},
		{name: "max results", mutate: func(cfg *WebSearchConfig) { cfg.Exa.MaxResults = 0 }, wantKey: envExaMaxResults},
		{name: "max results cap", mutate: func(cfg *WebSearchConfig) { cfg.Exa.MaxResults = 11 }, wantKey: envExaMaxResults},
		{name: "default search type", mutate: func(cfg *WebSearchConfig) { cfg.Exa.DefaultSearchType = "deep" }, wantKey: envExaDefaultSearchType},
		{name: "max fetch URLs", mutate: func(cfg *WebSearchConfig) { cfg.Exa.MaxFetchURLs = 0 }, wantKey: envExaMaxFetchURLs},
		{name: "max fetch URLs cap", mutate: func(cfg *WebSearchConfig) { cfg.Exa.MaxFetchURLs = 6 }, wantKey: envExaMaxFetchURLs},
		{name: "max content characters", mutate: func(cfg *WebSearchConfig) { cfg.Exa.MaxContentCharacters = 0 }, wantKey: envExaMaxContentCharacters},
		{name: "max content characters cap", mutate: func(cfg *WebSearchConfig) { cfg.Exa.MaxContentCharacters = 20001 }, wantKey: envExaMaxContentCharacters},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid
			tt.mutate(&candidate)
			err := validateWebSearchConfig(candidate)
			if err == nil || !strings.Contains(err.Error(), tt.wantKey) {
				t.Fatalf("validateWebSearchConfig() error = %v, want error containing %s", err, tt.wantKey)
			}
		})
	}
}

func TestValidateWebSearchAuditKey(t *testing.T) {
	if err := validateWebSearchAuditKey(WebSearchConfig{}, ""); err != nil {
		t.Fatalf("disabled web search audit-key validation error = %v", err)
	}
	if err := validateWebSearchAuditKey(WebSearchConfig{Enabled: true}, ""); err == nil || !strings.Contains(err.Error(), envAPIKeyEncryptionKey) {
		t.Fatalf("enabled web search audit-key validation error = %v", err)
	}
	if err := validateWebSearchAuditKey(WebSearchConfig{Enabled: true}, "too-short"); err == nil || !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("short web search audit-key validation error = %v", err)
	}
	if err := validateWebSearchAuditKey(WebSearchConfig{Enabled: true}, "01234567890123456789012345678901"); err != nil {
		t.Fatalf("configured web search audit-key validation error = %v", err)
	}
}

func webSearchEnvSource(values map[string]string) *envSource {
	return &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
}
